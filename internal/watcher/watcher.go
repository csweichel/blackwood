package watcher

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Watcher polls directories for .note and .md files.
type Watcher struct {
	dirs         []string
	pollInterval time.Duration
}

// New creates a Watcher that polls the given directories every pollInterval.
func New(dirs []string, pollInterval time.Duration) *Watcher {
	return &Watcher{
		dirs:         dirs,
		pollInterval: pollInterval,
	}
}

// Start begins polling and returns a channel that emits paths of discovered
// .note and .md files. Scans recursively. The channel is closed when ctx is
// cancelled.
func (w *Watcher) Start(ctx context.Context) (<-chan string, error) {
	for _, dir := range w.dirs {
		info, err := os.Stat(dir)
		if err != nil {
			return nil, fmt.Errorf("watch directory %s: %w", dir, err)
		}
		if !info.IsDir() {
			return nil, fmt.Errorf("watch path %s is not a directory", dir)
		}
	}

	ch := make(chan string)
	go func() {
		defer close(ch)

		// Initial scan before the first tick.
		w.scan(ctx, ch)

		ticker := time.NewTicker(w.pollInterval)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				w.scan(ctx, ch)
			}
		}
	}()

	return ch, nil
}

// scan walks each directory recursively, emitting .note and .md files.
func (w *Watcher) scan(ctx context.Context, ch chan<- string) {
	for _, dir := range w.dirs {
		err := filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
			if err != nil {
				return nil // skip inaccessible entries
			}
			if ctx.Err() != nil {
				return ctx.Err()
			}
			if d.IsDir() {
				return nil
			}

			lower := strings.ToLower(d.Name())
			if !strings.HasSuffix(lower, ".note") && !strings.HasSuffix(lower, ".md") {
				return nil
			}

			select {
			case ch <- path:
			case <-ctx.Done():
				return ctx.Err()
			}
			return nil
		})
		if err != nil && ctx.Err() == nil {
			slog.Warn("scan directory", "dir", dir, "error", err)
		}
	}
}
