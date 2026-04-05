package storage

import (
	"os"
	"path/filepath"
	"sort"
	"testing"
)

func TestSubpagePath(t *testing.T) {
	s := newTestStore(t)

	tests := []struct {
		name    string
		date    string
		subpage string
		wantErr bool
	}{
		{"valid name", "2025-04-05", "Foobar", false},
		{"name with spaces", "2025-04-05", "Meeting Notes", false},
		{"empty name", "2025-04-05", "", true},
		{"path traversal dotdot", "2025-04-05", "..", true},
		{"path traversal slash", "2025-04-05", "foo/bar", true},
		{"path traversal backslash", "2025-04-05", "foo\\bar", true},
		{"null byte", "2025-04-05", "foo\x00bar", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path, err := s.SubpagePath(tt.date, tt.subpage)
			if (err != nil) != tt.wantErr {
				t.Errorf("SubpagePath(%q, %q) error = %v, wantErr %v", tt.date, tt.subpage, err, tt.wantErr)
				return
			}
			if !tt.wantErr {
				// Path should end with <name>.md inside the day directory.
				if filepath.Ext(path) != ".md" {
					t.Errorf("expected .md extension, got %q", path)
				}
				if filepath.Base(path) != tt.subpage+".md" {
					t.Errorf("expected base %q, got %q", tt.subpage+".md", filepath.Base(path))
				}
			}
		})
	}
}

func TestListSubpageNames(t *testing.T) {
	s := newTestStore(t)
	date := "2025-04-05"

	// No directory yet — should return empty.
	names, err := s.ListSubpageNames(date)
	if err != nil {
		t.Fatalf("list empty: %v", err)
	}
	if len(names) != 0 {
		t.Errorf("expected empty list, got %v", names)
	}

	// Create the day directory and some files.
	dayDir := filepath.Join(s.dataDir, "notes", "2025", "04", "05")
	if err := os.MkdirAll(dayDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	// index.md should be excluded.
	for _, f := range []struct {
		name    string
		content []byte
	}{
		{"index.md", []byte("# Notes")},
		{"Foobar.md", []byte("hello")},
		{"Meeting.md", []byte("world")},
		{"photo.jpg", []byte{0xff}}, // non-.md files should be excluded
	} {
		if err := os.WriteFile(filepath.Join(dayDir, f.name), f.content, 0o644); err != nil {
			t.Fatalf("write %s: %v", f.name, err)
		}
	}

	names, err = s.ListSubpageNames(date)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	sort.Strings(names)
	if len(names) != 2 || names[0] != "Foobar" || names[1] != "Meeting" {
		t.Errorf("expected [Foobar Meeting], got %v", names)
	}
}
