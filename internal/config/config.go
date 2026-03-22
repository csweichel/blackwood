package config

import (
	"errors"
	"fmt"
	"os"
	"time"

	"gopkg.in/yaml.v3"
)

// Config holds the top-level application configuration.
type Config struct {
	WatchDir     string         `yaml:"watch_dir"`
	Hooks        []Hook         `yaml:"hooks"`
	PollInterval time.Duration  `yaml:"poll_interval"`
	StatePath    string         `yaml:"state_path"`
	Obsidian     ObsidianConfig `yaml:"obsidian"`
	LLM          LLMConfig      `yaml:"llm"`
	Dropbox      DropboxConfig  `yaml:"dropbox"`
}

// Hook defines a command to run when a file change is detected.
type Hook struct {
	Command string   `yaml:"command"`
	Args    []string `yaml:"args"`
}

// ObsidianConfig holds settings for Obsidian vault integration.
type ObsidianConfig struct {
	VaultPath      string `yaml:"vault_path"`
	DailyNotesDir  string `yaml:"daily_notes_dir"`
	DailyFormat    string `yaml:"daily_format"`
	AttachmentsDir string `yaml:"attachments_dir"`
}

// LLMConfig holds settings for the LLM provider used for OCR.
type LLMConfig struct {
	Provider  string `yaml:"provider"`
	Model     string `yaml:"model"`
	APIKeyEnv string `yaml:"api_key_env"`
	Prompt    string `yaml:"prompt"`
}

// DropboxConfig holds settings for Dropbox-synced folder integration.
type DropboxConfig struct {
	LocalPath string `yaml:"local_path"`
}

// Load reads a YAML configuration file from path and returns the parsed Config.
func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading config file: %w", err)
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parsing config file: %w", err)
	}

	return &cfg, nil
}

// hasObsidianConfig returns true if the Obsidian section has been configured.
func (c *Config) hasObsidianConfig() bool {
	return c.Obsidian.VaultPath != ""
}

// Validate checks that the configuration is usable. It applies defaults
// where possible (e.g. PollInterval defaults to 30s).
func (c *Config) Validate() error {
	// Either hooks or obsidian config must be present.
	if len(c.Hooks) == 0 && !c.hasObsidianConfig() {
		return errors.New("at least one hook or obsidian config is required")
	}

	if c.PollInterval == 0 {
		c.PollInterval = 30 * time.Second
	}
	if c.PollInterval < 0 {
		return fmt.Errorf("poll_interval must be positive, got %s", c.PollInterval)
	}
	if c.StatePath == "" {
		return errors.New("state_path is required")
	}

	// Dropbox: cannot set both watch_dir and dropbox.local_path.
	if c.WatchDir != "" && c.Dropbox.LocalPath != "" {
		return errors.New("watch_dir and dropbox.local_path are mutually exclusive")
	}
	// If dropbox.local_path is set and watch_dir is empty, use it as watch_dir.
	if c.Dropbox.LocalPath != "" && c.WatchDir == "" {
		c.WatchDir = c.Dropbox.LocalPath
	}

	// Obsidian defaults.
	if c.hasObsidianConfig() {
		if c.Obsidian.DailyNotesDir == "" {
			c.Obsidian.DailyNotesDir = "Daily Notes"
		}
		if c.Obsidian.DailyFormat == "" {
			c.Obsidian.DailyFormat = "2006-01-02"
		}
		if c.Obsidian.AttachmentsDir == "" {
			c.Obsidian.AttachmentsDir = "attachments"
		}

		// Verify vault path exists.
		info, err := os.Stat(c.Obsidian.VaultPath)
		if err != nil {
			return fmt.Errorf("obsidian vault_path: %w", err)
		}
		if !info.IsDir() {
			return fmt.Errorf("obsidian vault_path %q is not a directory", c.Obsidian.VaultPath)
		}
	}

	// LLM defaults.
	if c.LLM.Prompt == "" {
		c.LLM.Prompt = "You are an OCR assistant. Extract all handwritten text from this image. Return only the recognized text, preserving paragraph structure. If you cannot read something, indicate it with [illegible]."
	}

	// Verify LLM API key env var is set if configured.
	if c.LLM.APIKeyEnv != "" {
		if os.Getenv(c.LLM.APIKeyEnv) == "" {
			return fmt.Errorf("environment variable %q (llm.api_key_env) is not set", c.LLM.APIKeyEnv)
		}
	}

	return nil
}
