package hooks

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/csweichel/blackwood/internal/config"
)

func TestSingleHookSuccess(t *testing.T) {
	r := New([]config.Hook{
		{Command: "true"},
	})
	if err := r.Run(context.Background(), "/tmp/test.note"); err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
}

func TestSequentialOrder(t *testing.T) {
	// Write hook outputs to a file to verify ordering.
	tmp := filepath.Join(t.TempDir(), "order.txt")
	r := New([]config.Hook{
		{Command: "sh", Args: []string{"-c", "echo first >> " + tmp + " && true"}},
		{Command: "sh", Args: []string{"-c", "echo second >> " + tmp + " && true"}},
	})
	if err := r.Run(context.Background(), "/tmp/test.note"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	data, err := os.ReadFile(tmp)
	if err != nil {
		t.Fatalf("reading order file: %v", err)
	}
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(lines) != 2 || lines[0] != "first" || lines[1] != "second" {
		t.Fatalf("expected [first second], got %v", lines)
	}
}

func TestFailureStopsExecution(t *testing.T) {
	marker := filepath.Join(t.TempDir(), "should_not_exist")
	r := New([]config.Hook{
		{Command: "false"},
		{Command: "touch", Args: []string{marker}},
	})
	err := r.Run(context.Background(), "/tmp/test.note")
	if err == nil {
		t.Fatal("expected error from failing hook")
	}
	if _, statErr := os.Stat(marker); statErr == nil {
		t.Fatal("second hook should not have run after first hook failed")
	}
}

func TestBlackwoodFileEnv(t *testing.T) {
	out := filepath.Join(t.TempDir(), "env.txt")
	r := New([]config.Hook{
		{Command: "sh", Args: []string{"-c", "echo $BLACKWOOD_FILE > " + out + " && true"}},
	})
	filePath := "/some/path/note.note"
	if err := r.Run(context.Background(), filePath); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	data, err := os.ReadFile(out)
	if err != nil {
		t.Fatalf("reading env output: %v", err)
	}
	got := strings.TrimSpace(string(data))
	if got != filePath {
		t.Fatalf("BLACKWOOD_FILE = %q, want %q", got, filePath)
	}
}

func TestContextCancellation(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	r := New([]config.Hook{
		{Command: "sleep", Args: []string{"10"}},
	})
	err := r.Run(ctx, "/tmp/test.note")
	if err == nil {
		t.Fatal("expected error from cancelled context")
	}
}
