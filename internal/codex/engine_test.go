package codex

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type fakeRunner struct {
	results []RunResult
	errs    []error
	calls   []RunRequest
}

func (f *fakeRunner) Run(ctx context.Context, req RunRequest) (RunResult, error) {
	f.calls = append(f.calls, req)
	idx := len(f.calls) - 1
	if idx < len(f.errs) && f.errs[idx] != nil {
		return RunResult{}, f.errs[idx]
	}
	if idx < len(f.results) {
		return f.results[idx], nil
	}
	return RunResult{Stdout: `{"ok":true}`}, nil
}

func testCodexPath(t *testing.T) string {
	t.Helper()
	path, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	return path
}

func writeNote(t *testing.T, notesDir, date, content string) {
	t.Helper()
	parts := strings.Split(date, "-")
	if len(parts) != 3 {
		t.Fatalf("bad test date %q", date)
	}
	dir := filepath.Join(notesDir, parts[0], parts[1], parts[2])
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "index.md"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestEngineChatUsesNotesDirAndParsesSources(t *testing.T) {
	ctx := context.Background()
	notesDir := t.TempDir()
	writeNote(t, notesDir, "2026-06-01", "# Notes\nCodex integration notes")

	runner := &fakeRunner{
		results: []RunResult{
			{Stdout: `{"ok":true}`},
			{Stdout: `{"answer":"Use Codex.","sources":[{"date":"2026-06-01","snippet":"Codex integration notes","score":0.9}]}`},
		},
	}
	engine := NewWithRunner(ctx, Config{
		Path:     testCodexPath(t),
		NotesDir: notesDir,
	}, runner)

	answer, sources, err := engine.Chat(ctx, "What changed?", nil)
	if err != nil {
		t.Fatal(err)
	}
	if answer != "Use Codex." {
		t.Fatalf("answer = %q", answer)
	}
	if len(sources) != 1 || sources[0].DailyNoteDate != "2026-06-01" || sources[0].EntryID == "" {
		t.Fatalf("sources = %#v", sources)
	}
	if len(runner.calls) != 2 {
		t.Fatalf("runner calls = %d, want 2", len(runner.calls))
	}
	call := runner.calls[1]
	if call.Dir != notesDir {
		t.Fatalf("codex dir = %q, want %q", call.Dir, notesDir)
	}
	if strings.Contains(call.Stdin, notesDir) {
		t.Fatalf("prompt leaked absolute notes dir: %s", call.Stdin)
	}
	args := strings.Join(call.Args, " ")
	if !strings.Contains(args, `sandbox_mode="read-only"`) || !strings.Contains(args, `approval_policy="never"`) {
		t.Fatalf("codex args do not request read-only/no-approval policy: %#v", call.Args)
	}
}

func TestEngineSearchParsesResults(t *testing.T) {
	ctx := context.Background()
	notesDir := t.TempDir()
	writeNote(t, notesDir, "2026-06-01", "# Notes\nSearch me")

	runner := &fakeRunner{
		results: []RunResult{
			{Stdout: `{"ok":true}`},
			{Stdout: "```json\n{\"results\":[{\"date\":\"2026-06-01\",\"snippet\":\"Search me\",\"score\":0.8}]}\n```"},
		},
	}
	engine := NewWithRunner(ctx, Config{Path: testCodexPath(t), NotesDir: notesDir}, runner)

	results, err := engine.Search(ctx, "search", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 || results[0].Date != "2026-06-01" || results[0].Snippet != "Search me" {
		t.Fatalf("results = %#v", results)
	}
}

func TestEngineSummaryParsesResult(t *testing.T) {
	ctx := context.Background()
	notesDir := t.TempDir()

	runner := &fakeRunner{
		results: []RunResult{
			{Stdout: `{"ok":true}`},
			{Stdout: `{"summary":"Codex work replaced note chat."}`},
		},
	}
	engine := NewWithRunner(ctx, Config{Path: testCodexPath(t), NotesDir: notesDir}, runner)

	summary, err := engine.Summarize(ctx, "# Summary\nold\n\n# Notes\nCodex work replaced note chat.")
	if err != nil {
		t.Fatal(err)
	}
	if summary != "Codex work replaced note chat." {
		t.Fatalf("summary = %q", summary)
	}
	if strings.Contains(runner.calls[1].Stdin, "# Summary\nold") {
		t.Fatalf("summary prompt included existing summary: %s", runner.calls[1].Stdin)
	}
}

func TestEngineUnavailableWhenDisabled(t *testing.T) {
	enabled := false
	runner := &fakeRunner{}
	engine := NewWithRunner(context.Background(), Config{
		Enabled:  &enabled,
		Path:     testCodexPath(t),
		NotesDir: t.TempDir(),
	}, runner)

	if engine.Available() {
		t.Fatal("engine should be unavailable")
	}
	if len(runner.calls) != 0 {
		t.Fatalf("runner was called despite disabled config")
	}
}

func TestEngineUnavailableWhenProbeFails(t *testing.T) {
	runner := &fakeRunner{errs: []error{errors.New("auth required")}}
	engine := NewWithRunner(context.Background(), Config{
		Path:     testCodexPath(t),
		NotesDir: t.TempDir(),
	}, runner)

	if engine.Available() {
		t.Fatal("engine should be unavailable")
	}
	if !strings.Contains(engine.UnavailableReason(), "auth required") {
		t.Fatalf("unavailable reason = %q", engine.UnavailableReason())
	}
}

func TestCorpusSizeLimit(t *testing.T) {
	ctx := context.Background()
	notesDir := t.TempDir()
	writeNote(t, notesDir, "2026-06-01", "too large")

	runner := &fakeRunner{
		results: []RunResult{
			{Stdout: `{"ok":true}`},
		},
	}
	engine := NewWithRunner(ctx, Config{
		Path:           testCodexPath(t),
		NotesDir:       notesDir,
		MaxCorpusBytes: 1,
	}, runner)

	if _, err := engine.Search(ctx, "query", 10); !errors.Is(err, ErrCorpusTooLarge) {
		t.Fatalf("Search error = %v, want ErrCorpusTooLarge", err)
	}
	if len(runner.calls) != 1 {
		t.Fatalf("runner calls = %d, want only probe call", len(runner.calls))
	}
}
