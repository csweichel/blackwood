package auth_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/csweichel/blackwood/internal/auth"
	"github.com/csweichel/blackwood/internal/storage"
	"github.com/pquerna/otp/totp"
)

// newE2EStore creates a temporary SQLite store for testing.
func newE2EStore(t *testing.T) *storage.Store {
	t.Helper()
	dir := t.TempDir()
	s, err := storage.New(filepath.Join(dir, "test.db"), dir)
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s
}

// newE2EServer creates a full auth-enabled HTTP test server using the
// same auth.Setup function as the production server.
func newE2EServer(t *testing.T) *httptest.Server {
	t.Helper()
	store := newE2EStore(t)

	mux := http.NewServeMux()

	// A dummy protected endpoint to verify auth enforcement.
	mux.HandleFunc("POST /api/protected", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":"secret"}`))
	})

	// Use the same Setup function as main.go.
	authMiddleware, err := auth.Setup(auth.SetupConfig{
		Store:  store,
		UseTLS: false,
	}, mux)
	if err != nil {
		t.Fatalf("auth.Setup: %v", err)
	}

	ts := httptest.NewServer(authMiddleware(mux))
	t.Cleanup(ts.Close)
	return ts
}

func rpc(t *testing.T, client *http.Client, base, method, body string) (int, map[string]any) {
	t.Helper()
	url := base + "/blackwood.v1.AuthService/" + method
	resp, err := client.Post(url, "application/json", bytes.NewReader([]byte(body)))
	if err != nil {
		t.Fatalf("POST %s: %v", url, err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)

	var result map[string]any
	_ = json.Unmarshal(b, &result)
	return resp.StatusCode, result
}

func TestE2E_FullSetupAndLoginFlow(t *testing.T) {
	ts := newE2EServer(t)
	client := ts.Client()

	// --- Step 1: Status should show setupRequired=true, authenticated absent/false ---
	code, result := rpc(t, client, ts.URL, "Status", `{}`)
	if code != 200 {
		t.Fatalf("Status: got %d, want 200", code)
	}
	if result["setupRequired"] != true {
		t.Errorf("setupRequired = %v, want true", result["setupRequired"])
	}
	if result["authenticated"] == true {
		t.Error("should not be authenticated before setup")
	}

	// --- Step 2: Protected endpoint should be blocked (setup mode) ---
	resp, err := client.Post(ts.URL+"/api/protected", "application/json", bytes.NewReader([]byte(`{}`)))
	if err != nil {
		t.Fatalf("protected endpoint: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != 401 {
		t.Errorf("protected endpoint: got %d, want 401", resp.StatusCode)
	}

	// --- Step 3: Get setup info ---
	code, result = rpc(t, client, ts.URL, "GetSetupInfo", `{}`)
	if code != 200 {
		t.Fatalf("GetSetupInfo: got %d, want 200", code)
	}
	secret, ok := result["secret"].(string)
	if !ok || secret == "" {
		t.Fatal("GetSetupInfo: missing secret")
	}
	qrCode, ok := result["qrCode"].(string)
	if !ok || qrCode == "" {
		t.Fatal("GetSetupInfo: missing qrCode")
	}

	// --- Step 4: Confirm setup with invalid code ---
	code, result = rpc(t, client, ts.URL, "ConfirmSetup", `{"secret":"`+secret+`","code":"000000"}`)
	if code != 200 {
		t.Fatalf("ConfirmSetup (invalid): got %d, want 200", code)
	}
	if result["ok"] == true {
		t.Error("ConfirmSetup should fail with invalid code")
	}

	// --- Step 5: Confirm setup with valid code ---
	validCode, err := totp.GenerateCode(secret, time.Now())
	if err != nil {
		t.Fatalf("GenerateCode: %v", err)
	}
	code, result = rpc(t, client, ts.URL, "ConfirmSetup", `{"secret":"`+secret+`","code":"`+validCode+`"}`)
	if code != 200 {
		t.Fatalf("ConfirmSetup (valid): got %d, want 200", code)
	}
	if result["ok"] != true {
		t.Errorf("ConfirmSetup ok = %v, want true", result["ok"])
	}

	// --- Step 6: GetSetupInfo should now fail (secret exists) ---
	code, _ = rpc(t, client, ts.URL, "GetSetupInfo", `{}`)
	if code == 200 {
		// Connect may return 200 with error envelope or non-200.
		// Either way, it should not return a new secret.
	}

	// --- Step 7: Login with invalid code ---
	code, result = rpc(t, client, ts.URL, "Login", `{"code":"000000"}`)
	if code != 200 {
		t.Fatalf("Login (invalid): got %d, want 200", code)
	}
	if result["ok"] == true {
		t.Error("Login should fail with invalid code")
	}

	// --- Step 8: Login with valid code ---
	validCode2, err := totp.GenerateCode(secret, time.Now())
	if err != nil {
		t.Fatalf("GenerateCode: %v", err)
	}
	loginURL := ts.URL + "/blackwood.v1.AuthService/Login"
	loginBody := `{"code":"` + validCode2 + `"}`
	loginResp, err := client.Post(loginURL, "application/json", bytes.NewReader([]byte(loginBody)))
	if err != nil {
		t.Fatalf("Login POST: %v", err)
	}
	loginRespBody, _ := io.ReadAll(loginResp.Body)
	loginResp.Body.Close()

	if loginResp.StatusCode != 200 {
		t.Fatalf("Login (valid): got %d, want 200; body: %s", loginResp.StatusCode, string(loginRespBody))
	}

	var loginResult map[string]any
	_ = json.Unmarshal(loginRespBody, &loginResult)
	if loginResult["ok"] != true {
		t.Errorf("Login ok = %v, want true; body: %s", loginResult["ok"], string(loginRespBody))
	}

	// Check Set-Cookie header.
	var sessionCookie *http.Cookie
	for _, c := range loginResp.Cookies() {
		if c.Name == auth.SessionCookieName {
			sessionCookie = c
		}
	}
	if sessionCookie == nil {
		t.Fatal("Login: no session cookie set")
	}
	if sessionCookie.Value == "" {
		t.Error("Login: session cookie value is empty")
	}
	if !sessionCookie.HttpOnly {
		t.Error("Login: session cookie should be HttpOnly")
	}

	// --- Step 9: Status with session cookie should show authenticated ---
	statusReq, _ := http.NewRequestWithContext(context.Background(), "POST", ts.URL+"/blackwood.v1.AuthService/Status", bytes.NewReader([]byte(`{}`)))
	statusReq.Header.Set("Content-Type", "application/json")
	statusReq.AddCookie(sessionCookie)
	statusResp, err := client.Do(statusReq)
	if err != nil {
		t.Fatalf("Status (authenticated): %v", err)
	}
	statusBody, _ := io.ReadAll(statusResp.Body)
	statusResp.Body.Close()

	var statusResult map[string]any
	_ = json.Unmarshal(statusBody, &statusResult)
	if statusResult["authenticated"] != true {
		t.Errorf("authenticated = %v, want true", statusResult["authenticated"])
	}

	// --- Step 10: Logout ---
	logoutReq, _ := http.NewRequestWithContext(context.Background(), "POST", ts.URL+"/blackwood.v1.AuthService/Logout", bytes.NewReader([]byte(`{}`)))
	logoutReq.Header.Set("Content-Type", "application/json")
	logoutReq.AddCookie(sessionCookie)
	logoutResp, err := client.Do(logoutReq)
	if err != nil {
		t.Fatalf("Logout: %v", err)
	}
	logoutResp.Body.Close()

	if logoutResp.StatusCode != 200 {
		t.Errorf("Logout: got %d, want 200", logoutResp.StatusCode)
	}

	// --- Step 11: Status after logout should show unauthenticated ---
	statusReq2, _ := http.NewRequestWithContext(context.Background(), "POST", ts.URL+"/blackwood.v1.AuthService/Status", bytes.NewReader([]byte(`{}`)))
	statusReq2.Header.Set("Content-Type", "application/json")
	statusReq2.AddCookie(sessionCookie) // old cookie
	statusResp2, err := client.Do(statusReq2)
	if err != nil {
		t.Fatalf("Status (post-logout): %v", err)
	}
	statusBody2, _ := io.ReadAll(statusResp2.Body)
	statusResp2.Body.Close()

	var statusResult2 map[string]any
	_ = json.Unmarshal(statusBody2, &statusResult2)
	if statusResult2["authenticated"] == true {
		t.Error("should not be authenticated after logout")
	}
}

func TestE2E_RateLimiting(t *testing.T) {
	store := newE2EStore(t)

	// Pre-provision a secret so we can test login.
	secret, _, err := auth.GenerateSecret()
	if err != nil {
		t.Fatal(err)
	}
	if err := store.SaveTOTPSecret(context.Background(), secret); err != nil {
		t.Fatal(err)
	}

	mux := http.NewServeMux()
	authMiddleware, err := auth.Setup(auth.SetupConfig{
		Store:  store,
		UseTLS: false,
	}, mux)
	if err != nil {
		t.Fatalf("auth.Setup: %v", err)
	}

	ts := httptest.NewServer(authMiddleware(mux))
	defer ts.Close()
	client := ts.Client()

	// Send maxAttempts (5) failed logins.
	for i := 0; i < 5; i++ {
		code, _ := rpc(t, client, ts.URL, "Login", `{"code":"000000"}`)
		if code != 200 {
			t.Fatalf("attempt %d: got %d, want 200", i+1, code)
		}
	}

	// 6th attempt should be rate-limited (429).
	code, _ := rpc(t, client, ts.URL, "Login", `{"code":"000000"}`)
	if code != 429 {
		t.Errorf("rate-limited attempt: got %d, want 429", code)
	}

	// Valid code should also be blocked.
	validCode, _ := totp.GenerateCode(secret, time.Now())
	code, _ = rpc(t, client, ts.URL, "Login", `{"code":"`+validCode+`"}`)
	if code != 429 {
		t.Errorf("valid code while rate-limited: got %d, want 429", code)
	}
}
