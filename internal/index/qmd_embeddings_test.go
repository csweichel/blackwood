package index

import (
	"testing"
)

func TestEntryIDFromPath(t *testing.T) {
	tests := []struct {
		path string
		want string
	}{
		{"abc123.md", "abc123"},
		{"entries/abc123.md", "abc123"},
		{"/full/path/to/entries/xyz.md", "xyz"},
		{"qmd://blackwood/abc123.md", "abc123"},
		{"qmd://collection/sub/path/entry.md", "entry"},
		{"no-extension", "no-extension"},
		{"", "."},
	}
	for _, tt := range tests {
		got := entryIDFromPath(tt.path)
		if got != tt.want {
			t.Errorf("entryIDFromPath(%q) = %q, want %q", tt.path, got, tt.want)
		}
	}
}

func TestParseQMDResults_ObjectWrapper(t *testing.T) {
	input := `{"results":[{"docid":"#abc123","file":"qmd://blackwood/entry1.md","score":0.95,"snippet":"hello world","title":"Entry 1"}]}`
	results, err := parseQMDResults(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].EntryID != "entry1" {
		t.Errorf("expected entry ID 'entry1', got %q", results[0].EntryID)
	}
	if results[0].Score != 0.95 {
		t.Errorf("expected score 0.95, got %f", results[0].Score)
	}
	if results[0].Snippet != "hello world" {
		t.Errorf("expected snippet 'hello world', got %q", results[0].Snippet)
	}
}

func TestParseQMDResults_Array(t *testing.T) {
	input := `[{"docid":"#a","file":"qmd://coll/e1.md","score":0.8,"snippet":"first","title":"T1"},{"docid":"#b","file":"qmd://coll/e2.md","score":0.6,"snippet":"second","title":"T2"}]`
	results, err := parseQMDResults(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}
	if results[0].EntryID != "e1" {
		t.Errorf("expected 'e1', got %q", results[0].EntryID)
	}
	if results[1].EntryID != "e2" {
		t.Errorf("expected 'e2', got %q", results[1].EntryID)
	}
}

func TestParseQMDResults_Empty(t *testing.T) {
	results, err := parseQMDResults("")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if results != nil {
		t.Errorf("expected nil for empty input, got %v", results)
	}
}

func TestParseQMDResults_FallbackToTitle(t *testing.T) {
	input := `[{"docid":"#a","file":"e1.md","score":0.5,"snippet":"","title":"My Title"}]`
	results, err := parseQMDResults(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].Snippet != "My Title" {
		t.Errorf("expected snippet to fall back to title 'My Title', got %q", results[0].Snippet)
	}
}

func TestMapQMDEntries_SnippetTruncation(t *testing.T) {
	longSnippet := ""
	for i := 0; i < 250; i++ {
		longSnippet += "x"
	}
	entries := []qmdResultEntry{
		{DocID: "#a", File: "e1.md", Score: 0.9, Snippet: longSnippet},
	}
	results := mapQMDEntries(entries)
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if len(results[0].Snippet) != 200 {
		t.Errorf("expected snippet truncated to 200, got %d", len(results[0].Snippet))
	}
}

func TestParseQMDResults_InvalidJSON(t *testing.T) {
	_, err := parseQMDResults("not json at all")
	if err == nil {
		t.Fatal("expected error for invalid JSON, got nil")
	}
}
