package whatsapp

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/csweichel/blackwood/internal/describe"
	"github.com/csweichel/blackwood/internal/storage"
	"github.com/csweichel/blackwood/internal/transcribe"
)

// WebhookHandler handles incoming WhatsApp webhook events.
type WebhookHandler struct {
	verifyToken string
	appSecret   string
	client      *Client
	store       *storage.Store
	transcriber transcribe.Transcriber // may be nil
	describer   describe.Describer     // may be nil
}

// WebhookConfig holds the configuration for the WhatsApp webhook.
type WebhookConfig struct {
	VerifyToken   string
	AppSecret     string
	AccessToken   string
	PhoneNumberID string
}

func NewWebhookHandler(cfg WebhookConfig, store *storage.Store, transcriber transcribe.Transcriber, describer describe.Describer) *WebhookHandler {
	return &WebhookHandler{
		verifyToken: cfg.VerifyToken,
		appSecret:   cfg.AppSecret,
		client:      NewClient(cfg.AccessToken, cfg.PhoneNumberID),
		store:       store,
		transcriber: transcriber,
		describer:   describer,
	}
}

// --- Webhook payload types ---

type webhookPayload struct {
	Object string         `json:"object"`
	Entry  []webhookEntry `json:"entry"`
}

type webhookEntry struct {
	Changes []webhookChange `json:"changes"`
}

type webhookChange struct {
	Value webhookValue `json:"value"`
}

type webhookValue struct {
	Messages []webhookMessage `json:"messages"`
}

type webhookMessage struct {
	From  string        `json:"from"`
	Type  string        `json:"type"`
	Text  *messageText  `json:"text,omitempty"`
	Audio *messageMedia `json:"audio,omitempty"`
	Image *messageImage `json:"image,omitempty"`
}

type messageText struct {
	Body string `json:"body"`
}

type messageMedia struct {
	ID       string `json:"id"`
	MimeType string `json:"mime_type"`
}

type messageImage struct {
	ID       string `json:"id"`
	MimeType string `json:"mime_type"`
	Caption  string `json:"caption,omitempty"`
}

// ServeHTTP handles both GET (verification) and POST (incoming messages).
func (h *WebhookHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		h.handleVerification(w, r)
	case http.MethodPost:
		h.handleIncoming(w, r)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (h *WebhookHandler) handleVerification(w http.ResponseWriter, r *http.Request) {
	mode := r.URL.Query().Get("hub.mode")
	token := r.URL.Query().Get("hub.verify_token")
	challenge := r.URL.Query().Get("hub.challenge")

	if mode == "subscribe" && token == h.verifyToken {
		w.WriteHeader(http.StatusOK)
		_, _ = fmt.Fprint(w, challenge)
		return
	}

	http.Error(w, "forbidden", http.StatusForbidden)
}

func (h *WebhookHandler) handleIncoming(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		slog.Error("whatsapp: read body", "error", err)
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}

	// Verify HMAC-SHA256 signature if appSecret is configured.
	if h.appSecret != "" {
		sig := r.Header.Get("X-Hub-Signature-256")
		if !h.verifySignature(body, sig) {
			slog.Warn("whatsapp: invalid signature")
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
	}

	// Always return 200 quickly to acknowledge receipt.
	w.WriteHeader(http.StatusOK)

	var payload webhookPayload
	if err := json.Unmarshal(body, &payload); err != nil {
		slog.Error("whatsapp: parse payload", "error", err)
		return
	}

	if payload.Object != "whatsapp_business_account" {
		return
	}

	// Process messages synchronously (acceptable for low volume).
	for _, entry := range payload.Entry {
		for _, change := range entry.Changes {
			for _, msg := range change.Value.Messages {
				h.processMessage(r.Context(), msg)
			}
		}
	}
}

func (h *WebhookHandler) verifySignature(body []byte, signature string) bool {
	if !strings.HasPrefix(signature, "sha256=") {
		return false
	}
	expectedSig := signature[7:]

	mac := hmac.New(sha256.New, []byte(h.appSecret))
	mac.Write(body)
	computedSig := hex.EncodeToString(mac.Sum(nil))

	return hmac.Equal([]byte(computedSig), []byte(expectedSig))
}

func (h *WebhookHandler) processMessage(ctx context.Context, msg webhookMessage) {
	switch msg.Type {
	case "text":
		if msg.Text != nil {
			h.handleText(ctx, msg.From, msg.Text.Body)
		}
	case "audio":
		if msg.Audio != nil {
			h.handleAudio(ctx, msg.From, msg.Audio.ID, msg.Audio.MimeType)
		}
	case "image":
		if msg.Image != nil {
			h.handleImage(ctx, msg.From, msg.Image.ID, msg.Image.MimeType)
		}
	default:
		slog.Info("whatsapp: unsupported message type", "type", msg.Type, "from", msg.From)
	}
}

func (h *WebhookHandler) handleText(ctx context.Context, from, text string) {
	now := time.Now()
	date := now.Format("2006-01-02")

	note, err := h.store.GetOrCreateDailyNote(ctx, date)
	if err != nil {
		slog.Error("whatsapp: get daily note", "error", err)
		return
	}

	snippet := fmt.Sprintf("\n\n---\n*%s — WhatsApp*\n\n%s\n", now.Format("15:04"), text)
	if err := h.store.AppendDailyNoteContent(ctx, note.ID, snippet); err != nil {
		slog.Error("whatsapp: append text", "error", err)
		return
	}

	if err := h.store.CreateEntry(ctx, &storage.Entry{
		DailyNoteID: note.ID,
		Type:        "text",
		Content:     text,
		Source:      "whatsapp",
	}); err != nil {
		slog.Error("whatsapp: create text entry", "error", err)
		return
	}

	if err := h.client.SendTextMessage(ctx, from, "✓ Added to your daily notes"); err != nil {
		slog.Error("whatsapp: send reply", "error", err)
	}
}

func (h *WebhookHandler) handleAudio(ctx context.Context, from, mediaID, mimeType string) {
	if h.transcriber == nil {
		slog.Warn("whatsapp: audio received but no transcriber configured")
		return
	}

	now := time.Now()
	date := now.Format("2006-01-02")

	data, contentType, err := h.client.DownloadMedia(ctx, mediaID)
	if err != nil {
		slog.Error("whatsapp: download audio", "error", err)
		return
	}
	if contentType == "" {
		contentType = mimeType
	}

	format := formatFromMime(contentType)
	text, err := h.transcriber.Transcribe(ctx, data, format)
	if err != nil {
		slog.Error("whatsapp: transcribe audio", "error", err)
		return
	}

	note, err := h.store.GetOrCreateDailyNote(ctx, date)
	if err != nil {
		slog.Error("whatsapp: get daily note", "error", err)
		return
	}

	snippet := fmt.Sprintf("\n\n---\n*%s — WhatsApp voice message*\n\n%s\n", now.Format("15:04"), text)
	if err := h.store.AppendDailyNoteContent(ctx, note.ID, snippet); err != nil {
		slog.Error("whatsapp: append audio transcription", "error", err)
		return
	}

	entry := &storage.Entry{
		DailyNoteID: note.ID,
		Type:        "audio",
		Content:     text,
		Source:      "whatsapp",
	}
	if err := h.store.CreateEntry(ctx, entry); err != nil {
		slog.Error("whatsapp: create audio entry", "error", err)
		return
	}

	if err := h.store.CreateAttachment(ctx, &storage.Attachment{
		EntryID:     entry.ID,
		Filename:    "voice." + format,
		ContentType: contentType,
	}, data); err != nil {
		slog.Error("whatsapp: create audio attachment", "error", err)
		return
	}

	if err := h.client.SendTextMessage(ctx, from, "✓ Added to your daily notes"); err != nil {
		slog.Error("whatsapp: send reply", "error", err)
	}
}

func (h *WebhookHandler) handleImage(ctx context.Context, from, mediaID, mimeType string) {
	if h.describer == nil {
		slog.Warn("whatsapp: image received but no describer configured")
		return
	}

	now := time.Now()
	date := now.Format("2006-01-02")

	data, contentType, err := h.client.DownloadMedia(ctx, mediaID)
	if err != nil {
		slog.Error("whatsapp: download image", "error", err)
		return
	}
	if contentType == "" {
		contentType = mimeType
	}

	description, err := h.describer.Describe(ctx, data)
	if err != nil {
		slog.Error("whatsapp: describe image", "error", err)
		return
	}

	note, err := h.store.GetOrCreateDailyNote(ctx, date)
	if err != nil {
		slog.Error("whatsapp: get daily note", "error", err)
		return
	}

	snippet := fmt.Sprintf("\n\n---\n*%s — WhatsApp photo*\n\n%s\n", now.Format("15:04"), description)
	if err := h.store.AppendDailyNoteContent(ctx, note.ID, snippet); err != nil {
		slog.Error("whatsapp: append image description", "error", err)
		return
	}

	entry := &storage.Entry{
		DailyNoteID: note.ID,
		Type:        "photo",
		Content:     description,
		Source:      "whatsapp",
	}
	if err := h.store.CreateEntry(ctx, entry); err != nil {
		slog.Error("whatsapp: create image entry", "error", err)
		return
	}

	ext := "jpg"
	if contentType == "image/png" {
		ext = "png"
	}
	if err := h.store.CreateAttachment(ctx, &storage.Attachment{
		EntryID:     entry.ID,
		Filename:    "photo." + ext,
		ContentType: contentType,
	}, data); err != nil {
		slog.Error("whatsapp: create image attachment", "error", err)
		return
	}

	if err := h.client.SendTextMessage(ctx, from, "✓ Added to your daily notes"); err != nil {
		slog.Error("whatsapp: send reply", "error", err)
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
