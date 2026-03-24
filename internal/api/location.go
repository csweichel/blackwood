package api

import (
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/csweichel/blackwood/internal/storage"
)

type setLocationRequest struct {
	EntryID      string  `json:"entry_id"`
	Latitude     float64 `json:"latitude"`
	Longitude    float64 `json:"longitude"`
	LocationName string  `json:"location_name,omitempty"`
}

type setLocationResponse struct {
	OK bool `json:"ok"`
}

// ServeSetLocation returns an HTTP handler for POST /api/entries/location.
// It stores latitude, longitude, and optional location name in the entry's metadata.
func ServeSetLocation(store *storage.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		var req setLocationRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid JSON body", http.StatusBadRequest)
			return
		}

		if req.EntryID == "" {
			http.Error(w, "entry_id is required", http.StatusBadRequest)
			return
		}

		ctx := r.Context()
		entry, err := store.GetEntry(ctx, req.EntryID)
		if err != nil {
			http.Error(w, "entry not found", http.StatusNotFound)
			return
		}

		// Parse existing metadata, merge location fields.
		var meta map[string]interface{}
		if entry.Metadata != "" && entry.Metadata != "{}" {
			if err := json.Unmarshal([]byte(entry.Metadata), &meta); err != nil {
				meta = make(map[string]interface{})
			}
		} else {
			meta = make(map[string]interface{})
		}

		meta["latitude"] = req.Latitude
		meta["longitude"] = req.Longitude
		if req.LocationName != "" {
			meta["location_name"] = req.LocationName
		}

		metaBytes, err := json.Marshal(meta)
		if err != nil {
			slog.Error("marshal metadata", "error", err)
			http.Error(w, "failed to encode metadata", http.StatusInternalServerError)
			return
		}

		entry.Metadata = string(metaBytes)
		if err := store.UpdateEntry(ctx, entry); err != nil {
			slog.Error("update entry metadata", "error", err)
			http.Error(w, "failed to update entry", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(setLocationResponse{OK: true})
	}
}
