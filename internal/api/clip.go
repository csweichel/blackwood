package api

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"path/filepath"
	"strings"
	"time"

	"github.com/csweichel/blackwood/internal/opengraph"
	"github.com/csweichel/blackwood/internal/storage"
)

// ClipHandler handles POST /api/clip requests to clip web pages into daily notes.
type ClipHandler struct {
	store *storage.Store
}

// NewClipHandler creates a new ClipHandler.
func NewClipHandler(store *storage.Store) *ClipHandler {
	return &ClipHandler{store: store}
}

type clipRequest struct {
	URL string `json:"url"`
}

type clipResponse struct {
	Date string `json:"date"`
}

func (h *ClipHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req clipRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid JSON body", http.StatusBadRequest)
		return
	}

	parsed, err := url.Parse(req.URL)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		http.Error(w, "invalid URL", http.StatusBadRequest)
		return
	}

	ctx := r.Context()

	card, err := opengraph.Fetch(ctx, req.URL)
	if err != nil {
		slog.Warn("opengraph fetch failed, using URL only", "url", req.URL, "error", err)
	}

	today := UserTimezoneNowDate(ctx, h.store)
	note, err := h.store.GetOrCreateDailyNote(ctx, today)
	if err != nil {
		slog.Error("get or create daily note", "error", err)
		http.Error(w, "failed to access daily note", http.StatusInternalServerError)
		return
	}

	// Create an entry to track this clip.
	entry := &storage.Entry{
		DailyNoteID: note.ID,
		Type:        "webclip",
		RawContent:  req.URL,
		Source:      "web",
	}

	// Build the markdown card.
	var md strings.Builder

	if card != nil {
		// Download og:image if present.
		var imageRef string
		if card.Image != "" {
			if imgData, contentType, err := downloadImage(ctx, card.Image); err == nil {
				ext := imageExtFromContentType(contentType)
				att := &storage.Attachment{
					EntryID:     "", // set after entry creation
					Filename:    "og-clip" + ext,
					ContentType: contentType,
				}
				// We need the entry ID first, so create the entry now.
				if err := h.store.CreateEntry(ctx, entry); err != nil {
					slog.Error("create entry", "error", err)
					http.Error(w, "failed to create entry", http.StatusInternalServerError)
					return
				}
				att.EntryID = entry.ID
				if err := h.store.CreateAttachment(ctx, att, imgData, today); err != nil {
					slog.Warn("store og:image", "error", err)
				} else {
					imageRef = fmt.Sprintf("![](%s)", filepath.Base(att.StoragePath))
				}
			} else {
				slog.Warn("download og:image", "url", card.Image, "error", err)
			}
		}

		// If entry wasn't created yet (no image or image download failed), create it now.
		if entry.ID == "" {
			if err := h.store.CreateEntry(ctx, entry); err != nil {
				slog.Error("create entry", "error", err)
				http.Error(w, "failed to create entry", http.StatusInternalServerError)
				return
			}
		}

		// Format: > **Title — SiteName**
		title := card.Title
		if title != "" && card.SiteName != "" {
			title = title + " — " + card.SiteName
		}
		if title != "" {
			fmt.Fprintf(&md, "**%s**\n\n", title)
		}
		if card.Description != "" {
			fmt.Fprintf(&md, "%s\n\n", card.Description)
		}
		if imageRef != "" {
			fmt.Fprintf(&md, "%s\n\n", imageRef)
		}

		linkURL := card.URL
		if linkURL == "" {
			linkURL = req.URL
		}
		domain := domainAndPath(linkURL)
		fmt.Fprintf(&md, "[%s](%s)\n", domain, linkURL)
	} else {
		// No OG metadata — just link the URL.
		if entry.ID == "" {
			if err := h.store.CreateEntry(ctx, entry); err != nil {
				slog.Error("create entry", "error", err)
				http.Error(w, "failed to create entry", http.StatusInternalServerError)
				return
			}
		}
		domain := domainAndPath(req.URL)
		fmt.Fprintf(&md, "[%s](%s)\n", domain, req.URL)
	}

	content := md.String()
	entry.Content = content
	// Update the entry content in the DB (CreateEntry already inserted it).

	// Append to the Links section of the daily note.
	if err := h.store.AppendToSection(ctx, note.ID, "# Links", "\n\n---\n*Clipped*\n\n"+content+"\n"); err != nil {
		slog.Error("append daily note content", "error", err)
		http.Error(w, "failed to update daily note", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(clipResponse{Date: today})
}

// downloadImage fetches an image URL and returns the body bytes and content type.
func downloadImage(ctx context.Context, imageURL string) ([]byte, string, error) {
	client := &http.Client{Timeout: 10 * time.Second}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, imageURL, nil)
	if err != nil {
		return nil, "", err
	}
	req.Header.Set("User-Agent", "Blackwood/1.0 (web clipper)")

	resp, err := client.Do(req)
	if err != nil {
		return nil, "", err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, "", fmt.Errorf("image fetch status %d", resp.StatusCode)
	}

	// Limit to 10 MB.
	data, err := io.ReadAll(io.LimitReader(resp.Body, 10<<20))
	if err != nil {
		return nil, "", err
	}

	ct := resp.Header.Get("Content-Type")
	if ct == "" {
		ct = "image/jpeg"
	}
	return data, ct, nil
}

// imageExtFromContentType returns a file extension for common image content types.
func imageExtFromContentType(ct string) string {
	switch {
	case strings.Contains(ct, "png"):
		return ".png"
	case strings.Contains(ct, "gif"):
		return ".gif"
	case strings.Contains(ct, "webp"):
		return ".webp"
	case strings.Contains(ct, "svg"):
		return ".svg"
	default:
		return ".jpg"
	}
}

// domainAndPath returns "domain.com/path" for display in the link text.
func domainAndPath(rawURL string) string {
	u, err := url.Parse(rawURL)
	if err != nil {
		return rawURL
	}
	display := u.Host + u.Path
	display = strings.TrimSuffix(display, "/")
	return display
}
