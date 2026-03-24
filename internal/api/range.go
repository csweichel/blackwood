package api

import (
	"encoding/json"
	"net/http"

	"github.com/csweichel/blackwood/internal/storage"
)

type rangeSummary struct {
	Date    string `json:"date"`
	Summary string `json:"summary"`
}

// ServeRangeSummaries returns an HTTP handler for GET /api/daily-notes/range?start=...&end=...
// It returns summaries for each date in the range.
func ServeRangeSummaries(store *storage.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		start := r.URL.Query().Get("start")
		end := r.URL.Query().Get("end")
		if start == "" || end == "" {
			http.Error(w, "start and end query parameters are required", http.StatusBadRequest)
			return
		}

		summaries, err := store.ListSummariesInRange(r.Context(), start, end)
		if err != nil {
			http.Error(w, "failed to list summaries", http.StatusInternalServerError)
			return
		}

		result := make([]rangeSummary, 0, len(summaries))
		for _, s := range summaries {
			result = append(result, rangeSummary{Date: s.Date, Summary: s.Summary})
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(result)
	}
}
