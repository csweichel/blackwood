package whatsapp

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestDownloadMedia(t *testing.T) {
	mediaContent := []byte("fake-audio-data")

	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer test-token" {
			t.Errorf("unexpected auth header: %s", r.Header.Get("Authorization"))
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}

		switch r.URL.Path {
		case "/media123":
			// Return media metadata with download URL pointing back to this server.
			resp := mediaResponse{
				URL:      srv.URL + "/download/media123",
				MimeType: "audio/ogg",
			}
			_ = json.NewEncoder(w).Encode(resp)
		case "/download/media123":
			w.Header().Set("Content-Type", "audio/ogg")
			_, _ = w.Write(mediaContent)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	client := NewClient("test-token", "phone123")
	client.baseURL = srv.URL

	data, contentType, err := client.DownloadMedia(context.Background(), "media123")
	if err != nil {
		t.Fatalf("DownloadMedia: %v", err)
	}
	if contentType != "audio/ogg" {
		t.Errorf("content type = %q, want %q", contentType, "audio/ogg")
	}
	if string(data) != string(mediaContent) {
		t.Errorf("data = %q, want %q", data, mediaContent)
	}
}

func TestDownloadMedia_MetadataError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "not found", http.StatusNotFound)
	}))
	defer srv.Close()

	client := NewClient("test-token", "phone123")
	client.baseURL = srv.URL

	_, _, err := client.DownloadMedia(context.Background(), "bad-id")
	if err == nil {
		t.Fatal("expected error for bad media ID")
	}
}

func TestSendTextMessage(t *testing.T) {
	var receivedBody sendMessageRequest

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("method = %s, want POST", r.Method)
		}
		if r.Header.Get("Authorization") != "Bearer test-token" {
			t.Errorf("unexpected auth header: %s", r.Header.Get("Authorization"))
		}
		if r.URL.Path != "/phone123/messages" {
			t.Errorf("path = %s, want /phone123/messages", r.URL.Path)
		}

		_ = json.NewDecoder(r.Body).Decode(&receivedBody)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	client := NewClient("test-token", "phone123")
	client.baseURL = srv.URL

	err := client.SendTextMessage(context.Background(), "4915123456", "Hello!")
	if err != nil {
		t.Fatalf("SendTextMessage: %v", err)
	}

	if receivedBody.MessagingProduct != "whatsapp" {
		t.Errorf("messaging_product = %q, want %q", receivedBody.MessagingProduct, "whatsapp")
	}
	if receivedBody.To != "4915123456" {
		t.Errorf("to = %q, want %q", receivedBody.To, "4915123456")
	}
	if receivedBody.Type != "text" {
		t.Errorf("type = %q, want %q", receivedBody.Type, "text")
	}
	if receivedBody.Text == nil || receivedBody.Text.Body != "Hello!" {
		t.Errorf("text body = %v, want %q", receivedBody.Text, "Hello!")
	}
}

func TestSendTextMessage_Error(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "server error", http.StatusInternalServerError)
	}))
	defer srv.Close()

	client := NewClient("test-token", "phone123")
	client.baseURL = srv.URL

	err := client.SendTextMessage(context.Background(), "4915123456", "Hello!")
	if err == nil {
		t.Fatal("expected error for server error response")
	}
}
