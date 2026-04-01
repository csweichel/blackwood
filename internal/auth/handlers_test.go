package auth

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/csweichel/blackwood/gen/blackwood/v1/blackwoodv1connect"
	"github.com/pquerna/otp/totp"
)

// connectMux creates a test HTTP mux with the AuthService registered.
func connectMux(t *testing.T, h *Handler) *http.ServeMux {
	t.Helper()
	mux := http.NewServeMux()
	path, handler := blackwoodv1connect.NewAuthServiceHandler(h)
	mux.Handle(path, handler)
	return mux
}

// connectPost sends a Connect-style JSON POST to the given mux.
func connectPost(mux http.Handler, procedure string, body string, cookies ...*http.Cookie) *httptest.ResponseRecorder {
	req := httptest.NewRequest("POST", "/blackwood.v1.AuthService/"+procedure, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	for _, c := range cookies {
		req.AddCookie(c)
	}
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	return rec
}

func TestGetSetupInfo_NoSecret(t *testing.T) {
	store := newTestStore(t)
	h := NewHandler(store, NewRateLimiter(), false)
	mux := connectMux(t, h)

	rec := connectPost(mux, "GetSetupInfo", "{}")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body: %s", rec.Code, http.StatusOK, rec.Body.String())
	}

	var resp map[string]string
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp["secret"] == "" {
		t.Error("expected non-empty secret")
	}
	if resp["qrCode"] == "" {
		t.Error("expected non-empty qrCode")
	}
}

func TestGetSetupInfo_SecretExists(t *testing.T) {
	store := newTestStore(t)
	if err := store.SaveTOTPSecret(context.Background(), "EXISTING"); err != nil {
		t.Fatal(err)
	}

	h := NewHandler(store, NewRateLimiter(), false)
	mux := connectMux(t, h)

	rec := connectPost(mux, "GetSetupInfo", "{}")
	// Connect returns errors as HTTP 200 with error body, or non-200 depending on codec.
	// For not_found, Connect-JSON returns a non-OK status or an error envelope.
	if rec.Code == http.StatusOK {
		var resp map[string]any
		if err := json.NewDecoder(rec.Body).Decode(&resp); err == nil {
			if resp["code"] == nil && resp["secret"] != nil {
				t.Error("expected error response when secret already exists")
			}
		}
	}
	// Either non-200 or error envelope is acceptable.
}

func TestConfirmSetup_ValidCode(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	h := NewHandler(store, NewRateLimiter(), false)
	mux := connectMux(t, h)

	secret, _, err := GenerateSecret()
	if err != nil {
		t.Fatal(err)
	}
	code, err := totp.GenerateCode(secret, time.Now())
	if err != nil {
		t.Fatal(err)
	}

	body := fmt.Sprintf(`{"secret":%q,"code":%q}`, secret, code)
	rec := connectPost(mux, "ConfirmSetup", body)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body: %s", rec.Code, http.StatusOK, rec.Body.String())
	}

	var resp map[string]any
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp["ok"] != true {
		t.Errorf("ok = %v, want true", resp["ok"])
	}

	stored, err := store.GetTOTPSecret(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if stored != secret {
		t.Errorf("stored = %q, want %q", stored, secret)
	}

}

func TestConfirmSetup_InvalidCode(t *testing.T) {
	store := newTestStore(t)
	h := NewHandler(store, NewRateLimiter(), false)
	mux := connectMux(t, h)

	secret, _, err := GenerateSecret()
	if err != nil {
		t.Fatal(err)
	}

	body := fmt.Sprintf(`{"secret":%q,"code":"000000"}`, secret)
	rec := connectPost(mux, "ConfirmSetup", body)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body: %s", rec.Code, http.StatusOK, rec.Body.String())
	}

	var resp map[string]any
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	// Proto3 omits false from JSON, so ok may be nil or false.
	if resp["ok"] == true {
		t.Errorf("ok = %v, want false/absent", resp["ok"])
	}
	if resp["error"] == nil || resp["error"] == "" {
		t.Error("expected error message")
	}
}

func TestLogin_ValidCode(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	secret, _, err := GenerateSecret()
	if err != nil {
		t.Fatal(err)
	}
	if err := store.SaveTOTPSecret(ctx, secret); err != nil {
		t.Fatal(err)
	}

	h := NewHandler(store, NewRateLimiter(), false)
	mux := connectMux(t, h)

	code, err := totp.GenerateCode(secret, time.Now())
	if err != nil {
		t.Fatal(err)
	}

	body := fmt.Sprintf(`{"code":%q}`, code)
	rec := connectPost(mux, "Login", body)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body: %s", rec.Code, http.StatusOK, rec.Body.String())
	}

	var resp map[string]any
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp["ok"] != true {
		t.Errorf("ok = %v, want true", resp["ok"])
	}

	// Check session cookie was set via Set-Cookie header.
	cookies := rec.Result().Cookies()
	found := false
	for _, c := range cookies {
		if c.Name == SessionCookieName {
			found = true
			if c.Value == "" {
				t.Error("session cookie value is empty")
			}
			if !c.HttpOnly {
				t.Error("session cookie should be HttpOnly")
			}
		}
	}
	if !found {
		t.Error("session cookie not set")
	}
}

func TestLogin_InvalidCode(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	secret, _, err := GenerateSecret()
	if err != nil {
		t.Fatal(err)
	}
	if err := store.SaveTOTPSecret(ctx, secret); err != nil {
		t.Fatal(err)
	}

	h := NewHandler(store, NewRateLimiter(), false)
	mux := connectMux(t, h)

	rec := connectPost(mux, "Login", `{"code":"000000"}`)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body: %s", rec.Code, http.StatusOK, rec.Body.String())
	}

	var resp map[string]any
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	// Proto3 omits false from JSON.
	if resp["ok"] == true {
		t.Errorf("ok = %v, want false/absent", resp["ok"])
	}
}

func TestLogin_RateLimited(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	secret, _, err := GenerateSecret()
	if err != nil {
		t.Fatal(err)
	}
	if err := store.SaveTOTPSecret(ctx, secret); err != nil {
		t.Fatal(err)
	}

	rl := NewRateLimiter()
	h := NewHandler(store, rl, false)
	mux := connectMux(t, h)

	// Exhaust rate limit.
	for i := 0; i < maxAttempts; i++ {
		connectPost(mux, "Login", `{"code":"000000"}`)
	}

	// Next attempt should fail with resource_exhausted.
	rec := connectPost(mux, "Login", `{"code":"000000"}`)

	// Connect maps resource_exhausted to HTTP 429.
	if rec.Code != http.StatusTooManyRequests {
		t.Errorf("status = %d, want %d; body: %s", rec.Code, http.StatusTooManyRequests, rec.Body.String())
	}
}

func TestLogout(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	token, err := CreateSession(ctx, store)
	if err != nil {
		t.Fatal(err)
	}

	h := NewHandler(store, NewRateLimiter(), false)
	mux := connectMux(t, h)

	rec := connectPost(mux, "Logout", "{}", &http.Cookie{Name: SessionCookieName, Value: token})

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body: %s", rec.Code, http.StatusOK, rec.Body.String())
	}

	if ValidateSession(ctx, store, token) {
		t.Error("session should be invalid after logout")
	}

	// Cookie should be cleared.
	cookies := rec.Result().Cookies()
	for _, c := range cookies {
		if c.Name == SessionCookieName && c.MaxAge > 0 {
			t.Error("session cookie should have negative MaxAge")
		}
	}
}

func TestStatus_Unauthenticated(t *testing.T) {
	store := newTestStore(t)
	h := NewHandler(store, NewRateLimiter(), false)
	mux := connectMux(t, h)

	rec := connectPost(mux, "Status", "{}")

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body: %s", rec.Code, http.StatusOK, rec.Body.String())
	}

	var resp map[string]any
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	// Proto3 omits false from JSON, so authenticated may be nil.
	if resp["authenticated"] == true {
		t.Errorf("authenticated = %v, want false/absent", resp["authenticated"])
	}
	if resp["setupRequired"] != true {
		t.Errorf("setupRequired = %v, want true", resp["setupRequired"])
	}
}
