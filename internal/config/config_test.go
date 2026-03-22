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

func TestValidate_NoHooksNoObsidian(t *testing.T) {
	cfg := &Config{
		WatchDir:  "/tmp",
		StatePath: "/tmp/state.json",
		Hooks:     nil,
	}
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected error when neither hooks nor obsidian config is present")
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

func TestValidate_ObsidianDefaults(t *testing.T) {
	vaultDir := t.TempDir()
	cfg := &Config{
		WatchDir:  "/tmp",
		StatePath: "/tmp/state.json",
		Obsidian: ObsidianConfig{
			VaultPath: vaultDir,
		},
	}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Obsidian.DailyNotesDir != "Daily Notes" {
		t.Errorf("DailyNotesDir = %q, want %q", cfg.Obsidian.DailyNotesDir, "Daily Notes")
	}
	if cfg.Obsidian.DailyFormat != "2006-01-02" {
		t.Errorf("DailyFormat = %q, want %q", cfg.Obsidian.DailyFormat, "2006-01-02")
	}
	if cfg.Obsidian.AttachmentsDir != "attachments" {
		t.Errorf("AttachmentsDir = %q, want %q", cfg.Obsidian.AttachmentsDir, "attachments")
	}
}

func TestValidate_ObsidianVaultPathMustExist(t *testing.T) {
	cfg := &Config{
		WatchDir:  "/tmp",
		StatePath: "/tmp/state.json",
		Obsidian: ObsidianConfig{
			VaultPath: "/nonexistent/vault",
		},
	}
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected error for nonexistent vault path")
	}
}

func TestValidate_ObsidianVaultPathMustBeDir(t *testing.T) {
	// Create a file (not a directory) to use as vault path.
	f, err := os.CreateTemp(t.TempDir(), "fakevault")
	if err != nil {
		t.Fatal(err)
	}
	f.Close()

	cfg := &Config{
		WatchDir:  "/tmp",
		StatePath: "/tmp/state.json",
		Obsidian: ObsidianConfig{
			VaultPath: f.Name(),
		},
	}
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected error when vault path is a file, not a directory")
	}
}

func TestValidate_ObsidianWithoutHooks(t *testing.T) {
	vaultDir := t.TempDir()
	cfg := &Config{
		WatchDir:  "/tmp",
		StatePath: "/tmp/state.json",
		Obsidian: ObsidianConfig{
			VaultPath: vaultDir,
		},
	}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("config with obsidian but no hooks should be valid, got: %v", err)
	}
}

func TestValidate_DropboxPopulatesWatchDir(t *testing.T) {
	cfg := &Config{
		StatePath: "/tmp/state.json",
		Hooks:     []Hook{{Command: "echo"}},
		Dropbox: DropboxConfig{
			LocalPath: "/home/user/Dropbox/Apps/Viwoods",
		},
	}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.WatchDir != "/home/user/Dropbox/Apps/Viwoods" {
		t.Errorf("WatchDir = %q, want %q", cfg.WatchDir, "/home/user/Dropbox/Apps/Viwoods")
	}
}

func TestValidate_DropboxAndWatchDirConflict(t *testing.T) {
	cfg := &Config{
		WatchDir:  "/tmp/notes",
		StatePath: "/tmp/state.json",
		Hooks:     []Hook{{Command: "echo"}},
		Dropbox: DropboxConfig{
			LocalPath: "/home/user/Dropbox/Apps/Viwoods",
		},
	}
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected error when both watch_dir and dropbox.local_path are set")
	}
}

func TestValidate_LLMDefaultPrompt(t *testing.T) {
	cfg := &Config{
		WatchDir:  "/tmp",
		StatePath: "/tmp/state.json",
		Hooks:     []Hook{{Command: "echo"}},
	}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.LLM.Prompt == "" {
		t.Error("expected default LLM prompt to be set")
	}
}

func TestValidate_LLMAPIKeyEnvMissing(t *testing.T) {
	// Use a unique env var name that is guaranteed to be unset.
	envVar := "BLACKWOOD_TEST_MISSING_KEY_12345"
	os.Unsetenv(envVar)

	cfg := &Config{
		WatchDir:  "/tmp",
		StatePath: "/tmp/state.json",
		Hooks:     []Hook{{Command: "echo"}},
		LLM: LLMConfig{
			APIKeyEnv: envVar,
		},
	}
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected error when api_key_env references an unset variable")
	}
}

func TestValidate_LLMAPIKeyEnvSet(t *testing.T) {
	envVar := "BLACKWOOD_TEST_KEY_SET_12345"
	t.Setenv(envVar, "sk-test-key")

	cfg := &Config{
		WatchDir:  "/tmp",
		StatePath: "/tmp/state.json",
		Hooks:     []Hook{{Command: "echo"}},
		LLM: LLMConfig{
			APIKeyEnv: envVar,
		},
	}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}
