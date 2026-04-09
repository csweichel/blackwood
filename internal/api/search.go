package api

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/csweichel/blackwood/internal/index"
	"github.com/csweichel/blackwood/internal/storage"
)

type searchResult struct {
	EntryID string  `json:"entry_id"`
	Date    string  `json:"date"`
	Snippet string  `json:"snippet"`
	Score   float64 `json:"score"`
}

type searchResponse struct {
	Results []searchResult `json:"results"`
}

// ServeSearch returns an HTTP handler for GET /api/search?q=...&limit=20.
// It performs semantic search and enriches results with dates.
func ServeSearch(store *storage.Store, idx index.Indexer) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		query := r.URL.Query().Get("q")
		if query == "" {
			http.Error(w, "q parameter is required", http.StatusBadRequest)
			return
		}

		limit := 20
		if l := r.URL.Query().Get("limit"); l != "" {
			if parsed, err := strconv.Atoi(l); err == nil && parsed > 0 {
				limit = parsed
			}
		}

		results, err := idx.Search(r.Context(), query, limit)
		if err != nil {
			slog.Error("search failed", "query", query, "error", err)
			http.Error(w, "search failed", http.StatusInternalServerError)
			return
		}

		out := make([]searchResult, 0, len(results))
		for _, r := range results {
			date := lookupEntryDate(store, r.EntryID)
			out = append(out, searchResult{
				EntryID: r.EntryID,
				Date:    date,
				Snippet: r.Snippet,
				Score:   r.Score,
			})
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(searchResponse{Results: out})
	}
}

// lookupEntryDate finds the date for an entry by looking up its daily note.
func lookupEntryDate(store *storage.Store, entryID string) string {
	ctx := context.Background()
	entry, err := store.GetEntry(ctx, entryID)
	if err != nil {
		return ""
	}
	note, err := store.GetDailyNote(ctx, entry.DailyNoteID)
	if err != nil {
		return ""
	}
	return note.Date
}
