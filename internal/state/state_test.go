package state

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoad_NonExistentFile(t *testing.T) {
	s, err := Load(filepath.Join(t.TempDir(), "missing.json"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(s.Files) != 0 {
		t.Errorf("expected empty state, got %d entries", len(s.Files))
	}
}

func TestSaveAndReload(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")

	s, err := Load(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	s.MarkProcessed("/tmp/a.txt", "abc123")
	s.MarkProcessed("/tmp/b.txt", "def456")

	if err := s.Save(); err != nil {
		t.Fatalf("save error: %v", err)
	}

	s2, err := Load(path)
	if err != nil {
		t.Fatalf("reload error: %v", err)
	}

	if len(s2.Files) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(s2.Files))
	}
	if s2.Files["/tmp/a.txt"].Hash != "abc123" {
		t.Errorf("hash = %q, want %q", s2.Files["/tmp/a.txt"].Hash, "abc123")
	}
	if s2.Files["/tmp/b.txt"].Hash != "def456" {
		t.Errorf("hash = %q, want %q", s2.Files["/tmp/b.txt"].Hash, "def456")
	}
}

func TestIsProcessed(t *testing.T) {
	s, err := Load(filepath.Join(t.TempDir(), "state.json"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if s.IsProcessed("/tmp/unknown.txt") {
		t.Error("expected false for unknown file")
	}

	s.MarkProcessed("/tmp/unknown.txt", "hash")

	if !s.IsProcessed("/tmp/unknown.txt") {
		t.Error("expected true after MarkProcessed")
	}
}

func TestComputeHash_Consistent(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.txt")
	if err := os.WriteFile(path, []byte("hello world\n"), 0644); err != nil {
		t.Fatal(err)
	}

	h1, err := ComputeHash(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	h2, err := ComputeHash(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if h1 != h2 {
		t.Errorf("hashes differ: %q vs %q", h1, h2)
	}

	if len(h1) != 64 {
		t.Errorf("expected 64-char hex string, got %d chars", len(h1))
	}
}
