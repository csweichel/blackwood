package api

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"time"

	"github.com/csweichel/blackwood/internal/storage"
)

type setLocationRequest struct {
	EntryID      string  `json:"entry_id"`
	Latitude     float64 `json:"latitude"`
	Longitude    float64 `json:"longitude"`
	LocationName string  `json:"location_name,omitempty"`
}

type setLocationResponse struct {
	OK           bool   `json:"ok"`
	LocationName string `json:"location_name,omitempty"`
}

// ServeSetLocation returns an HTTP handler for POST /api/entries/location.
// It stores latitude, longitude, and location name in the entry's metadata.
// If no location_name is provided, it reverse-geocodes the coordinates via
// OpenStreetMap Nominatim.
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

		// Reverse-geocode if no location name was provided.
		locationName := req.LocationName
		if locationName == "" {
			if name, err := reverseGeocode(ctx, req.Latitude, req.Longitude); err != nil {
				slog.Warn("reverse geocode failed", "lat", req.Latitude, "lon", req.Longitude, "error", err)
			} else {
				locationName = name
			}
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
		if locationName != "" {
			meta["location_name"] = locationName
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
		_ = json.NewEncoder(w).Encode(setLocationResponse{OK: true, LocationName: locationName})
	}
}

// nominatimResponse is the subset of the Nominatim reverse geocoding response we use.
type nominatimResponse struct {
	DisplayName string           `json:"display_name"`
	Address     nominatimAddress `json:"address"`
}

type nominatimAddress struct {
	HouseNumber  string `json:"house_number"`
	Road         string `json:"road"`
	City         string `json:"city"`
	Town         string `json:"town"`
	Village      string `json:"village"`
	Municipality string `json:"municipality"`
}

// reverseGeocode resolves lat/lon to a short human-readable address via
// OpenStreetMap Nominatim. Returns the best short address it can build.
func reverseGeocode(ctx context.Context, lat, lon float64) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	url := fmt.Sprintf(
		"https://nominatim.openstreetmap.org/reverse?lat=%f&lon=%f&format=json&zoom=18&addressdetails=1",
		lat, lon,
	)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", "Blackwood/1.0")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("nominatim returned HTTP %d", resp.StatusCode)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, 64<<10))
	if err != nil {
		return "", err
	}

	var nr nominatimResponse
	if err := json.Unmarshal(body, &nr); err != nil {
		return "", err
	}

	// Build a short address from the most useful fields.
	a := nr.Address
	var parts []string
	if a.Road != "" {
		if a.HouseNumber != "" {
			parts = append(parts, a.Road+" "+a.HouseNumber)
		} else {
			parts = append(parts, a.Road)
		}
	}
	city := a.City
	if city == "" {
		city = a.Town
	}
	if city == "" {
		city = a.Village
	}
	if city == "" {
		city = a.Municipality
	}
	if city != "" {
		parts = append(parts, city)
	}

	if len(parts) > 0 {
		result := parts[0]
		for _, p := range parts[1:] {
			result += ", " + p
		}
		return result, nil
	}

	return nr.DisplayName, nil
}
