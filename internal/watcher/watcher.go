package watcher

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Watcher polls a directory for new .note files.
type Watcher struct {
	watchDir     string
	pollInterval time.Duration
	seen         map[string]struct{}
}

// New creates a Watcher that polls watchDir every pollInterval.
func New(watchDir string, pollInterval time.Duration) *Watcher {
	return &Watcher{
		watchDir:     watchDir,
		pollInterval: pollInterval,
		seen:         make(map[string]struct{}),
	}
}

// Start begins polling and returns a channel that emits paths of newly
// discovered .note files. The channel is closed when ctx is cancelled.
func (w *Watcher) Start(ctx context.Context) (<-chan string, error) {
	info, err := os.Stat(w.watchDir)
	if err != nil {
		return nil, fmt.Errorf("watch directory: %w", err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("watch directory %s is not a directory", w.watchDir)
	}

	ch := make(chan string)
	go func() {
		defer close(ch)

		// Do an initial scan before waiting for the first tick.
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

// scan lists the watch directory and sends any unseen .note files on ch.
func (w *Watcher) scan(ctx context.Context, ch chan<- string) {
	entries, err := os.ReadDir(w.watchDir)
	if err != nil {
		return
	}

	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		if !strings.HasSuffix(strings.ToLower(e.Name()), ".note") {
			continue
		}

		fullPath := filepath.Join(w.watchDir, e.Name())
		if _, ok := w.seen[fullPath]; ok {
			continue
		}
		w.seen[fullPath] = struct{}{}

		select {
		case ch <- fullPath:
		case <-ctx.Done():
			return
		}
	}
}
