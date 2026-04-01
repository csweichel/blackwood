package auth

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/csweichel/blackwood/internal/storage"
)

// storeWithSecret creates a test store with a TOTP secret already provisioned.
func storeWithSecret(t *testing.T) *storage.Store {
	t.Helper()
	s := newTestStore(t)
	if err := s.SaveTOTPSecret(context.Background(), "TESTSECRET"); err != nil {
		t.Fatalf("SaveTOTPSecret: %v", err)
	}
	return s
}

func TestMiddleware_ValidSession(t *testing.T) {
	store := storeWithSecret(t)
	ctx := context.Background()

	token, err := CreateSession(ctx, store)
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}

	mw := Middleware(store)
	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("GET", "/day/2025-01-15", nil)
	req.AddCookie(&http.Cookie{Name: SessionCookieName, Value: token})
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusOK)
	}
}

func TestMiddleware_NoCookie_BrowserPassesThrough(t *testing.T) {
	store := storeWithSecret(t)

	mw := Middleware(store)
	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	// Non-API (browser) requests pass through so the SPA can render the login page.
	req := httptest.NewRequest("GET", "/day/2025-01-15", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusOK)
	}
}

func TestMiddleware_NoCookie_APIReturns401(t *testing.T) {
	store := storeWithSecret(t)

	mw := Middleware(store)
	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("POST", "/blackwood.v1.DailyNotesService/GetDailyNote", nil)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusUnauthorized)
	}
}

func TestMiddleware_HealthCheckExempt(t *testing.T) {
	store := storeWithSecret(t)

	mw := Middleware(store)
	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("POST", "/blackwood.v1.HealthService/Check", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusOK)
	}
}

func TestMiddleware_AuthRoutesExempt(t *testing.T) {
	store := storeWithSecret(t)

	mw := Middleware(store)
	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	// Legacy /auth/ path.
	req := httptest.NewRequest("GET", "/auth/login", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Errorf("/auth/login: status = %d, want %d", rec.Code, http.StatusOK)
	}

	// Connect AuthService path.
	req = httptest.NewRequest("POST", "/blackwood.v1.AuthService/Login", nil)
	req.Header.Set("Content-Type", "application/json")
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Errorf("AuthService/Login: status = %d, want %d", rec.Code, http.StatusOK)
	}
}

func TestMiddleware_SetupMode_BrowserPassesThrough(t *testing.T) {
	store := newTestStore(t) // no secret = setup mode

	mw := Middleware(store)
	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	// Non-API requests pass through so the SPA can render the setup page.
	req := httptest.NewRequest("GET", "/day/2025-01-15", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusOK)
	}
}

func TestMiddleware_SetupMode_API401(t *testing.T) {
	store := newTestStore(t) // no secret = setup mode

	mw := Middleware(store)
	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("POST", "/blackwood.v1.DailyNotesService/GetDailyNote", nil)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusUnauthorized)
	}
}

func TestMiddleware_ConnectProtocolHeader(t *testing.T) {
	store := storeWithSecret(t)

	mw := Middleware(store)
	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("POST", "/blackwood.v1.ChatService/Chat", nil)
	req.Header.Set("Connect-Protocol-Version", "1")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusUnauthorized)
	}
}
