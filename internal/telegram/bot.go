package telegram

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"math/big"
	"net/http"
	neturl "net/url"
	"strings"
	"time"

	"github.com/csweichel/blackwood/internal/describe"
	"github.com/csweichel/blackwood/internal/index"
	"github.com/csweichel/blackwood/internal/opengraph"
	"github.com/csweichel/blackwood/internal/storage"
	"github.com/csweichel/blackwood/internal/transcribe"
)

// Telegram Bot API types.

type getUpdatesResponse struct {
	OK     bool     `json:"ok"`
	Result []Update `json:"result"`
}

type Update struct {
	UpdateID int64    `json:"update_id"`
	Message  *Message `json:"message,omitempty"`
}

type Message struct {
	MessageID int64           `json:"message_id"`
	Chat      Chat            `json:"chat"`
	Text      string          `json:"text,omitempty"`
	Voice     *Voice          `json:"voice,omitempty"`
	Photo     []PhotoSize     `json:"photo,omitempty"`
	Caption   string          `json:"caption,omitempty"`
	Entities  []MessageEntity `json:"entities,omitempty"`
}

type MessageEntity struct {
	Type   string `json:"type"`   // "url", "text_link", etc.
	Offset int    `json:"offset"` // offset in UTF-16 code units
	Length int    `json:"length"` // length in UTF-16 code units
	URL    string `json:"url"`    // only for "text_link" type
}

type Chat struct {
	ID int64 `json:"id"`
}

type Voice struct {
	FileID   string `json:"file_id"`
	MimeType string `json:"mime_type,omitempty"`
}

type PhotoSize struct {
	FileID   string `json:"file_id"`
	Width    int    `json:"width"`
	Height   int    `json:"height"`
	FileSize int    `json:"file_size,omitempty"`
}

type getFileResponse struct {
	OK     bool `json:"ok"`
	Result struct {
		FilePath string `json:"file_path"`
	} `json:"result"`
}

type sendMessageRequest struct {
	ChatID int64  `json:"chat_id"`
	Text   string `json:"text"`
}

// Bot receives Telegram messages and adds them as daily note entries.
type Bot struct {
	token       string
	staticIDs   map[int64]bool // from config, always authorized
	authCode    string         // one-time auth code for pairing
	store       *storage.Store
	transcriber transcribe.Transcriber
	describer   describe.Describer
	indexer     *index.Index
}

// BotConfig holds the configuration for the Telegram bot.
type BotConfig struct {
	Token          string
	AllowedChatIDs []int64
}

// NewBot creates a new Telegram bot.
func NewBot(cfg BotConfig, store *storage.Store, transcriber transcribe.Transcriber, describer describe.Describer, indexer *index.Index) *Bot {
	static := make(map[int64]bool, len(cfg.AllowedChatIDs))
	for _, id := range cfg.AllowedChatIDs {
		static[id] = true
	}

	code := generateAuthCode()

	return &Bot{
		token:       cfg.Token,
		staticIDs:   static,
		authCode:    code,
		store:       store,
		transcriber: transcriber,
		describer:   describer,
		indexer:     indexer,
	}
}

// generateAuthCode returns a cryptographically random 6-digit code.
func generateAuthCode() string {
	n, err := rand.Int(rand.Reader, big.NewInt(1_000_000))
	if err != nil {
		return fmt.Sprintf("%06d", time.Now().UnixNano()%1_000_000)
	}
	return fmt.Sprintf("%06d", n.Int64())
}

// Start runs the long-poll loop. It blocks until ctx is cancelled.
func (b *Bot) Start(ctx context.Context) {
	slog.Info("telegram: bot started — send this code to the bot to authorize a chat",
		"auth_code", b.authCode)

	var offset int64
	client := &http.Client{Timeout: 60 * time.Second}

	for {
		if ctx.Err() != nil {
			return
		}

		updates, err := b.getUpdates(ctx, client, offset)
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			slog.Error("telegram: getUpdates", "error", err)
			select {
			case <-time.After(5 * time.Second):
			case <-ctx.Done():
				return
			}
			continue
		}

		for _, u := range updates {
			if u.UpdateID >= offset {
				offset = u.UpdateID + 1
			}
			if u.Message != nil {
				b.processMessage(ctx, client, u.Message)
			}
		}
	}
}

func (b *Bot) getUpdates(ctx context.Context, client *http.Client, offset int64) ([]Update, error) {
	url := fmt.Sprintf("https://api.telegram.org/bot%s/getUpdates?offset=%d&timeout=30", b.token, offset)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	var result getUpdatesResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}
	if !result.OK {
		return nil, fmt.Errorf("telegram API returned ok=false")
	}
	return result.Result, nil
}

// isAuthorized checks if a chat ID is allowed (static config or DB-persisted).
func (b *Bot) isAuthorized(ctx context.Context, chatID int64) bool {
	if b.staticIDs[chatID] {
		return true
	}
	ok, err := b.store.IsTelegramChatAuthorized(ctx, chatID)
	if err != nil {
		slog.Error("telegram: check auth", "chat_id", chatID, "error", err)
		return false
	}
	return ok
}

func (b *Bot) processMessage(ctx context.Context, client *http.Client, msg *Message) {
	chatID := msg.Chat.ID
	text := strings.TrimSpace(msg.Text)

	// Handle auth code from anyone — this is how new chats get authorized.
	if text == b.authCode {
		if err := b.store.AuthorizeTelegramChat(ctx, chatID); err != nil {
			slog.Error("telegram: authorize chat", "chat_id", chatID, "error", err)
			b.sendMessage(ctx, client, chatID, "Failed to authorize. Check server logs.")
			return
		}
		slog.Info("telegram: chat authorized", "chat_id", chatID)
		b.sendMessage(ctx, client, chatID, "✓ Authorized! You can now send messages to your daily notes.\n\nSend text, voice messages, or photos and they'll be added to today's note.")
		// Rotate the code so it can't be reused by someone else.
		b.authCode = generateAuthCode()
		slog.Info("telegram: new auth code generated — check logs to authorize another chat",
			"auth_code", b.authCode)
		return
	}

	// Handle /start for unauthenticated users.
	if text == "/start" && !b.isAuthorized(ctx, chatID) {
		b.sendMessage(ctx, client, chatID, "Welcome to Blackwood!\n\nTo connect this chat, send the authorization code shown in the Blackwood server logs.")
		return
	}

	// Handle /revoke from authorized users.
	if text == "/revoke" {
		if b.staticIDs[chatID] {
			b.sendMessage(ctx, client, chatID, "This chat is authorized via server config and cannot be revoked here.")
			return
		}
		if err := b.store.RevokeTelegramChat(ctx, chatID); err != nil {
			slog.Error("telegram: revoke chat", "chat_id", chatID, "error", err)
			b.sendMessage(ctx, client, chatID, "Failed to revoke. Check server logs.")
			return
		}
		slog.Info("telegram: chat revoked", "chat_id", chatID)
		b.sendMessage(ctx, client, chatID, "✓ Access revoked. Send the auth code again to re-authorize.")
		return
	}

	// All other messages require authorization.
	if !b.isAuthorized(ctx, chatID) {
		slog.Warn("telegram: message from unauthorized chat", "chat_id", chatID)
		b.sendMessage(ctx, client, chatID, "This chat is not authorized. Send the authorization code from the Blackwood server logs to get started.")
		return
	}

	// Authorized — process the message.
	switch {
	case msg.Voice != nil:
		b.handleVoice(ctx, client, chatID, msg.Voice)
	case len(msg.Photo) > 0:
		b.handlePhoto(ctx, client, chatID, msg.Photo, msg.Caption)
	case text != "":
		b.handleText(ctx, client, chatID, text, msg.Entities)
	default:
		slog.Info("telegram: unsupported message type", "chat_id", chatID)
	}
}

func (b *Bot) handleText(ctx context.Context, client *http.Client, chatID int64, text string, entities []MessageEntity) {
	// Check for RAG query prefix.
	if strings.HasPrefix(text, "Q:") {
		slog.Warn("telegram: RAG queries not yet supported via Telegram", "chat_id", chatID)
		b.sendMessage(ctx, client, chatID, "RAG queries are not yet supported via Telegram")
		return
	}

	now := time.Now()
	date := now.Format("2006-01-02")

	note, err := b.store.GetOrCreateDailyNote(ctx, date)
	if err != nil {
		slog.Error("telegram: get daily note", "error", err)
		return
	}

	// Extract URLs from entities.
	urls := extractURLs(text, entities)

	// Build the snippet content: if we have URLs, try to generate preview cards.
	snippetBody := text
	if len(urls) > 0 {
		snippetBody = b.buildSnippetWithPreviews(ctx, text, urls, note.ID, date)
	}

	snippet := fmt.Sprintf("\n\n---\n*%s — Telegram*\n\n%s\n", now.Format("15:04"), snippetBody)
	if err := b.store.AppendDailyNoteContent(ctx, note.ID, snippet); err != nil {
		slog.Error("telegram: append text", "error", err)
		return
	}

	entry := &storage.Entry{
		DailyNoteID: note.ID,
		Type:        "text",
		Content:     text,
		Source:      "telegram",
	}
	if err := b.store.CreateEntry(ctx, entry); err != nil {
		slog.Error("telegram: create text entry", "error", err)
		return
	}

	if b.indexer != nil && text != "" {
		if err := b.indexer.IndexEntry(ctx, entry.ID, text); err != nil {
			slog.Warn("telegram: failed to index text entry", "entry_id", entry.ID, "error", err)
		}
	}

	b.sendMessage(ctx, client, chatID, "✓ Added to your daily notes")
}

// extractedURL holds a URL and its position in the original message text.
type extractedURL struct {
	URL    string
	Offset int // byte offset in the Go string
	Length int // byte length in the Go string
}

// extractURLs pulls URLs from Telegram message entities.
// Telegram encodes offsets/lengths in UTF-16 code units, so we convert accordingly.
func extractURLs(text string, entities []MessageEntity) []extractedURL {
	if len(entities) == 0 {
		return nil
	}

	// Convert Go string (UTF-8) to UTF-16 code units for offset mapping.
	runes := []rune(text)
	// Build a mapping from UTF-16 offset to rune index.
	utf16Pos := 0
	utf16ToRuneIdx := make(map[int]int, len(runes))
	for i, r := range runes {
		utf16ToRuneIdx[utf16Pos] = i
		if r >= 0x10000 {
			utf16Pos += 2 // surrogate pair
		} else {
			utf16Pos++
		}
	}

	var urls []extractedURL
	for _, e := range entities {
		var u string
		switch e.Type {
		case "url":
			// The URL is embedded in the text at the given offset/length.
			startRune, ok := utf16ToRuneIdx[e.Offset]
			if !ok {
				continue
			}
			// Find the end rune index by scanning forward.
			endUTF16 := e.Offset + e.Length
			endRune, ok := utf16ToRuneIdx[endUTF16]
			if !ok {
				endRune = len(runes)
			}
			u = string(runes[startRune:endRune])
		case "text_link":
			u = e.URL
		default:
			continue
		}
		if u == "" {
			continue
		}
		// Ensure the URL has a scheme.
		if !strings.Contains(u, "://") {
			u = "https://" + u
		}

		// Compute byte offset/length for the URL in the original text (for "url" type).
		var byteOff, byteLen int
		if e.Type == "url" {
			startRune := utf16ToRuneIdx[e.Offset]
			endUTF16 := e.Offset + e.Length
			endRune, ok := utf16ToRuneIdx[endUTF16]
			if !ok {
				endRune = len(runes)
			}
			prefix := string(runes[:startRune])
			extracted := string(runes[startRune:endRune])
			byteOff = len(prefix)
			byteLen = len(extracted)
		}

		urls = append(urls, extractedURL{URL: u, Offset: byteOff, Length: byteLen})
	}
	return urls
}

// buildSnippetWithPreviews replaces URLs in the message text with OG preview cards.
// Non-URL text is preserved. If a fetch fails, the raw URL is kept.
func (b *Bot) buildSnippetWithPreviews(ctx context.Context, text string, urls []extractedURL, noteID, date string) string {
	// For "text_link" entities (Offset/Length == 0), we can't replace in-text,
	// so we append cards at the end.
	type replacement struct {
		byteOffset int
		byteLength int
		card       string
	}

	var replacements []replacement
	var appendCards []string

	for _, u := range urls {
		card, err := opengraph.Fetch(ctx, u.URL)
		if err != nil {
			slog.Debug("telegram: opengraph fetch failed", "url", u.URL, "error", err)
			continue
		}
		if card == nil {
			continue
		}

		cardMD := b.formatCard(ctx, card, u.URL, noteID, date)

		if u.Length > 0 {
			replacements = append(replacements, replacement{
				byteOffset: u.Offset,
				byteLength: u.Length,
				card:       cardMD,
			})
		} else {
			appendCards = append(appendCards, cardMD)
		}
	}

	if len(replacements) == 0 && len(appendCards) == 0 {
		return text
	}

	// Sort replacements by offset descending so we can replace from the end
	// without invalidating earlier offsets.
	for i := 0; i < len(replacements); i++ {
		for j := i + 1; j < len(replacements); j++ {
			if replacements[j].byteOffset > replacements[i].byteOffset {
				replacements[i], replacements[j] = replacements[j], replacements[i]
			}
		}
	}

	result := text
	for _, r := range replacements {
		before := result[:r.byteOffset]
		after := result[r.byteOffset+r.byteLength:]
		result = before + r.card + after
	}

	for _, c := range appendCards {
		result += "\n\n" + c
	}

	return result
}

// formatCard renders an OG card as a markdown blockquote.
func (b *Bot) formatCard(ctx context.Context, card *opengraph.Card, rawURL, noteID, date string) string {
	var lines []string

	// Title line, optionally with site name.
	titleLine := card.Title
	if card.SiteName != "" && card.Title != "" {
		titleLine = card.Title + " — " + card.SiteName
	} else if card.SiteName != "" {
		titleLine = card.SiteName
	}
	if titleLine != "" {
		lines = append(lines, "> **"+titleLine+"**")
	}

	if card.Description != "" {
		lines = append(lines, "> "+card.Description)
	}

	// Download and attach the OG image if present.
	if card.Image != "" {
		if imgRef := b.downloadOGImage(ctx, card.Image, noteID, date); imgRef != "" {
			lines = append(lines, "> "+imgRef)
		}
	}

	// Link line — show a short domain/path.
	displayURL := rawURL
	if parsed, err := neturl.Parse(rawURL); err == nil {
		displayURL = parsed.Host + parsed.Path
		// Trim trailing slash for cleanliness.
		displayURL = strings.TrimRight(displayURL, "/")
	}
	lines = append(lines, "> ["+displayURL+"]("+rawURL+")")

	return strings.Join(lines, "\n")
}

// downloadOGImage fetches an OG image and stores it as an attachment.
// Returns a markdown image reference, or empty string on failure.
func (b *Bot) downloadOGImage(ctx context.Context, imageURL, noteID, date string) string {
	imgCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(imgCtx, http.MethodGet, imageURL, nil)
	if err != nil {
		slog.Debug("telegram: create og image request", "error", err)
		return ""
	}
	req.Header.Set("User-Agent", "Blackwood/1.0 (link preview)")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		slog.Debug("telegram: download og image", "url", imageURL, "error", err)
		return ""
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return ""
	}

	// Limit image download to 5 MB.
	data, err := io.ReadAll(io.LimitReader(resp.Body, 5*1024*1024))
	if err != nil {
		slog.Debug("telegram: read og image", "error", err)
		return ""
	}
	if len(data) == 0 {
		return ""
	}

	// Determine content type and extension.
	contentType := resp.Header.Get("Content-Type")
	ext := ".jpg"
	switch {
	case strings.Contains(contentType, "png"):
		ext = ".png"
	case strings.Contains(contentType, "gif"):
		ext = ".gif"
	case strings.Contains(contentType, "webp"):
		ext = ".webp"
	case strings.Contains(contentType, "svg"):
		ext = ".svg"
	}

	// We need an entry to attach to. Create a lightweight entry for the OG image.
	entry := &storage.Entry{
		DailyNoteID: noteID,
		Type:        "text",
		Content:     "og-preview image",
		Source:      "telegram",
	}
	if err := b.store.CreateEntry(ctx, entry); err != nil {
		slog.Debug("telegram: create og image entry", "error", err)
		return ""
	}

	att := &storage.Attachment{
		EntryID:     entry.ID,
		Filename:    "og-preview" + ext,
		ContentType: contentType,
	}
	if err := b.store.CreateAttachment(ctx, att, data, date); err != nil {
		slog.Debug("telegram: create og image attachment", "error", err)
		return ""
	}

	return fmt.Sprintf("![](/api/attachments/%s)", att.ID)
}

func (b *Bot) handleVoice(ctx context.Context, client *http.Client, chatID int64, voice *Voice) {
	if b.transcriber == nil {
		slog.Warn("telegram: voice received but no transcriber configured")
		return
	}

	now := time.Now()
	date := now.Format("2006-01-02")

	data, err := b.downloadFile(ctx, client, voice.FileID)
	if err != nil {
		slog.Error("telegram: download voice", "error", err)
		return
	}

	mimeType := voice.MimeType
	if mimeType == "" {
		mimeType = "audio/ogg"
	}
	format := formatFromMime(mimeType)

	text, err := b.transcriber.Transcribe(ctx, data, format)
	if err != nil {
		slog.Error("telegram: transcribe voice", "error", err)
		return
	}

	note, err := b.store.GetOrCreateDailyNote(ctx, date)
	if err != nil {
		slog.Error("telegram: get daily note", "error", err)
		return
	}

	entry := &storage.Entry{
		DailyNoteID: note.ID,
		Type:        "audio",
		Content:     text,
		Source:      "telegram",
	}
	if err := b.store.CreateEntry(ctx, entry); err != nil {
		slog.Error("telegram: create voice entry", "error", err)
		return
	}

	if b.indexer != nil && text != "" {
		if err := b.indexer.IndexEntry(ctx, entry.ID, text); err != nil {
			slog.Warn("telegram: failed to index voice entry", "entry_id", entry.ID, "error", err)
		}
	}

	contentType := mimeType
	att := &storage.Attachment{
		EntryID:     entry.ID,
		Filename:    "voice." + format,
		ContentType: contentType,
	}
	if err := b.store.CreateAttachment(ctx, att, data, date); err != nil {
		slog.Error("telegram: create voice attachment", "error", err)
		return
	}

	audioRef := fmt.Sprintf(`<audio controls src="/api/attachments/%s"></audio>`, att.ID)
	snippet := fmt.Sprintf("\n\n---\n*%s — Telegram voice message*\n\n%s\n\n%s\n", now.Format("15:04"), audioRef, text)
	if err := b.store.AppendDailyNoteContent(ctx, note.ID, snippet); err != nil {
		slog.Error("telegram: append voice transcription", "error", err)
		return
	}

	b.sendMessage(ctx, client, chatID, "✓ Added to your daily notes")
}

func (b *Bot) handlePhoto(ctx context.Context, client *http.Client, chatID int64, photos []PhotoSize, caption string) {
	if b.describer == nil {
		slog.Warn("telegram: photo received but no describer configured")
		return
	}

	now := time.Now()
	date := now.Format("2006-01-02")

	// Use the last PhotoSize (largest resolution).
	photo := photos[len(photos)-1]

	data, err := b.downloadFile(ctx, client, photo.FileID)
	if err != nil {
		slog.Error("telegram: download photo", "error", err)
		return
	}

	description, err := b.describer.Describe(ctx, data)
	if err != nil {
		slog.Error("telegram: describe photo", "error", err)
		return
	}

	note, err := b.store.GetOrCreateDailyNote(ctx, date)
	if err != nil {
		slog.Error("telegram: get daily note", "error", err)
		return
	}

	snippet := fmt.Sprintf("\n\n---\n*%s — Telegram photo*\n\n%s\n", now.Format("15:04"), description)
	if err := b.store.AppendDailyNoteContent(ctx, note.ID, snippet); err != nil {
		slog.Error("telegram: append photo description", "error", err)
		return
	}

	entry := &storage.Entry{
		DailyNoteID: note.ID,
		Type:        "photo",
		Content:     description,
		Source:      "telegram",
	}
	if err := b.store.CreateEntry(ctx, entry); err != nil {
		slog.Error("telegram: create photo entry", "error", err)
		return
	}

	if b.indexer != nil && description != "" {
		if err := b.indexer.IndexEntry(ctx, entry.ID, description); err != nil {
			slog.Warn("telegram: failed to index photo entry", "entry_id", entry.ID, "error", err)
		}
	}

	contentType := "image/jpeg"
	ext := "jpg"
	if err := b.store.CreateAttachment(ctx, &storage.Attachment{
		EntryID:     entry.ID,
		Filename:    "photo." + ext,
		ContentType: contentType,
	}, data, date); err != nil {
		slog.Error("telegram: create photo attachment", "error", err)
		return
	}

	b.sendMessage(ctx, client, chatID, "✓ Added to your daily notes")
}

// downloadFile retrieves a file from Telegram by file_id.
func (b *Bot) downloadFile(ctx context.Context, client *http.Client, fileID string) ([]byte, error) {
	// Step 1: get file path.
	url := fmt.Sprintf("https://api.telegram.org/bot%s/getFile?file_id=%s", b.token, fileID)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	var fileResp getFileResponse
	if err := json.NewDecoder(resp.Body).Decode(&fileResp); err != nil {
		return nil, fmt.Errorf("decode getFile response: %w", err)
	}
	if !fileResp.OK {
		return nil, fmt.Errorf("telegram getFile returned ok=false")
	}

	// Step 2: download the file.
	dlURL := fmt.Sprintf("https://api.telegram.org/file/bot%s/%s", b.token, fileResp.Result.FilePath)
	dlReq, err := http.NewRequestWithContext(ctx, http.MethodGet, dlURL, nil)
	if err != nil {
		return nil, err
	}

	dlResp, err := client.Do(dlReq)
	if err != nil {
		return nil, err
	}
	defer func() { _ = dlResp.Body.Close() }()

	return io.ReadAll(dlResp.Body)
}

// sendMessage sends a text reply to a Telegram chat.
func (b *Bot) sendMessage(ctx context.Context, client *http.Client, chatID int64, text string) {
	url := fmt.Sprintf("https://api.telegram.org/bot%s/sendMessage", b.token)

	body, err := json.Marshal(sendMessageRequest{ChatID: chatID, Text: text})
	if err != nil {
		slog.Error("telegram: marshal sendMessage", "error", err)
		return
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		slog.Error("telegram: create sendMessage request", "error", err)
		return
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		slog.Error("telegram: sendMessage", "error", err)
		return
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		slog.Error("telegram: sendMessage non-200", "status", resp.StatusCode)
	}
}

// formatFromMime extracts an audio format string from a MIME type.
func formatFromMime(mime string) string {
	switch {
	case strings.Contains(mime, "ogg"):
		return "ogg"
	case strings.Contains(mime, "mpeg"), strings.Contains(mime, "mp3"):
		return "mp3"
	case strings.Contains(mime, "mp4"), strings.Contains(mime, "m4a"):
		return "m4a"
	case strings.Contains(mime, "wav"):
		return "wav"
	case strings.Contains(mime, "webm"):
		return "webm"
	default:
		return "ogg"
	}
}
