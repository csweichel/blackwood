package api

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/csweichel/blackwood/internal/storage"
)

type summarizeResponse struct {
	Summary string `json:"summary"`
}

type summaryEngine interface {
	Available() bool
	UnavailableReason() string
	Summarize(ctx context.Context, content string) (string, error)
}

// ServeSummarize returns an HTTP handler for POST /api/daily-notes/{date}/summarize.
// It generates an AI summary of the note and writes it into the "# Summary" section.
func ServeSummarize(store *storage.Store, engine summaryEngine) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		if engine == nil || !engine.Available() {
			http.Error(w, "summarization is not available: "+unavailableReason(engine), http.StatusServiceUnavailable)
			return
		}

		date := r.PathValue("date")
		if date == "" {
			http.Error(w, "missing date", http.StatusBadRequest)
			return
		}

		ctx := r.Context()

		note, err := store.GetDailyNoteByDate(ctx, date)
		if err != nil {
			http.Error(w, "note not found", http.StatusNotFound)
			return
		}

		if note.Content == "" {
			http.Error(w, "note has no content to summarize", http.StatusBadRequest)
			return
		}

		summary, err := engine.Summarize(ctx, note.Content)
		if err != nil {
			slog.Error("summarize failed", "date", date, "error", err)
			http.Error(w, "summarization failed", http.StatusInternalServerError)
			return
		}

		if err := store.SetSection(ctx, note.ID, "# Summary", summary+"\n"); err != nil {
			slog.Error("set summary section", "date", date, "error", err)
			http.Error(w, "failed to write summary", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(summarizeResponse{Summary: summary})
	}
}
