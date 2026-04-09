package index

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

const (
	// qmdCommandTimeout is the default timeout for qmd CLI invocations.
	qmdCommandTimeout = 60 * time.Second
)

// QMDIndex implements indexing and search by delegating to the qmd CLI.
// Each entry is written as a markdown file in a managed directory, and qmd
// handles chunking and BM25 full-text search locally.
type QMDIndex struct {
	collection string // qmd collection name
	dataDir    string // directory where entry files are stored
	qmdPath    string // path to qmd binary (default: "qmd")

	mu sync.Mutex // serializes writes to the data directory
}

// QMDConfig holds configuration for the QMD index.
type QMDConfig struct {
	// Collection is the qmd collection name. Defaults to "blackwood".
	Collection string
	// DataDir is the directory where entry markdown files are stored.
	// Must be provided.
	DataDir string
	// QMDPath overrides the qmd binary path. Defaults to "qmd".
	QMDPath string
}

// NewQMD creates a QMDIndex, ensuring the data directory exists and the qmd
// collection is registered.
func NewQMD(ctx context.Context, cfg QMDConfig) (*QMDIndex, error) {
	if cfg.DataDir == "" {
		return nil, fmt.Errorf("QMD data directory is required")
	}
	if cfg.Collection == "" {
		cfg.Collection = "blackwood"
	}
	if cfg.QMDPath == "" {
		cfg.QMDPath = "qmd"
	}

	if err := os.MkdirAll(cfg.DataDir, 0o755); err != nil {
		return nil, fmt.Errorf("creating QMD data dir: %w", err)
	}

	idx := &QMDIndex{
		collection: cfg.Collection,
		dataDir:    cfg.DataDir,
		qmdPath:    cfg.QMDPath,
	}

	// Ensure the collection exists. `qmd collection add` is idempotent-ish:
	// if it already exists we ignore the error.
	if err := idx.ensureCollection(ctx); err != nil {
		return nil, fmt.Errorf("ensuring QMD collection: %w", err)
	}

	return idx, nil
}

// IndexEntry writes the entry text to a file and triggers qmd to re-index.
func (q *QMDIndex) IndexEntry(ctx context.Context, entryID string, text string) error {
	if text == "" {
		return nil
	}

	q.mu.Lock()
	defer q.mu.Unlock()

	if err := q.writeEntryFile(entryID, text); err != nil {
		return err
	}

	// Update the qmd FTS index.
	if err := q.runQMD(ctx, "update"); err != nil {
		return fmt.Errorf("qmd update: %w", err)
	}
	return nil
}

// Search delegates to `qmd search --json` (BM25 full-text search) and maps
// results back to entry IDs. Uses BM25 rather than hybrid `qmd query` to
// avoid loading GGUF models which require significant RAM and GPU.
func (q *QMDIndex) Search(ctx context.Context, query string, limit int) ([]SearchResult, error) {
	if limit <= 0 {
		limit = 10
	}

	out, err := q.runQMDOutput(ctx, "search", "--json",
		"-n", fmt.Sprintf("%d", limit),
		"-c", q.collection,
		query,
	)
	if err != nil {
		return nil, fmt.Errorf("qmd search: %w", err)
	}

	return parseQMDResults(out)
}

// DeleteEntry removes the entry file and updates the qmd index.
func (q *QMDIndex) DeleteEntry(entryID string) error {
	q.mu.Lock()
	defer q.mu.Unlock()

	path := q.entryPath(entryID)
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("removing entry file: %w", err)
	}

	// Update qmd so it drops the removed file.
	if err := q.runQMD(context.Background(), "update"); err != nil {
		slog.Warn("qmd update after delete", "entry_id", entryID, "error", err)
	}
	return nil
}

// Reindex writes all entries to disk and triggers a full qmd update.
func (q *QMDIndex) Reindex(ctx context.Context, entries []EntryForIndex) error {
	q.mu.Lock()
	defer q.mu.Unlock()

	for _, e := range entries {
		if e.Text == "" {
			continue
		}
		if err := q.writeEntryFile(e.ID, e.Text); err != nil {
			return err
		}
	}

	if err := q.runQMD(ctx, "update"); err != nil {
		return fmt.Errorf("qmd update: %w", err)
	}
	return nil
}

// --- internal helpers ---

func (q *QMDIndex) entryPath(entryID string) string {
	return filepath.Join(q.dataDir, entryID+".md")
}

func (q *QMDIndex) writeEntryFile(entryID, text string) error {
	path := q.entryPath(entryID)
	if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
		return fmt.Errorf("writing entry file %s: %w", entryID, err)
	}
	return nil
}

func (q *QMDIndex) ensureCollection(ctx context.Context) error {
	// Try to add; if it already exists qmd exits non-zero — that's fine.
	out, err := q.runQMDOutput(ctx, "collection", "add", q.dataDir, "--name", q.collection, "--mask", "**/*.md")
	if err != nil {
		// Check if the error is "already exists" — qmd prints this to stdout/stderr.
		if strings.Contains(out, "already") || strings.Contains(strings.ToLower(out), "exists") {
			return nil
		}
		return fmt.Errorf("qmd collection add: %s: %w", out, err)
	}
	return nil
}

func (q *QMDIndex) runQMD(ctx context.Context, args ...string) error {
	ctx, cancel := context.WithTimeout(ctx, qmdCommandTimeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, q.qmdPath, args...)
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("qmd %s: %w", strings.Join(args, " "), err)
	}
	return nil
}

func (q *QMDIndex) runQMDOutput(ctx context.Context, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, qmdCommandTimeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, q.qmdPath, args...)
	out, err := cmd.CombinedOutput()
	return string(out), err
}

// qmdJSONResult matches the JSON output of `qmd search --json`.
type qmdJSONResult struct {
	Results []qmdResultEntry `json:"results"`
}

type qmdResultEntry struct {
	DocID   string  `json:"docid"`
	File    string  `json:"file"`
	Score   float64 `json:"score"`
	Snippet string  `json:"snippet"`
	Title   string  `json:"title"`
}

// parseQMDResults extracts SearchResults from qmd JSON output.
// Entry IDs are derived from the filename (without .md extension).
func parseQMDResults(jsonOutput string) ([]SearchResult, error) {
	jsonOutput = strings.TrimSpace(jsonOutput)
	if jsonOutput == "" {
		return nil, nil
	}

	// qmd --json may output either a top-level array or an object with "results".
	// Try object first, then array.
	var wrapper qmdJSONResult
	if err := json.Unmarshal([]byte(jsonOutput), &wrapper); err == nil && len(wrapper.Results) > 0 {
		return mapQMDEntries(wrapper.Results), nil
	}

	var entries []qmdResultEntry
	if err := json.Unmarshal([]byte(jsonOutput), &entries); err != nil {
		return nil, fmt.Errorf("parsing qmd JSON output: %w", err)
	}
	return mapQMDEntries(entries), nil
}

func mapQMDEntries(entries []qmdResultEntry) []SearchResult {
	var results []SearchResult
	for _, e := range entries {
		entryID := entryIDFromPath(e.File)
		if entryID == "" {
			continue
		}
		snippet := e.Snippet
		if snippet == "" {
			snippet = e.Title
		}
		results = append(results, SearchResult{
			EntryID: entryID,
			Score:   e.Score,
			Snippet: truncate(snippet, 200),
		})
	}
	return results
}

// entryIDFromPath extracts the entry ID from a qmd file reference.
// Handles both filesystem paths ("entries/abc123.md") and qmd URIs
// ("qmd://collection/abc123.md").
func entryIDFromPath(path string) string {
	// Strip qmd:// URI prefix if present.
	if strings.HasPrefix(path, "qmd://") {
		// qmd://collection/filename.md → filename.md
		path = strings.TrimPrefix(path, "qmd://")
		if idx := strings.Index(path, "/"); idx >= 0 {
			path = path[idx+1:]
		}
	}
	base := filepath.Base(path)
	return strings.TrimSuffix(base, ".md")
}
