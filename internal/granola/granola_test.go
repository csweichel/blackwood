package granola

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/csweichel/blackwood/internal/storage"
)

func TestBuildNoteMarkdown(t *testing.T) {
	tests := []struct {
		name     string
		detail   noteDetail
		contains []string
		absent   []string
	}{
		{
			name: "full note with attendees and transcript",
			detail: noteDetail{
				Title: "Sprint Planning",
				CalendarEvent: &calendarEvent{
					ScheduledStartTime: "2026-01-27T15:30:00Z",
					ScheduledEndTime:   "2026-01-27T16:30:00Z",
				},
				Attendees: []Person{
					{Name: "Alice", Email: "alice@example.com"},
					{Name: "Bob", Email: "bob@example.com"},
				},
				SummaryMarkdown: "## Summary\n\nWe planned the sprint.",
				Transcript: []transcriptLine{
					{Speaker: speaker{Source: "microphone"}, Text: "Let's start."},
					{Speaker: speaker{Source: "speaker"}, Text: "Sounds good."},
				},
			},
			contains: []string{
				"## Sprint Planning",
				"**Time:** 15:30 – 16:30",
				"**Attendees:** Alice, Bob",
				"## Summary",
				"We planned the sprint.",
				"### Transcript",
				"**You:** Let's start.",
				"**Speaker:** Sounds good.",
			},
		},
		{
			name: "note with only summary text, no transcript",
			detail: noteDetail{
				Title:       "Quick Sync",
				SummaryText: "A brief sync about the project.",
			},
			contains: []string{
				"## Quick Sync",
				"A brief sync about the project.",
			},
			absent: []string{
				"### Transcript",
				"**Time:**",
				"**Attendees:**",
			},
		},
		{
			name: "attendee with no name falls back to email",
			detail: noteDetail{
				Title: "Meeting",
				Attendees: []Person{
					{Name: "", Email: "anon@example.com"},
				},
			},
			contains: []string{
				"**Attendees:** anon@example.com",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			md := buildNoteMarkdown(&tt.detail)
			for _, s := range tt.contains {
				if !strings.Contains(md, s) {
					t.Errorf("expected markdown to contain %q\ngot:\n%s", s, md)
				}
			}
			for _, s := range tt.absent {
				if strings.Contains(md, s) {
					t.Errorf("expected markdown NOT to contain %q\ngot:\n%s", s, md)
				}
			}
		})
	}
}

func TestParseDateFromISO(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"2026-01-27T15:30:00Z", "2026-01-27"},
		{"2026-03-01", "2026-03-01"},
		{"short", ""},
		{"", ""},
	}
	for _, tt := range tests {
		got := parseDateFromISO(tt.input)
		if got != tt.want {
			t.Errorf("parseDateFromISO(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestSyncWithFakeServer(t *testing.T) {
	// Set up a fake Granola API server.
	notesList := listNotesResponse{
		Notes: []Note{
			{
				ID:        "not_test1",
				Title:     "Test Meeting",
				Owner:     Person{Name: "Alice", Email: "alice@example.com"},
				CreatedAt: "2026-01-27T15:30:00Z",
				UpdatedAt: "2026-01-27T16:45:00Z",
			},
		},
		HasMore: false,
	}

	noteDetailResp := noteDetail{
		ID:        "not_test1",
		Title:     "Test Meeting",
		Owner:     Person{Name: "Alice", Email: "alice@example.com"},
		CreatedAt: "2026-01-27T15:30:00Z",
		UpdatedAt: "2026-01-27T16:45:00Z",
		CalendarEvent: &calendarEvent{
			EventTitle:         "Test Meeting",
			ScheduledStartTime: "2026-01-27T15:30:00Z",
			ScheduledEndTime:   "2026-01-27T16:30:00Z",
		},
		Attendees: []Person{
			{Name: "Alice", Email: "alice@example.com"},
			{Name: "Bob", Email: "bob@example.com"},
		},
		SummaryMarkdown: "We discussed testing strategies.",
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.URL.Path == "/v1/notes" && r.Method == http.MethodGet:
			_ = json.NewEncoder(w).Encode(notesList)
		case r.URL.Path == "/v1/notes/not_test1" && r.Method == http.MethodGet:
			_ = json.NewEncoder(w).Encode(noteDetailResp)
		default:
			http.Error(w, "not found", http.StatusNotFound)
		}
	}))
	defer srv.Close()

	// Set up storage.
	dir := t.TempDir()
	store, err := storage.New(filepath.Join(dir, "test.db"), dir)
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	defer store.Close()

	// Create syncer pointing at the fake server.
	syncer := New("test-api-key", store, nil, 1*time.Hour)
	syncer.client.baseURL = srv.URL + "/v1"

	ctx := context.Background()

	// First sync should import the note.
	if err := syncer.sync(ctx); err != nil {
		t.Fatalf("first sync: %v", err)
	}

	// Verify the sync state was recorded.
	state, err := store.GetGranolaSyncState(ctx, "not_test1")
	if err != nil {
		t.Fatalf("get sync state: %v", err)
	}
	if state.UpdatedAt != "2026-01-27T16:45:00Z" {
		t.Errorf("sync state updated_at = %q, want %q", state.UpdatedAt, "2026-01-27T16:45:00Z")
	}
	if state.EntryID == "" {
		t.Fatal("expected non-empty entry ID")
	}

	// Verify the entry was created.
	entry, err := store.GetEntry(ctx, state.EntryID)
	if err != nil {
		t.Fatalf("get entry: %v", err)
	}
	if !strings.Contains(entry.Content, "Test Meeting") {
		t.Errorf("entry content should contain title, got: %s", entry.Content)
	}
	if !strings.Contains(entry.Content, "We discussed testing strategies.") {
		t.Errorf("entry content should contain summary, got: %s", entry.Content)
	}
	if entry.Source != "import" {
		t.Errorf("entry source = %q, want %q", entry.Source, "import")
	}

	// Verify the daily note was created for the meeting date.
	dailyNote, err := store.GetDailyNoteByDate(ctx, "2026-01-27")
	if err != nil {
		t.Fatalf("get daily note: %v", err)
	}
	if !strings.Contains(dailyNote.Content, "Imported from Granola") {
		t.Errorf("daily note should contain import marker, got: %s", dailyNote.Content)
	}

	// Second sync with same updated_at should skip.
	if err := syncer.sync(ctx); err != nil {
		t.Fatalf("second sync: %v", err)
	}

	// Verify no duplicate entries — the entry ID should be the same.
	state2, err := store.GetGranolaSyncState(ctx, "not_test1")
	if err != nil {
		t.Fatalf("get sync state after second sync: %v", err)
	}
	if state2.EntryID != state.EntryID {
		t.Errorf("expected same entry ID after skip, got %q vs %q", state2.EntryID, state.EntryID)
	}
}
