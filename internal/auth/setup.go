package auth

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"

	"github.com/csweichel/blackwood/gen/blackwood/v1/blackwoodv1connect"
	"github.com/csweichel/blackwood/internal/storage"
)

// SetupConfig configures the auth system.
type SetupConfig struct {
	Store  *storage.Store
	UseTLS bool
}

// Setup initialises the auth system: registers the AuthService Connect
// handler on mux and returns a middleware that enforces authentication.
//
// The middleware queries the store on each request to determine whether
// setup is still required, so no mode-swapping is needed.
//
// Both the production server and tests use this function.
func Setup(cfg SetupConfig, mux *http.ServeMux) (func(http.Handler) http.Handler, error) {
	rl := NewRateLimiter()
	handler := NewHandler(cfg.Store, rl, cfg.UseTLS)
	path, httpHandler := blackwoodv1connect.NewAuthServiceHandler(handler)
	mux.Handle(path, httpHandler)

	return Middleware(cfg.Store), nil
}

// Cleanup removes any stored TOTP secret and sessions. Call this on
// startup when TOTP is disabled to avoid leaving stale credentials.
func Cleanup(store *storage.Store) error {
	secret, err := store.GetTOTPSecret(context.Background())
	if err != nil {
		return fmt.Errorf("check TOTP secret: %w", err)
	}
	if secret == "" {
		return nil
	}
	if err := store.DeleteTOTPSecret(context.Background()); err != nil {
		return fmt.Errorf("delete TOTP secret: %w", err)
	}
	if err := store.DeleteAllSessions(context.Background()); err != nil {
		return fmt.Errorf("delete sessions: %w", err)
	}
	slog.Info("TOTP disabled, removed stored secret and sessions")
	return nil
}
