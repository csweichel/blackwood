package auth

import (
	"encoding/json"
	"net/http"

	"github.com/csweichel/blackwood/internal/storage"
)

// TokenHandler handles POST /auth/token — a plain HTTP endpoint that
// accepts a TOTP code and returns a session token in the response body.
// Intended for API clients (browser extensions, CLI tools) that cannot
// use HttpOnly session cookies.
type TokenHandler struct {
	store       *storage.Store
	rateLimiter *RateLimiter
}

// NewTokenHandler creates a new TokenHandler.
func NewTokenHandler(store *storage.Store, rateLimiter *RateLimiter) *TokenHandler {
	return &TokenHandler{store: store, rateLimiter: rateLimiter}
}

type tokenRequest struct {
	Code string `json:"code"`
}

type tokenResponse struct {
	OK    bool   `json:"ok"`
	Token string `json:"token,omitempty"`
	Error string `json:"error,omitempty"`
}

func (h *TokenHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	ip := clientIPFromHeader(r.Header, r.RemoteAddr)
	if !h.rateLimiter.Allow(ip) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusTooManyRequests)
		_ = json.NewEncoder(w).Encode(tokenResponse{Error: "too many attempts, try again later"})
		return
	}

	var req tokenRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Code == "" {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(tokenResponse{Error: "code is required"})
		return
	}

	secret, err := h.store.GetTOTPSecret(r.Context())
	if err != nil || secret == "" {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(tokenResponse{Error: "TOTP not configured"})
		return
	}

	if !ValidateCode(secret, req.Code) {
		h.rateLimiter.RecordFailure(ip)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		_ = json.NewEncoder(w).Encode(tokenResponse{Error: "invalid code"})
		return
	}

	h.rateLimiter.Reset(ip)
	_ = h.store.CleanExpiredSessions(r.Context())

	token, err := CreateSession(r.Context(), h.store)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(tokenResponse{Error: "failed to create session"})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(tokenResponse{OK: true, Token: token})
}
