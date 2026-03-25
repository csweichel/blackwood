package storage

import (
	"context"
	"os"
	"strings"
	"path/filepath"
	"testing"
)

func newTestStore(t *testing.T) *Store {
	t.Helper()
	dir := t.TempDir()
	s, err := New(filepath.Join(dir, "test.db"), dir)
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s
}

func TestCreateAndGetDailyNote(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	note, err := s.CreateDailyNote(ctx, "2025-01-15")
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if note.Date != "2025-01-15" {
		t.Errorf("date = %q, want %q", note.Date, "2025-01-15")
	}
	if note.ID == "" {
		t.Fatal("expected non-empty ID")
	}

	got, err := s.GetDailyNote(ctx, note.ID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.Date != note.Date {
		t.Errorf("got date %q, want %q", got.Date, note.Date)
	}

	byDate, err := s.GetDailyNoteByDate(ctx, "2025-01-15")
	if err != nil {
		t.Fatalf("get by date: %v", err)
	}
	if byDate.ID != note.ID {
		t.Errorf("got ID %q, want %q", byDate.ID, note.ID)
	}
}

func TestGetOrCreateDailyNote(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	first, err := s.GetOrCreateDailyNote(ctx, "2025-03-01")
	if err != nil {
		t.Fatalf("first call: %v", err)
	}

	second, err := s.GetOrCreateDailyNote(ctx, "2025-03-01")
	if err != nil {
		t.Fatalf("second call: %v", err)
	}

	if first.ID != second.ID {
		t.Errorf("expected same ID, got %q and %q", first.ID, second.ID)
	}
}

func TestListDailyNotes(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	dates := []string{"2025-01-01", "2025-01-02", "2025-01-03", "2025-01-04", "2025-01-05"}
	for _, d := range dates {
		if _, err := s.CreateDailyNote(ctx, d); err != nil {
			t.Fatalf("create %s: %v", d, err)
		}
	}

	// First page
	page1, err := s.ListDailyNotes(ctx, 2, 0)
	if err != nil {
		t.Fatalf("list page 1: %v", err)
	}
	if len(page1) != 2 {
		t.Fatalf("page 1 len = %d, want 2", len(page1))
	}
	// Ordered by date DESC
	if page1[0].Date != "2025-01-05" {
		t.Errorf("page1[0].Date = %q, want 2025-01-05", page1[0].Date)
	}

	// Second page
	page2, err := s.ListDailyNotes(ctx, 2, 2)
	if err != nil {
		t.Fatalf("list page 2: %v", err)
	}
	if len(page2) != 2 {
		t.Fatalf("page 2 len = %d, want 2", len(page2))
	}
	if page2[0].Date != "2025-01-03" {
		t.Errorf("page2[0].Date = %q, want 2025-01-03", page2[0].Date)
	}
}

func TestEntryCRUD(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	note, err := s.CreateDailyNote(ctx, "2025-02-10")
	if err != nil {
		t.Fatalf("create note: %v", err)
	}

	e := &Entry{
		DailyNoteID: note.ID,
		Type:        "text",
		Content:     "# Hello",
		RawContent:  "Hello",
		Source:      "api",
		Metadata:    `{"key":"value"}`,
	}
	if err := s.CreateEntry(ctx, e); err != nil {
		t.Fatalf("create entry: %v", err)
	}
	if e.ID == "" {
		t.Fatal("expected non-empty entry ID")
	}

	got, err := s.GetEntry(ctx, e.ID)
	if err != nil {
		t.Fatalf("get entry: %v", err)
	}
	if got.Content != "# Hello" {
		t.Errorf("content = %q, want %q", got.Content, "# Hello")
	}
	if got.Source != "api" {
		t.Errorf("source = %q, want %q", got.Source, "api")
	}

	// Update
	got.Content = "# Updated"
	got.Type = "webclip"
	if err := s.UpdateEntry(ctx, got); err != nil {
		t.Fatalf("update entry: %v", err)
	}
	updated, err := s.GetEntry(ctx, got.ID)
	if err != nil {
		t.Fatalf("get updated: %v", err)
	}
	if updated.Content != "# Updated" {
		t.Errorf("updated content = %q, want %q", updated.Content, "# Updated")
	}
	if updated.Type != "webclip" {
		t.Errorf("updated type = %q, want %q", updated.Type, "webclip")
	}

	// List
	e2 := &Entry{DailyNoteID: note.ID, Type: "audio", Content: "audio note", Source: "telegram", Metadata: "{}"}
	if err := s.CreateEntry(ctx, e2); err != nil {
		t.Fatalf("create entry 2: %v", err)
	}
	entries, err := s.ListEntries(ctx, note.ID)
	if err != nil {
		t.Fatalf("list entries: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("entries len = %d, want 2", len(entries))
	}

	// Delete
	if err := s.DeleteEntry(ctx, e.ID); err != nil {
		t.Fatalf("delete entry: %v", err)
	}
	_, err = s.GetEntry(ctx, e.ID)
	if err == nil {
		t.Fatal("expected error after delete")
	}
}

func TestCreateEntryRequestIdempotencyMapping(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	note, err := s.CreateDailyNote(ctx, "2025-02-11")
	if err != nil {
		t.Fatalf("create note: %v", err)
	}

	e := &Entry{
		DailyNoteID: note.ID,
		Type:        "audio",
		Content:     "voice memo",
		RawContent:  "voice memo",
		Source:      "api",
		Metadata:    "{}",
	}
	if err := s.CreateEntry(ctx, e); err != nil {
		t.Fatalf("create entry: %v", err)
	}

	if err := s.RecordCreateEntryRequest(ctx, "client-123", e.ID); err != nil {
		t.Fatalf("record request: %v", err)
	}
	if err := s.RecordCreateEntryRequest(ctx, "client-123", e.ID); err != nil {
		t.Fatalf("record duplicate request: %v", err)
	}

	got, err := s.GetEntryByClientRequestID(ctx, "client-123")
	if err != nil {
		t.Fatalf("get by client request ID: %v", err)
	}
	if got.ID != e.ID {
		t.Fatalf("got entry ID %q, want %q", got.ID, e.ID)
	}
}

func TestCreateAttachmentWithFileStorage(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	note, _ := s.CreateDailyNote(ctx, "2025-04-01")
	e := &Entry{DailyNoteID: note.ID, Type: "photo", Source: "api", Metadata: "{}"}
	if err := s.CreateEntry(ctx, e); err != nil {
		t.Fatalf("create entry: %v", err)
	}

	data := []byte("fake image data")
	a := &Attachment{
		EntryID:     e.ID,
		Filename:    "photo.jpg",
		ContentType: "image/jpeg",
	}
	if err := s.CreateAttachment(ctx, a, data, "2025-04-01"); err != nil {
		t.Fatalf("create attachment: %v", err)
	}
	if a.ID == "" {
		t.Fatal("expected non-empty attachment ID")
	}
	if a.Size != int64(len(data)) {
		t.Errorf("size = %d, want %d", a.Size, len(data))
	}

	// Verify file exists on disk
	if _, err := os.Stat(a.StoragePath); err != nil {
		t.Fatalf("attachment file missing: %v", err)
	}
}

func TestGetAttachmentData(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	note, _ := s.CreateDailyNote(ctx, "2025-04-02")
	e := &Entry{DailyNoteID: note.ID, Type: "text", Source: "api", Metadata: "{}"}
	_ = s.CreateEntry(ctx, e)

	content := []byte("hello attachment")
	a := &Attachment{EntryID: e.ID, Filename: "doc.txt", ContentType: "text/plain"}
	if err := s.CreateAttachment(ctx, a, content, "2025-04-02"); err != nil {
		t.Fatalf("create: %v", err)
	}

	got, err := s.GetAttachmentData(ctx, a.ID)
	if err != nil {
		t.Fatalf("get data: %v", err)
	}
	if string(got) != string(content) {
		t.Errorf("data = %q, want %q", got, content)
	}
}

func TestListAttachments(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	note, _ := s.CreateDailyNote(ctx, "2025-04-03")
	e := &Entry{DailyNoteID: note.ID, Type: "text", Source: "api", Metadata: "{}"}
	_ = s.CreateEntry(ctx, e)

	for _, name := range []string{"a.txt", "b.txt", "c.txt"} {
		a := &Attachment{EntryID: e.ID, Filename: name, ContentType: "text/plain"}
		if err := s.CreateAttachment(ctx, a, []byte("data"), "2025-04-03"); err != nil {
			t.Fatalf("create %s: %v", name, err)
		}
	}

	list, err := s.ListAttachments(ctx, e.ID)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(list) != 3 {
		t.Fatalf("len = %d, want 3", len(list))
	}
}

func TestDeleteEntryCascadesToAttachments(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	note, _ := s.CreateDailyNote(ctx, "2025-04-04")
	e := &Entry{DailyNoteID: note.ID, Type: "photo", Source: "api", Metadata: "{}"}
	_ = s.CreateEntry(ctx, e)

	a := &Attachment{EntryID: e.ID, Filename: "img.png", ContentType: "image/png"}
	if err := s.CreateAttachment(ctx, a, []byte("png data"), "2025-04-04"); err != nil {
		t.Fatalf("create attachment: %v", err)
	}
	storagePath := a.StoragePath

	// Delete the entry — should cascade to attachments
	if err := s.DeleteEntry(ctx, e.ID); err != nil {
		t.Fatalf("delete entry: %v", err)
	}

	// Attachment row should be gone
	_, err := s.GetAttachment(ctx, a.ID)
	if err == nil {
		t.Fatal("expected error getting deleted attachment")
	}

	// Attachment file should be removed from disk
	if _, err := os.Stat(storagePath); !os.IsNotExist(err) {
		t.Errorf("expected attachment file to be removed, got err: %v", err)
	}
}

func TestInsertIntoSection(t *testing.T) {
	tests := []struct {
		name    string
		content string
		section string
		text    string
		want    string
	}{
		{
			name:    "append to Notes in empty structured note",
			content: "# Notes\n\n# Links\n",
			section: "# Notes",
			text:    "\n\n---\n*14:32 — Telegram*\n\nHello world\n",
			want:    "# Notes\n\n\n---\n*14:32 — Telegram*\n\nHello world\n\n# Links\n",
		},
		{
			name:    "append to Links in empty structured note",
			content: "# Notes\n\n# Links\n",
			section: "# Links",
			text:    "\n\n---\n*Clipped*\n\nSome link\n",
			want:    "# Notes\n\n# Links\n\n\n---\n*Clipped*\n\nSome link\n\n",
		},
		{
			name:    "append to Notes with existing content",
			content: "# Notes\n\n\n---\n*14:00*\n\nFirst note\n\n# Links\n",
			section: "# Notes",
			text:    "\n\n---\n*14:30*\n\nSecond note\n",
			want:    "# Notes\n\n\n---\n*14:00*\n\nFirst note\n\n\n---\n*14:30*\n\nSecond note\n\n# Links\n",
		},
		{
			name:    "append to Links with existing content",
			content: "# Notes\n\n# Links\n\n\n---\n*Clipped*\n\nFirst link\n",
			section: "# Links",
			text:    "\n\n---\n*Clipped*\n\nSecond link\n",
			want:    "# Notes\n\n# Links\n\n\n---\n*Clipped*\n\nFirst link\n\n\n---\n*Clipped*\n\nSecond link\n\n",
		},
		{
			name:    "section not found — appended at end",
			content: "Some legacy content\n",
			section: "# Notes",
			text:    "\n\n---\n*14:00*\n\nNew note\n",
			want:    "Some legacy content\n\n# Notes\n\n\n---\n*14:00*\n\nNew note\n",
		},
		{
			name:    "empty content — section appended",
			content: "",
			section: "# Notes",
			text:    "\n\n---\n*14:00*\n\nNew note\n",
			want:    "\n# Notes\n\n\n---\n*14:00*\n\nNew note\n",
		},
		{
			name:    "only Notes section exists, append to Links",
			content: "# Notes\n\nSome note\n",
			section: "# Links",
			text:    "\n\n---\n*Clipped*\n\nA link\n",
			want:    "# Notes\n\nSome note\n\n# Links\n\n\n---\n*Clipped*\n\nA link\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := insertIntoSection(tt.content, tt.section, tt.text)
			if got != tt.want {
				t.Errorf("insertIntoSection():\ngot:\n%q\nwant:\n%q", got, tt.want)
			}
		})
	}
}

func TestAppendToSection(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	note, err := s.CreateDailyNote(ctx, "2025-06-01")
	if err != nil {
		t.Fatalf("create note: %v", err)
	}

	// First append to Notes — should initialize the section structure.
	if err := s.AppendToSection(ctx, note.ID, "# Notes", "\n\n---\n*10:00*\n\nMorning note\n"); err != nil {
		t.Fatalf("append to Notes: %v", err)
	}

	content := s.readNoteContent("2025-06-01")
	if !strings.Contains(content, "# Notes") {
		t.Error("expected # Notes heading")
	}
	if !strings.Contains(content, "# Links") {
		t.Error("expected # Links heading")
	}
	if !strings.Contains(content, "Morning note") {
		t.Error("expected note content")
	}

	// Append to Links.
	if err := s.AppendToSection(ctx, note.ID, "# Links", "\n\n---\n*Clipped*\n\nA link\n"); err != nil {
		t.Fatalf("append to Links: %v", err)
	}

	content = s.readNoteContent("2025-06-01")
	if !strings.Contains(content, "A link") {
		t.Error("expected link content")
	}

	// Verify ordering: Notes section comes before Links section.
	notesIdx := strings.Index(content, "# Notes")
	linksIdx := strings.Index(content, "# Links")
	if notesIdx >= linksIdx {
		t.Errorf("expected # Notes (%d) before # Links (%d)", notesIdx, linksIdx)
	}

	// Verify note content is in Notes section (before Links).
	morningIdx := strings.Index(content, "Morning note")
	if morningIdx >= linksIdx {
		t.Errorf("expected 'Morning note' (%d) before # Links (%d)", morningIdx, linksIdx)
	}

	// Verify link content is after Links heading.
	linkContentIdx := strings.Index(content, "A link")
	if linkContentIdx <= linksIdx {
		t.Errorf("expected 'A link' (%d) after # Links (%d)", linkContentIdx, linksIdx)
	}
}

func TestAppendToSectionLegacyNote(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	note, err := s.CreateDailyNote(ctx, "2025-06-02")
	if err != nil {
		t.Fatalf("create note: %v", err)
	}

	// Write legacy content without sections.
	if err := s.writeNoteContent("2025-06-02", "Some old content\n"); err != nil {
		t.Fatalf("write legacy content: %v", err)
	}

	// Appending to a section should add the heading at the end.
	if err := s.AppendToSection(ctx, note.ID, "# Notes", "\n\n---\n*10:00*\n\nNew note\n"); err != nil {
		t.Fatalf("append to Notes: %v", err)
	}

	content := s.readNoteContent("2025-06-02")
	if !strings.Contains(content, "Some old content") {
		t.Error("expected legacy content preserved")
	}
	if !strings.Contains(content, "# Notes") {
		t.Error("expected # Notes heading added")
	}
	if !strings.Contains(content, "New note") {
		t.Error("expected new note content")
	}
}

func TestGranolaSyncState(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	// Getting a non-existent state should fail.
	_, err := s.GetGranolaSyncState(ctx, "not_xxx")
	if err == nil {
		t.Fatal("expected error for missing sync state")
	}

	// Insert a sync state.
	gs := &GranolaSyncState{
		NoteID:    "not_abc123",
		EntryID:   "entry-1",
		UpdatedAt: "2026-01-27T16:45:00Z",
	}
	if err := s.UpsertGranolaSyncState(ctx, gs); err != nil {
		t.Fatalf("upsert: %v", err)
	}

	// Retrieve it.
	got, err := s.GetGranolaSyncState(ctx, "not_abc123")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.EntryID != "entry-1" {
		t.Errorf("entry_id = %q, want %q", got.EntryID, "entry-1")
	}
	if got.UpdatedAt != "2026-01-27T16:45:00Z" {
		t.Errorf("updated_at = %q, want %q", got.UpdatedAt, "2026-01-27T16:45:00Z")
	}
	if got.SyncedAt.IsZero() {
		t.Error("synced_at should be set")
	}

	// Update the sync state (note was edited in Granola).
	gs2 := &GranolaSyncState{
		NoteID:    "not_abc123",
		EntryID:   "entry-2",
		UpdatedAt: "2026-01-28T10:00:00Z",
	}
	if err := s.UpsertGranolaSyncState(ctx, gs2); err != nil {
		t.Fatalf("upsert update: %v", err)
	}

	got2, err := s.GetGranolaSyncState(ctx, "not_abc123")
	if err != nil {
		t.Fatalf("get after update: %v", err)
	}
	if got2.EntryID != "entry-2" {
		t.Errorf("entry_id = %q, want %q", got2.EntryID, "entry-2")
	}
	if got2.UpdatedAt != "2026-01-28T10:00:00Z" {
		t.Errorf("updated_at = %q, want %q", got2.UpdatedAt, "2026-01-28T10:00:00Z")
	}
}
