package auth

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"fmt"
	"net/http"
	"time"

	"github.com/csweichel/blackwood/internal/storage"
)

const (
	// SessionCookieName is the name of the session cookie.
	SessionCookieName = "blackwood_session"

	// SessionLifetime is how long a session remains valid.
	SessionLifetime = 30 * 24 * time.Hour // 30 days
)

// generateToken returns a cryptographically random 32-byte hex-encoded token.
func generateToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("generate session token: %w", err)
	}
	return hex.EncodeToString(b), nil
}

// CreateSession generates a new session token, stores it, and returns it.
func CreateSession(ctx context.Context, store *storage.Store) (string, error) {
	token, err := generateToken()
	if err != nil {
		return "", err
	}
	expiresAt := time.Now().Add(SessionLifetime)
	if err := store.CreateSession(ctx, token, expiresAt); err != nil {
		return "", fmt.Errorf("store session: %w", err)
	}
	return token, nil
}

// ValidateSession checks whether a session token exists and is not expired.
func ValidateSession(ctx context.Context, store *storage.Store, token string) bool {
	if token == "" {
		return false
	}
	expiresAt, err := store.GetSession(ctx, token)
	if err == sql.ErrNoRows {
		return false
	}
	if err != nil {
		return false
	}
	return time.Now().Before(expiresAt)
}

// GetSessionToken extracts the session token from the request cookie.
func GetSessionToken(r *http.Request) string {
	c, err := r.Cookie(SessionCookieName)
	if err != nil {
		return ""
	}
	return c.Value
}
