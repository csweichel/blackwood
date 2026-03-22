package whatsapp

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestVerification_Success(t *testing.T) {
	h := NewWebhookHandler(WebhookConfig{
		VerifyToken: "my-secret",
	}, nil, nil, nil)

	req := httptest.NewRequest(http.MethodGet,
		"/api/webhooks/whatsapp?hub.mode=subscribe&hub.verify_token=my-secret&hub.challenge=challenge123",
		nil)
	w := httptest.NewRecorder()

	h.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", w.Code, http.StatusOK)
	}
	body := w.Body.String()
	if body != "challenge123" {
		t.Errorf("body = %q, want %q", body, "challenge123")
	}
}

func TestVerification_WrongToken(t *testing.T) {
	h := NewWebhookHandler(WebhookConfig{
		VerifyToken: "my-secret",
	}, nil, nil, nil)

	req := httptest.NewRequest(http.MethodGet,
		"/api/webhooks/whatsapp?hub.mode=subscribe&hub.verify_token=wrong-token&hub.challenge=challenge123",
		nil)
	w := httptest.NewRecorder()

	h.ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Errorf("status = %d, want %d", w.Code, http.StatusForbidden)
	}
}

func TestVerification_WrongMode(t *testing.T) {
	h := NewWebhookHandler(WebhookConfig{
		VerifyToken: "my-secret",
	}, nil, nil, nil)

	req := httptest.NewRequest(http.MethodGet,
		"/api/webhooks/whatsapp?hub.mode=unsubscribe&hub.verify_token=my-secret&hub.challenge=challenge123",
		nil)
	w := httptest.NewRecorder()

	h.ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Errorf("status = %d, want %d", w.Code, http.StatusForbidden)
	}
}

func TestSignatureVerification(t *testing.T) {
	h := &WebhookHandler{appSecret: "test-secret"}

	body := []byte(`{"test": true}`)
	mac := hmac.New(sha256.New, []byte("test-secret"))
	mac.Write(body)
	validSig := "sha256=" + hex.EncodeToString(mac.Sum(nil))

	if !h.verifySignature(body, validSig) {
		t.Error("valid signature rejected")
	}
	if h.verifySignature(body, "sha256=invalid") {
		t.Error("invalid signature accepted")
	}
	if h.verifySignature(body, "invalid-format") {
		t.Error("malformed signature accepted")
	}
}

func TestPayloadParsing(t *testing.T) {
	payload := webhookPayload{
		Object: "whatsapp_business_account",
		Entry: []webhookEntry{
			{
				Changes: []webhookChange{
					{
						Value: webhookValue{
							Messages: []webhookMessage{
								{
									From: "1234567890",
									Type: "text",
									Text: &messageText{Body: "Hello world"},
								},
								{
									From:  "1234567890",
									Type:  "audio",
									Audio: &messageMedia{ID: "audio123", MimeType: "audio/ogg"},
								},
								{
									From:  "1234567890",
									Type:  "image",
									Image: &messageImage{ID: "img123", MimeType: "image/jpeg", Caption: "A photo"},
								},
							},
						},
					},
				},
			},
		},
	}

	data, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var parsed webhookPayload
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	msgs := parsed.Entry[0].Changes[0].Value.Messages
	if len(msgs) != 3 {
		t.Fatalf("messages count = %d, want 3", len(msgs))
	}

	if msgs[0].Type != "text" || msgs[0].Text.Body != "Hello world" {
		t.Errorf("text message: type=%q body=%q", msgs[0].Type, msgs[0].Text.Body)
	}
	if msgs[1].Type != "audio" || msgs[1].Audio.ID != "audio123" {
		t.Errorf("audio message: type=%q id=%q", msgs[1].Type, msgs[1].Audio.ID)
	}
	if msgs[2].Type != "image" || msgs[2].Image.ID != "img123" {
		t.Errorf("image message: type=%q id=%q", msgs[2].Type, msgs[2].Image.ID)
	}
}

func TestIncomingPost_InvalidSignature(t *testing.T) {
	h := NewWebhookHandler(WebhookConfig{
		VerifyToken: "token",
		AppSecret:   "secret",
	}, nil, nil, nil)

	body := `{"object":"whatsapp_business_account","entry":[]}`
	req := httptest.NewRequest(http.MethodPost, "/api/webhooks/whatsapp", strings.NewReader(body))
	req.Header.Set("X-Hub-Signature-256", "sha256=invalid")
	w := httptest.NewRecorder()

	h.ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Errorf("status = %d, want %d", w.Code, http.StatusForbidden)
	}
}

func TestIncomingPost_ValidSignature(t *testing.T) {
	secret := "test-secret"
	h := NewWebhookHandler(WebhookConfig{
		VerifyToken: "token",
		AppSecret:   secret,
	}, nil, nil, nil)

	body := `{"object":"whatsapp_business_account","entry":[]}`
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(body))
	sig := "sha256=" + hex.EncodeToString(mac.Sum(nil))

	req := httptest.NewRequest(http.MethodPost, "/api/webhooks/whatsapp", strings.NewReader(body))
	req.Header.Set("X-Hub-Signature-256", sig)
	w := httptest.NewRecorder()

	h.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", w.Code, http.StatusOK)
	}
}

func TestIncomingPost_NoSignatureCheck(t *testing.T) {
	// When appSecret is empty, signature verification is skipped.
	h := NewWebhookHandler(WebhookConfig{
		VerifyToken: "token",
	}, nil, nil, nil)

	body := `{"object":"whatsapp_business_account","entry":[]}`
	req := httptest.NewRequest(http.MethodPost, "/api/webhooks/whatsapp", strings.NewReader(body))
	w := httptest.NewRecorder()

	h.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", w.Code, http.StatusOK)
	}
}

func TestMethodNotAllowed(t *testing.T) {
	h := NewWebhookHandler(WebhookConfig{}, nil, nil, nil)

	req := httptest.NewRequest(http.MethodPut, "/api/webhooks/whatsapp", nil)
	w := httptest.NewRecorder()

	h.ServeHTTP(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("status = %d, want %d", w.Code, http.StatusMethodNotAllowed)
	}
}

func TestFormatFromMime(t *testing.T) {
	tests := []struct {
		mime string
		want string
	}{
		{"audio/ogg", "ogg"},
		{"audio/ogg; codecs=opus", "ogg"},
		{"audio/mpeg", "mp3"},
		{"audio/mp3", "mp3"},
		{"audio/mp4", "m4a"},
		{"audio/m4a", "m4a"},
		{"audio/wav", "wav"},
		{"audio/webm", "webm"},
		{"audio/unknown", "ogg"},
	}

	for _, tt := range tests {
		got := formatFromMime(tt.mime)
		if got != tt.want {
			t.Errorf("formatFromMime(%q) = %q, want %q", tt.mime, got, tt.want)
		}
	}
}

func TestTextMessageProcessing(t *testing.T) {
	// This test verifies the full text message flow using a real SQLite store
	// and a mock WhatsApp API server.
	var sentReply sendMessageRequest
	apiSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/messages") {
			body, _ := io.ReadAll(r.Body)
			_ = json.Unmarshal(body, &sentReply)
			w.WriteHeader(http.StatusOK)
			return
		}
		http.NotFound(w, r)
	}))
	defer apiSrv.Close()

	store := newTestStore(t)
	defer func() { _ = store.Close() }()

	h := NewWebhookHandler(WebhookConfig{
		VerifyToken:   "token",
		AccessToken:   "test-token",
		PhoneNumberID: "phone123",
	}, store, nil, nil)
	// Override the client's base URL to point to our test server.
	h.client.baseURL = apiSrv.URL

	payload := webhookPayload{
		Object: "whatsapp_business_account",
		Entry: []webhookEntry{{
			Changes: []webhookChange{{
				Value: webhookValue{
					Messages: []webhookMessage{{
						From: "4915123456",
						Type: "text",
						Text: &messageText{Body: "Test note content"},
					}},
				},
			}},
		}},
	}
	body, _ := json.Marshal(payload)

	req := httptest.NewRequest(http.MethodPost, "/api/webhooks/whatsapp", strings.NewReader(string(body)))
	w := httptest.NewRecorder()

	h.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}

	// Verify a reply was sent.
	if sentReply.To != "4915123456" {
		t.Errorf("reply to = %q, want %q", sentReply.To, "4915123456")
	}
}
