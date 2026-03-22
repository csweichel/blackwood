package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func writeTestConfig(t *testing.T, content string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestLoad_ValidConfig(t *testing.T) {
	yaml := `
watch_dir: /tmp/notes
state_path: /tmp/state.json
poll_interval: 10s
hooks:
  - command: echo
    args: ["hello", "world"]
  - command: notify-send
    args: ["new file"]
`
	path := writeTestConfig(t, yaml)
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.WatchDir != "/tmp/notes" {
		t.Errorf("WatchDir = %q, want %q", cfg.WatchDir, "/tmp/notes")
	}
	if cfg.StatePath != "/tmp/state.json" {
		t.Errorf("StatePath = %q, want %q", cfg.StatePath, "/tmp/state.json")
	}
	if cfg.PollInterval != 10*time.Second {
		t.Errorf("PollInterval = %v, want %v", cfg.PollInterval, 10*time.Second)
	}
	if len(cfg.Hooks) != 2 {
		t.Fatalf("len(Hooks) = %d, want 2", len(cfg.Hooks))
	}
	if cfg.Hooks[0].Command != "echo" {
		t.Errorf("Hooks[0].Command = %q, want %q", cfg.Hooks[0].Command, "echo")
	}
	if len(cfg.Hooks[0].Args) != 2 || cfg.Hooks[0].Args[0] != "hello" {
		t.Errorf("Hooks[0].Args = %v, want [hello world]", cfg.Hooks[0].Args)
	}
}

func TestLoad_FileNotFound(t *testing.T) {
	_, err := Load("/nonexistent/config.yaml")
	if err == nil {
		t.Fatal("expected error for missing file")
	}
}

func TestValidate_NoHooks(t *testing.T) {
	cfg := &Config{
		WatchDir:  "/tmp",
		StatePath: "/tmp/state.json",
		Hooks:     nil,
	}
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected error for no hooks")
	}
}

func TestValidate_MissingStatePath(t *testing.T) {
	cfg := &Config{
		WatchDir: "/tmp",
		Hooks:    []Hook{{Command: "echo"}},
	}
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected error for missing state_path")
	}
}

func TestValidate_DefaultPollInterval(t *testing.T) {
	cfg := &Config{
		WatchDir:  "/tmp",
		StatePath: "/tmp/state.json",
		Hooks:     []Hook{{Command: "echo"}},
	}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.PollInterval != 30*time.Second {
		t.Errorf("PollInterval = %v, want %v (default)", cfg.PollInterval, 30*time.Second)
	}
}

func TestValidate_NegativePollInterval(t *testing.T) {
	cfg := &Config{
		WatchDir:     "/tmp",
		StatePath:    "/tmp/state.json",
		PollInterval: -1 * time.Second,
		Hooks:        []Hook{{Command: "echo"}},
	}
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected error for negative poll_interval")
	}
}
