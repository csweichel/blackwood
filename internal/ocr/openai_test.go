package ocr

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

func TestRecognize_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify request properties.
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer test-key" {
			t.Errorf("expected Authorization 'Bearer test-key', got %q", got)
		}
		if got := r.Header.Get("Content-Type"); got != "application/json" {
			t.Errorf("expected Content-Type 'application/json', got %q", got)
		}

		// Verify request body contains the model.
		var req chatRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decoding request body: %v", err)
		}
		if req.Model != "gpt-4o" {
			t.Errorf("expected model 'gpt-4o', got %q", req.Model)
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(chatResponse{
			Choices: []struct {
				Message struct {
					Content string `json:"content"`
				} `json:"message"`
			}{
				{Message: struct {
					Content string `json:"content"`
				}{Content: "Hello from the note"}},
			},
		})
	}))
	defer srv.Close()

	rec := NewOpenAI("test-key", "gpt-4o", "Extract text")
	rec.endpoint = srv.URL

	result, err := rec.Recognize(context.Background(), []byte("fake-png-data"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != "Hello from the note" {
		t.Errorf("expected 'Hello from the note', got %q", result)
	}
}

func TestRecognize_RetryOn500(t *testing.T) {
	var attempts atomic.Int32

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := attempts.Add(1)
		if n < 3 {
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte(`{"error": {"message": "server error", "type": "server_error"}}`))
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(chatResponse{
			Choices: []struct {
				Message struct {
					Content string `json:"content"`
				} `json:"message"`
			}{
				{Message: struct {
					Content string `json:"content"`
				}{Content: "recovered"}},
			},
		})
	}))
	defer srv.Close()

	rec := NewOpenAI("test-key", "gpt-4o", "Extract text")
	rec.endpoint = srv.URL

	result, err := rec.Recognize(context.Background(), []byte("fake-png-data"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != "recovered" {
		t.Errorf("expected 'recovered', got %q", result)
	}
	if got := attempts.Load(); got != 3 {
		t.Errorf("expected 3 attempts, got %d", got)
	}
}

func TestRecognize_Non429_4xxNotRetried(t *testing.T) {
	var attempts atomic.Int32

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts.Add(1)
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error": {"message": "bad request", "type": "invalid_request_error"}}`))
	}))
	defer srv.Close()

	rec := NewOpenAI("test-key", "gpt-4o", "Extract text")
	rec.endpoint = srv.URL

	_, err := rec.Recognize(context.Background(), []byte("fake-png-data"))
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "400") {
		t.Errorf("expected error to mention 400, got %q", err.Error())
	}
	if got := attempts.Load(); got != 1 {
		t.Errorf("expected 1 attempt (no retry on 400), got %d", got)
	}
}

func TestRecognize_ContextCancellation(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Simulate slow response — the context should cancel before this returns.
		<-r.Context().Done()
	}))
	defer srv.Close()

	rec := NewOpenAI("test-key", "gpt-4o", "Extract text")
	rec.endpoint = srv.URL

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately.

	_, err := rec.Recognize(ctx, []byte("fake-png-data"))
	if err == nil {
		t.Fatal("expected error from cancelled context, got nil")
	}
	if !strings.Contains(err.Error(), "cancel") {
		t.Errorf("expected cancellation error, got %q", err.Error())
	}
}

func TestRecognize_AuthorizationHeader(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth := r.Header.Get("Authorization")
		if auth != "Bearer my-secret-key" {
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte(`{"error": {"message": "invalid key", "type": "auth_error"}}`))
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(chatResponse{
			Choices: []struct {
				Message struct {
					Content string `json:"content"`
				} `json:"message"`
			}{
				{Message: struct {
					Content string `json:"content"`
				}{Content: "ok"}},
			},
		})
	}))
	defer srv.Close()

	rec := NewOpenAI("my-secret-key", "gpt-4o", "Extract text")
	rec.endpoint = srv.URL

	result, err := rec.Recognize(context.Background(), []byte("fake-png-data"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != "ok" {
		t.Errorf("expected 'ok', got %q", result)
	}
}
