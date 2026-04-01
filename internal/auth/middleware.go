package auth

import (
	"net/http"
	"strings"

	"github.com/csweichel/blackwood/internal/storage"
)

// Middleware returns an HTTP middleware that enforces TOTP authentication.
// It queries the store on each API request to determine the current auth
// state — no external swapping or flags needed.
//
// Static assets and non-API routes are always allowed through so the
// SPA can load and render its own login/setup pages.
func Middleware(store *storage.Store) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			path := r.URL.Path

			// Always allow auth routes through (Connect service + any legacy paths).
			if strings.HasPrefix(path, "/blackwood.v1.AuthService/") || strings.HasPrefix(path, "/auth/") {
				next.ServeHTTP(w, r)
				return
			}

			// Always allow health check.
			if strings.HasPrefix(path, "/blackwood.v1.HealthService/") {
				next.ServeHTTP(w, r)
				return
			}

			// Non-API requests (static assets, SPA HTML) are always served.
			// The SPA handles auth routing client-side.
			if !isAPIRequest(r) {
				next.ServeHTTP(w, r)
				return
			}

			// --- From here, only API/RPC requests ---

			// Check if TOTP is set up yet.
			secret, _ := store.GetTOTPSecret(r.Context())
			if secret == "" {
				w.Header().Set("Content-Type", "application/json")
				w.Header().Set("X-Auth-Setup-Required", "true")
				http.Error(w, `{"code":"setup_required","message":"TOTP setup required"}`, http.StatusUnauthorized)
				return
			}

			// Validate session cookie.
			token := GetSessionToken(r)
			if ValidateSession(r.Context(), store, token) {
				next.ServeHTTP(w, r)
				return
			}

			// Unauthenticated API request.
			w.Header().Set("Content-Type", "application/json")
			http.Error(w, `{"code":"unauthorized","message":"Authentication required"}`, http.StatusUnauthorized)
		})
	}
}

// isAPIRequest returns true for Connect-go RPC calls and JSON API requests.
func isAPIRequest(r *http.Request) bool {
	ct := r.Header.Get("Content-Type")
	if strings.HasPrefix(ct, "application/json") || strings.HasPrefix(ct, "application/connect") {
		return true
	}
	if r.Header.Get("Connect-Protocol-Version") != "" {
		return true
	}
	// POST to RPC-style paths (e.g. /blackwood.v1.*)
	if r.Method == http.MethodPost && strings.HasPrefix(r.URL.Path, "/blackwood.v1.") {
		return true
	}
	return false
}
