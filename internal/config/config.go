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
	WatchDir     string        `yaml:"watch_dir"`
	Hooks        []Hook        `yaml:"hooks"`
	PollInterval time.Duration `yaml:"poll_interval"`
	StatePath    string        `yaml:"state_path"`
}

// Hook defines a command to run when a file change is detected.
type Hook struct {
	Command string   `yaml:"command"`
	Args    []string `yaml:"args"`
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

// Validate checks that the configuration is usable. It applies defaults
// where possible (e.g. PollInterval defaults to 30s).
func (c *Config) Validate() error {
	if len(c.Hooks) == 0 {
		return errors.New("at least one hook is required")
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
	return nil
}
