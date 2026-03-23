package watcher

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestDetectsNewNoteFile(t *testing.T) {
	dir := t.TempDir()

	w := New([]string{dir}, 50*time.Millisecond)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	ch, err := w.Start(ctx)
	if err != nil {
		t.Fatalf("Start: %v", err)
	}

	// Create a .note file after the watcher has started.
	notePath := filepath.Join(dir, "test.note")
	if err := os.WriteFile(notePath, []byte("data"), 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	select {
	case got := <-ch:
		if got != notePath {
			t.Errorf("got %q, want %q", got, notePath)
		}
	case <-ctx.Done():
		t.Fatal("timed out waiting for .note file detection")
	}
}

func TestDetectsMdFile(t *testing.T) {
	dir := t.TempDir()

	// Pre-create a .md file.
	mdPath := filepath.Join(dir, "note.md")
	if err := os.WriteFile(mdPath, []byte("# Hello"), 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	w := New([]string{dir}, 50*time.Millisecond)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	ch, err := w.Start(ctx)
	if err != nil {
		t.Fatalf("Start: %v", err)
	}

	select {
	case got := <-ch:
		if got != mdPath {
			t.Errorf("got %q, want %q", got, mdPath)
		}
	case <-ctx.Done():
		t.Fatal("timed out waiting for .md file detection")
	}
}

func TestRecursiveScanning(t *testing.T) {
	dir := t.TempDir()
	subdir := filepath.Join(dir, "sub", "deep")
	if err := os.MkdirAll(subdir, 0755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}

	notePath := filepath.Join(subdir, "nested.note")
	if err := os.WriteFile(notePath, []byte("data"), 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	w := New([]string{dir}, 50*time.Millisecond)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	ch, err := w.Start(ctx)
	if err != nil {
		t.Fatalf("Start: %v", err)
	}

	select {
	case got := <-ch:
		if got != notePath {
			t.Errorf("got %q, want %q", got, notePath)
		}
	case <-ctx.Done():
		t.Fatal("timed out waiting for nested file detection")
	}
}

func TestMultipleDirs(t *testing.T) {
	dir1 := t.TempDir()
	dir2 := t.TempDir()

	note1 := filepath.Join(dir1, "a.note")
	note2 := filepath.Join(dir2, "b.md")
	if err := os.WriteFile(note1, []byte("1"), 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	if err := os.WriteFile(note2, []byte("2"), 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	w := New([]string{dir1, dir2}, 50*time.Millisecond)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	ch, err := w.Start(ctx)
	if err != nil {
		t.Fatalf("Start: %v", err)
	}

	seen := make(map[string]bool)
	for i := 0; i < 2; i++ {
		select {
		case got := <-ch:
			seen[got] = true
		case <-ctx.Done():
			t.Fatalf("timed out, only got %d of 2 files", i)
		}
	}

	if !seen[note1] {
		t.Errorf("missing %q", note1)
	}
	if !seen[note2] {
		t.Errorf("missing %q", note2)
	}
}

func TestIgnoresNonNoteFiles(t *testing.T) {
	dir := t.TempDir()

	// Pre-create non-.note/.md files before starting the watcher.
	for _, name := range []string{"readme.txt", "image.png", "data.json"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("x"), 0644); err != nil {
			t.Fatalf("WriteFile: %v", err)
		}
	}

	w := New([]string{dir}, 50*time.Millisecond)
	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()

	ch, err := w.Start(ctx)
	if err != nil {
		t.Fatalf("Start: %v", err)
	}

	select {
	case path := <-ch:
		t.Fatalf("unexpected file emitted: %s", path)
	case <-ctx.Done():
		// Expected: no files emitted.
	}
}

func TestReEmitsFilesOnEachScan(t *testing.T) {
	dir := t.TempDir()

	// Pre-create a .note file.
	notePath := filepath.Join(dir, "existing.note")
	if err := os.WriteFile(notePath, []byte("data"), 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	w := New([]string{dir}, 50*time.Millisecond)
	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	ch, err := w.Start(ctx)
	if err != nil {
		t.Fatalf("Start: %v", err)
	}

	// First emission.
	select {
	case got := <-ch:
		if got != notePath {
			t.Errorf("got %q, want %q", got, notePath)
		}
	case <-ctx.Done():
		t.Fatal("timed out waiting for first emission")
	}

	// Second emission should also occur (no in-memory dedup).
	select {
	case got := <-ch:
		if got != notePath {
			t.Errorf("got %q, want %q", got, notePath)
		}
	case <-ctx.Done():
		t.Fatal("timed out waiting for second emission — watcher should re-emit files")
	}
}

func TestStopsOnContextCancel(t *testing.T) {
	dir := t.TempDir()

	w := New([]string{dir}, 50*time.Millisecond)
	ctx, cancel := context.WithCancel(context.Background())

	ch, err := w.Start(ctx)
	if err != nil {
		t.Fatalf("Start: %v", err)
	}

	cancel()

	// Channel should be closed after context cancellation.
	timer := time.NewTimer(time.Second)
	defer timer.Stop()

	select {
	case _, ok := <-ch:
		if ok {
			// Draining is fine; wait for close.
			select {
			case _, ok2 := <-ch:
				if ok2 {
					t.Fatal("channel still open after context cancel")
				}
			case <-timer.C:
				t.Fatal("channel not closed after context cancel")
			}
		}
	case <-timer.C:
		t.Fatal("channel not closed after context cancel")
	}
}
