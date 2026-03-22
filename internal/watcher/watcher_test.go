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

	w := New(dir, 50*time.Millisecond)
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

func TestIgnoresNonNoteFiles(t *testing.T) {
	dir := t.TempDir()

	// Pre-create non-.note files before starting the watcher.
	for _, name := range []string{"readme.txt", "image.png", "data.json"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("x"), 0644); err != nil {
			t.Fatalf("WriteFile: %v", err)
		}
	}

	w := New(dir, 50*time.Millisecond)
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

func TestDoesNotReEmitSeenFiles(t *testing.T) {
	dir := t.TempDir()

	// Pre-create a .note file.
	notePath := filepath.Join(dir, "existing.note")
	if err := os.WriteFile(notePath, []byte("data"), 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	w := New(dir, 50*time.Millisecond)
	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	ch, err := w.Start(ctx)
	if err != nil {
		t.Fatalf("Start: %v", err)
	}

	// First emission is expected.
	select {
	case got := <-ch:
		if got != notePath {
			t.Errorf("got %q, want %q", got, notePath)
		}
	case <-ctx.Done():
		t.Fatal("timed out waiting for first emission")
	}

	// No second emission should occur for the same file.
	select {
	case path := <-ch:
		t.Fatalf("file re-emitted: %s", path)
	case <-ctx.Done():
		// Expected: no re-emission.
	}
}

func TestStopsOnContextCancel(t *testing.T) {
	dir := t.TempDir()

	w := New(dir, 50*time.Millisecond)
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
