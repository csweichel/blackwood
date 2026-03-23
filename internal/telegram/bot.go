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
	"strings"
	"time"

	"github.com/csweichel/blackwood/internal/describe"
	"github.com/csweichel/blackwood/internal/index"
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
	MessageID int64       `json:"message_id"`
	Chat      Chat        `json:"chat"`
	Text      string      `json:"text,omitempty"`
	Voice     *Voice      `json:"voice,omitempty"`
	Photo     []PhotoSize `json:"photo,omitempty"`
	Caption   string      `json:"caption,omitempty"`
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
		b.handleText(ctx, client, chatID, text)
	default:
		slog.Info("telegram: unsupported message type", "chat_id", chatID)
	}
}

func (b *Bot) handleText(ctx context.Context, client *http.Client, chatID int64, text string) {
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

	snippet := fmt.Sprintf("\n\n---\n*%s — Telegram*\n\n%s\n", now.Format("15:04"), text)
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
