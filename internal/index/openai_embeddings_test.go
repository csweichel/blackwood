package index

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

func TestOpenAIEmbed_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer test-key" {
			t.Errorf("expected Authorization 'Bearer test-key', got %q", got)
		}

		var req embeddingRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decoding request: %v", err)
		}
		if req.Model != defaultEmbeddingModel {
			t.Errorf("expected model %q, got %q", defaultEmbeddingModel, req.Model)
		}
		if len(req.Input) != 1 || req.Input[0] != "hello world" {
			t.Errorf("unexpected input: %v", req.Input)
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(embeddingResponse{
			Data: []embeddingData{
				{Embedding: []float32{0.1, 0.2, 0.3}, Index: 0},
			},
		})
	}))
	defer srv.Close()

	client := NewOpenAIEmbeddingClient("test-key")
	client.endpoint = srv.URL

	emb, err := client.Embed(context.Background(), "hello world")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(emb) != 3 {
		t.Fatalf("expected 3 dimensions, got %d", len(emb))
	}
	if emb[0] != 0.1 || emb[1] != 0.2 || emb[2] != 0.3 {
		t.Errorf("unexpected embedding values: %v", emb)
	}
}

func TestOpenAIEmbedBatch_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req embeddingRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decoding request: %v", err)
		}
		if len(req.Input) != 2 {
			t.Errorf("expected 2 inputs, got %d", len(req.Input))
		}

		w.Header().Set("Content-Type", "application/json")
		// Return in reverse order to test index-based reordering.
		_ = json.NewEncoder(w).Encode(embeddingResponse{
			Data: []embeddingData{
				{Embedding: []float32{0.4, 0.5, 0.6}, Index: 1},
				{Embedding: []float32{0.1, 0.2, 0.3}, Index: 0},
			},
		})
	}))
	defer srv.Close()

	client := NewOpenAIEmbeddingClient("test-key")
	client.endpoint = srv.URL

	results, err := client.EmbedBatch(context.Background(), []string{"first", "second"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}
	// Verify ordering matches input, not response order.
	if results[0][0] != 0.1 {
		t.Errorf("expected first result[0]=0.1, got %f", results[0][0])
	}
	if results[1][0] != 0.4 {
		t.Errorf("expected second result[0]=0.4, got %f", results[1][0])
	}
}

func TestOpenAIEmbed_RetryOn429(t *testing.T) {
	var attempts atomic.Int32

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := attempts.Add(1)
		if n < 3 {
			w.WriteHeader(http.StatusTooManyRequests)
			_, _ = w.Write([]byte(`{"error": {"message": "rate limited", "type": "rate_limit"}}`))
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(embeddingResponse{
			Data: []embeddingData{
				{Embedding: []float32{1.0}, Index: 0},
			},
		})
	}))
	defer srv.Close()

	client := NewOpenAIEmbeddingClient("test-key")
	client.endpoint = srv.URL

	emb, err := client.Embed(context.Background(), "test")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(emb) != 1 || emb[0] != 1.0 {
		t.Errorf("unexpected embedding: %v", emb)
	}
	if got := attempts.Load(); got != 3 {
		t.Errorf("expected 3 attempts, got %d", got)
	}
}

func TestOpenAIEmbed_Non429_4xxNotRetried(t *testing.T) {
	var attempts atomic.Int32

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts.Add(1)
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error": {"message": "bad request", "type": "invalid_request_error"}}`))
	}))
	defer srv.Close()

	client := NewOpenAIEmbeddingClient("test-key")
	client.endpoint = srv.URL

	_, err := client.Embed(context.Background(), "test")
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

func TestOpenAIEmbed_ContextCancellation(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-r.Context().Done()
	}))
	defer srv.Close()

	client := NewOpenAIEmbeddingClient("test-key")
	client.endpoint = srv.URL

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := client.Embed(ctx, "test")
	if err == nil {
		t.Fatal("expected error from cancelled context, got nil")
	}
	if !strings.Contains(err.Error(), "cancel") {
		t.Errorf("expected cancellation error, got %q", err.Error())
	}
}

func TestOpenAIEmbedBatch_EmptyInput(t *testing.T) {
	client := NewOpenAIEmbeddingClient("test-key")
	// No server needed — empty input should return immediately.
	results, err := client.EmbedBatch(context.Background(), nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if results != nil {
		t.Errorf("expected nil for empty input, got %v", results)
	}
}
