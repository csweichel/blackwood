package api

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/csweichel/blackwood/internal/codex"
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

type searchEngine interface {
	Available() bool
	UnavailableReason() string
	Search(ctx context.Context, query string, limit int) ([]codex.SearchResult, error)
}

// ServeSearch returns an HTTP handler for GET /api/search?q=...&limit=20.
// It performs Codex-backed search.
func ServeSearch(engine searchEngine) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if engine == nil || !engine.Available() {
			http.Error(w, "search is not available: "+unavailableReason(engine), http.StatusServiceUnavailable)
			return
		}
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

		results, err := engine.Search(r.Context(), query, limit)
		if err != nil {
			slog.Error("search failed", "query", query, "error", err)
			http.Error(w, "search failed", http.StatusInternalServerError)
			return
		}

		out := make([]searchResult, 0, len(results))
		for _, r := range results {
			out = append(out, searchResult{
				EntryID: r.EntryID,
				Date:    r.Date,
				Snippet: r.Snippet,
				Score:   r.Score,
			})
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(searchResponse{Results: out})
	}
}
