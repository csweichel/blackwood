package codex

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

var (
	// ErrUnavailable is returned when Codex-backed functionality is disabled.
	ErrUnavailable    = errors.New("codex is not available")
	ErrCorpusTooLarge = errors.New("note corpus exceeds codex limit")
)

// Config controls the Codex CLI integration.
type Config struct {
	Enabled        *bool
	Path           string
	NotesDir       string
	Timeout        time.Duration
	MaxCorpusBytes int64
	MaxOutputBytes int64
	ExtraArgs      []string
}

// Message is a prior conversation message.
type Message struct {
	Role    string
	Content string
}

// SourceReference identifies a note excerpt used by Codex.
type SourceReference struct {
	EntryID       string
	DailyNoteDate string
	Snippet       string
	Score         float64
}

// SearchResult is a Codex-backed search result.
type SearchResult struct {
	EntryID string
	Date    string
	Snippet string
	Score   float64
}

// Engine provides Codex-backed chat, search, and summaries.
type Engine struct {
	cfg         Config
	runner      Runner
	env         []string
	path        string
	available   bool
	unavailable string
}

// New creates and probes a Codex engine.
func New(ctx context.Context, cfg Config) *Engine {
	return NewWithRunner(ctx, cfg, CLIRunner{})
}

// NewWithRunner creates and probes a Codex engine with a custom runner.
func NewWithRunner(ctx context.Context, cfg Config, runner Runner) *Engine {
	if cfg.Path == "" {
		cfg.Path = "codex"
	}
	if cfg.Timeout <= 0 {
		cfg.Timeout = 2 * time.Minute
	}
	if cfg.MaxCorpusBytes <= 0 {
		cfg.MaxCorpusBytes = 10 << 20
	}
	if cfg.MaxOutputBytes <= 0 {
		cfg.MaxOutputBytes = 1 << 20
	}
	e := &Engine{
		cfg:    cfg,
		runner: runner,
		env:    safeEnv(),
	}
	e.probe(ctx)
	return e
}

// Available reports whether Codex is ready for note interactions.
func (e *Engine) Available() bool {
	return e != nil && e.available
}

// UnavailableReason returns the startup probe failure, if any.
func (e *Engine) UnavailableReason() string {
	if e == nil {
		return "codex engine is nil"
	}
	return e.unavailable
}

func (e *Engine) probe(ctx context.Context) {
	if e.cfg.Enabled != nil && !*e.cfg.Enabled {
		e.disable("disabled by configuration")
		return
	}

	path, err := resolvePath(e.cfg.Path)
	if err != nil {
		e.disable(err.Error())
		return
	}
	e.path = path

	if e.cfg.NotesDir == "" {
		e.disable("notes directory is not configured")
		return
	}
	if err := os.MkdirAll(e.cfg.NotesDir, 0o755); err != nil {
		e.disable(fmt.Sprintf("create notes directory: %v", err))
		return
	}

	ctx, cancel := context.WithTimeout(ctx, e.cfg.Timeout)
	defer cancel()
	out, err := e.runRaw(ctx, "Return exactly this JSON and nothing else: {\"ok\":true}")
	if err != nil {
		e.disable(err.Error())
		return
	}
	var probe struct {
		OK bool `json:"ok"`
	}
	if err := parseJSON(out, &probe); err != nil || !probe.OK {
		if err == nil {
			err = errors.New("probe returned ok=false")
		}
		e.disable(fmt.Sprintf("parse probe response: %v", err))
		return
	}
	e.available = true
}

func (e *Engine) disable(reason string) {
	e.available = false
	e.unavailable = reason
}

func resolvePath(path string) (string, error) {
	if strings.ContainsRune(path, os.PathSeparator) {
		if _, err := os.Stat(path); err != nil {
			return "", fmt.Errorf("codex executable %q is not available: %w", path, err)
		}
		return path, nil
	}
	resolved, err := exec.LookPath(path)
	if err != nil {
		return "", fmt.Errorf("codex executable %q is not available: %w", path, err)
	}
	return resolved, nil
}

func (e *Engine) run(ctx context.Context, prompt string) (string, error) {
	if !e.Available() {
		if e.unavailable != "" {
			return "", fmt.Errorf("%w: %s", ErrUnavailable, e.unavailable)
		}
		return "", ErrUnavailable
	}
	ctx, cancel := context.WithTimeout(ctx, e.cfg.Timeout)
	defer cancel()
	return e.runRaw(ctx, prompt)
}

func (e *Engine) runRaw(ctx context.Context, prompt string) (string, error) {
	args := []string{
		"exec",
		"--skip-git-repo-check",
		"-c", `sandbox_mode="read-only"`,
		"-c", `approval_policy="never"`,
	}
	args = append(args, e.cfg.ExtraArgs...)

	result, err := e.runner.Run(ctx, RunRequest{
		Path:           e.path,
		Args:           args,
		Dir:            e.cfg.NotesDir,
		Stdin:          prompt,
		Env:            e.env,
		MaxOutputBytes: e.cfg.MaxOutputBytes,
	})
	if err != nil {
		detail := strings.TrimSpace(result.Stderr)
		if detail == "" {
			detail = strings.TrimSpace(result.Stdout)
		}
		if detail != "" {
			return "", fmt.Errorf("%w: %s", err, detail)
		}
		return "", err
	}
	if strings.TrimSpace(result.Stdout) == "" {
		return "", errors.New("codex returned empty output")
	}
	return result.Stdout, nil
}

// Chat answers a user message from notes.
func (e *Engine) Chat(ctx context.Context, query string, history []Message) (string, []SourceReference, error) {
	manifest, err := e.manifest(ctx)
	if err != nil {
		return "", nil, err
	}
	prompt, err := chatPrompt(query, history, manifest)
	if err != nil {
		return "", nil, err
	}
	out, err := e.run(ctx, prompt)
	if err != nil {
		return "", nil, err
	}
	var parsed chatOutput
	if err := parseJSON(out, &parsed); err != nil {
		return "", nil, fmt.Errorf("parse codex chat response: %w", err)
	}
	answer := strings.TrimSpace(parsed.Answer)
	if answer == "" {
		return "", nil, errors.New("codex chat response missing answer")
	}
	sources := make([]SourceReference, 0, len(parsed.Sources))
	for _, s := range parsed.Sources {
		sources = append(sources, normalizeSource(s.Date, s.Snippet, s.Score))
	}
	return answer, sources, nil
}

// Search returns Codex-ranked note matches.
func (e *Engine) Search(ctx context.Context, query string, limit int) ([]SearchResult, error) {
	if limit <= 0 {
		limit = 20
	}
	manifest, err := e.manifest(ctx)
	if err != nil {
		return nil, err
	}
	prompt, err := searchPrompt(query, limit, manifest)
	if err != nil {
		return nil, err
	}
	out, err := e.run(ctx, prompt)
	if err != nil {
		return nil, err
	}
	var parsed searchOutput
	if err := parseJSON(out, &parsed); err != nil {
		return nil, fmt.Errorf("parse codex search response: %w", err)
	}
	results := make([]SearchResult, 0, len(parsed.Results))
	for _, r := range parsed.Results {
		src := normalizeSource(r.Date, r.Snippet, r.Score)
		results = append(results, SearchResult{
			EntryID: src.EntryID,
			Date:    src.DailyNoteDate,
			Snippet: src.Snippet,
			Score:   src.Score,
		})
		if len(results) >= limit {
			break
		}
	}
	return results, nil
}

// Summarize creates a one-line daily-note summary through Codex.
func (e *Engine) Summarize(ctx context.Context, content string) (string, error) {
	if strings.TrimSpace(content) == "" {
		return "", errors.New("empty content")
	}
	prompt, err := summaryPrompt(content)
	if err != nil {
		return "", err
	}
	out, err := e.run(ctx, prompt)
	if err != nil {
		return "", err
	}
	var parsed summaryOutput
	if err := parseJSON(out, &parsed); err != nil {
		return "", fmt.Errorf("parse codex summary response: %w", err)
	}
	summary := strings.TrimSpace(parsed.Summary)
	if summary == "" {
		return "", errors.New("codex summary response missing summary")
	}
	return summary, nil
}

type noteManifest struct {
	Notes []noteFile `json:"notes"`
}

type noteFile struct {
	Path string `json:"path"`
	Date string `json:"date,omitempty"`
	Size int64  `json:"size"`
}

func (e *Engine) manifest(ctx context.Context) (noteManifest, error) {
	var manifest noteManifest
	var total int64
	err := filepath.WalkDir(e.cfg.NotesDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		if d.IsDir() || !strings.HasSuffix(d.Name(), ".md") {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		total += info.Size()
		if total > e.cfg.MaxCorpusBytes {
			return fmt.Errorf("%w: %d > %d", ErrCorpusTooLarge, total, e.cfg.MaxCorpusBytes)
		}
		rel, err := filepath.Rel(e.cfg.NotesDir, path)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		manifest.Notes = append(manifest.Notes, noteFile{
			Path: rel,
			Date: dateFromRelPath(rel),
			Size: info.Size(),
		})
		return nil
	})
	if err != nil {
		return noteManifest{}, fmt.Errorf("build notes manifest: %w", err)
	}
	sort.Slice(manifest.Notes, func(i, j int) bool {
		return manifest.Notes[i].Path < manifest.Notes[j].Path
	})
	return manifest, nil
}

func dateFromRelPath(path string) string {
	parts := strings.Split(path, "/")
	if len(parts) >= 4 && parts[3] == "index.md" {
		return parts[0] + "-" + parts[1] + "-" + parts[2]
	}
	if len(parts) >= 3 {
		return parts[0] + "-" + parts[1] + "-" + parts[2]
	}
	return ""
}

func normalizeSource(date, snippet string, score float64) SourceReference {
	snippet = strings.TrimSpace(snippet)
	if len(snippet) > 500 {
		snippet = snippet[:500]
	}
	if score < 0 {
		score = 0
	}
	if score > 1 {
		score = 1
	}
	return SourceReference{
		EntryID:       syntheticEntryID(date, snippet),
		DailyNoteDate: strings.TrimSpace(date),
		Snippet:       snippet,
		Score:         score,
	}
}

func syntheticEntryID(date, snippet string) string {
	sum := sha1.Sum([]byte(date + "\x00" + snippet))
	return "codex:" + hex.EncodeToString(sum[:8])
}

func manifestJSON(manifest noteManifest) (string, error) {
	data, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return "", fmt.Errorf("marshal notes manifest: %w", err)
	}
	return string(data), nil
}
