package api

import (
	"bytes"
	"context"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/csweichel/blackwood/internal/storage"
)

func newUploadTestStore(t *testing.T) *storage.Store {
	t.Helper()
	dir := t.TempDir()
	store, err := storage.New(filepath.Join(dir, "blackwood.db"), dir)
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	return store
}

func TestServeUploadAttachmentStoresImageWithoutAppendingToNote(t *testing.T) {
	store := newUploadTestStore(t)

	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	part, err := writer.CreateFormFile("file", "photo.png")
	if err != nil {
		t.Fatalf("create form file: %v", err)
	}
	png := []byte{0x89, 'P', 'N', 'G', 0x0d, 0x0a, 0x1a, 0x0a, 0x00, 0x00, 0x00, 0x0d}
	if _, err := part.Write(png); err != nil {
		t.Fatalf("write form file: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close writer: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/api/daily-notes/2025-01-15/attachments", &body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	req.SetPathValue("date", "2025-01-15")
	rec := httptest.NewRecorder()

	ServeUploadAttachment(store).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}

	var resp struct {
		AttachmentID string `json:"attachmentId"`
		Filename     string `json:"filename"`
		URL          string `json:"url"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.AttachmentID == "" {
		t.Fatal("expected attachment ID")
	}
	if !strings.HasPrefix(resp.Filename, "photo-") || !strings.HasSuffix(resp.Filename, ".png") {
		t.Fatalf("filename = %q, want stored photo filename", resp.Filename)
	}
	if resp.URL != "/api/daily-notes/2025-01-15/attachments/"+resp.Filename {
		t.Fatalf("url = %q", resp.URL)
	}

	path, err := store.AttachmentPath("2025-01-15", resp.Filename)
	if err != nil {
		t.Fatalf("attachment path: %v", err)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read attachment: %v", err)
	}
	if !bytes.Equal(got, png) {
		t.Fatalf("stored bytes = %v, want %v", got, png)
	}

	note, err := store.GetDailyNoteByDate(context.Background(), "2025-01-15")
	if err != nil {
		t.Fatalf("get note: %v", err)
	}
	if note.Content != "" {
		t.Fatalf("note content = %q, want empty", note.Content)
	}
	entries, err := store.ListEntries(context.Background(), note.ID)
	if err != nil {
		t.Fatalf("list entries: %v", err)
	}
	if len(entries) != 1 || entries[0].Type != "photo" {
		t.Fatalf("entries = %#v, want one photo entry", entries)
	}
}
