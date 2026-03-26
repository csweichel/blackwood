package transcribe

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

func TestTranscribe_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if auth := r.Header.Get("Authorization"); auth != "Bearer test-key" {
			t.Errorf("unexpected Authorization header: %s", auth)
		}

		resp := whisperResponse{Text: "hello world"}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	tr := &WhisperTranscriber{
		apiKey:   "test-key",
		endpoint: server.URL,
	}

	text, err := tr.Transcribe(context.Background(), []byte("fake-audio"), "mp3")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if text != "hello world" {
		t.Errorf("expected %q, got %q", "hello world", text)
	}
}

func TestTranscribe_MultipartFormat(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Parse multipart form to verify fields.
		if err := r.ParseMultipartForm(10 << 20); err != nil {
			t.Fatalf("parsing multipart form: %v", err)
		}

		// Check model field.
		model := r.FormValue("model")
		if model != "whisper-1" {
			t.Errorf("expected model %q, got %q", "whisper-1", model)
		}

		// Check file field.
		file, header, err := r.FormFile("file")
		if err != nil {
			t.Fatalf("getting form file: %v", err)
		}
		defer func() { _ = file.Close() }()

		if header.Filename != "audio.m4a" {
			t.Errorf("expected filename %q, got %q", "audio.m4a", header.Filename)
		}
		if got := header.Header.Get("Content-Type"); got != "audio/x-m4a" {
			t.Errorf("expected part Content-Type %q, got %q", "audio/x-m4a", got)
		}

		data, _ := io.ReadAll(file)
		if string(data) != "test-audio-data" {
			t.Errorf("unexpected file content: %s", string(data))
		}

		resp := whisperResponse{Text: "transcribed"}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	tr := &WhisperTranscriber{
		apiKey:   "test-key",
		endpoint: server.URL,
	}

	text, err := tr.Transcribe(context.Background(), []byte("test-audio-data"), "m4a")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if text != "transcribed" {
		t.Errorf("expected %q, got %q", "transcribed", text)
	}
}

func TestTranscribe_RetryOn500(t *testing.T) {
	var attempts atomic.Int32

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := attempts.Add(1)
		if n < 3 {
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte("server error"))
			return
		}
		resp := whisperResponse{Text: "recovered"}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	tr := &WhisperTranscriber{
		apiKey:   "test-key",
		endpoint: server.URL,
	}

	text, err := tr.Transcribe(context.Background(), []byte("audio"), "wav")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if text != "recovered" {
		t.Errorf("expected %q, got %q", "recovered", text)
	}
	if got := attempts.Load(); got != 3 {
		t.Errorf("expected 3 attempts, got %d", got)
	}
}

func TestTranscribe_ContextCancellation(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Block until context is cancelled — the client should abort.
		<-r.Context().Done()
	}))
	defer server.Close()

	tr := &WhisperTranscriber{
		apiKey:   "test-key",
		endpoint: server.URL,
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately.

	_, err := tr.Transcribe(ctx, []byte("audio"), "mp3")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if err != context.Canceled {
		t.Errorf("expected context.Canceled, got %v", err)
	}
}

func TestMimeTypeForFormat(t *testing.T) {
	tests := map[string]string{
		"m4a":  "audio/x-m4a",
		"mp4":  "audio/mp4",
		"mp3":  "audio/mpeg",
		"mpeg": "audio/mpeg",
		"wav":  "audio/wav",
		"ogg":  "audio/ogg",
		"webm": "audio/webm",
		"flac": "audio/flac",
		"foo":  "application/octet-stream",
	}

	for input, want := range tests {
		if got := mimeTypeForFormat(strings.ToUpper(input)); got != want {
			t.Errorf("mimeTypeForFormat(%q) = %q, want %q", input, got, want)
		}
	}
}
