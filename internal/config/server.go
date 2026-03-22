package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// ServerConfig holds configuration for the blackwood server.
type ServerConfig struct {
	Server   ServerSettings   `yaml:"server"`
	OpenAI   OpenAISettings   `yaml:"openai"`
	WhatsApp WhatsAppSettings `yaml:"whatsapp"`
	Watcher  WatcherSettings  `yaml:"watcher"`

	// Resolved secrets (not serialized).
	openaiAPIKey       string
	whatsappAppSecret  string
	whatsappAccessToken string
}

// WatcherSettings holds configuration for the Viwoods file watcher.
type WatcherSettings struct {
	WatchDir     string `yaml:"watch_dir"`
	PollInterval string `yaml:"poll_interval"` // e.g. "30s"
}

// ServerSettings holds general server settings.
type ServerSettings struct {
	Addr    string `yaml:"addr"`
	DataDir string `yaml:"data_dir"`
}

// OpenAISettings holds OpenAI-related configuration.
type OpenAISettings struct {
	APIKeyFile     string `yaml:"api_key_file"`
	Model          string `yaml:"model"`
	ChatModel      string `yaml:"chat_model"`
	EmbeddingModel string `yaml:"embedding_model"`
	OCRPrompt      string `yaml:"ocr_prompt"`
}

// WhatsAppSettings holds WhatsApp webhook configuration.
type WhatsAppSettings struct {
	Enabled         bool   `yaml:"enabled"`
	VerifyToken     string `yaml:"verify_token"`
	AppSecretFile   string `yaml:"app_secret_file"`
	AccessTokenFile string `yaml:"access_token_file"`
	PhoneNumberID   string `yaml:"phone_number_id"`
}

// LoadServerConfig loads the server config from a YAML file.
// If path is empty, returns a config with zero values (use Resolve to apply defaults).
func LoadServerConfig(path string) (*ServerConfig, error) {
	if path == "" {
		return &ServerConfig{}, nil
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading server config: %w", err)
	}

	var cfg ServerConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parsing server config: %w", err)
	}
	return &cfg, nil
}

// Resolve reads secret files, applies env var fallbacks, and sets defaults.
// Priority: config file value > env var > default.
func (c *ServerConfig) Resolve() error {
	// Server defaults.
	if c.Server.Addr == "" {
		c.Server.Addr = ":8080"
	}
	if c.Server.DataDir == "" {
		c.Server.DataDir = "~/.blackwood"
	}
	if strings.HasPrefix(c.Server.DataDir, "~/") {
		home, err := os.UserHomeDir()
		if err == nil {
			c.Server.DataDir = filepath.Join(home, c.Server.DataDir[2:])
		}
	}

	// Watcher defaults.
	if c.Watcher.PollInterval == "" {
		c.Watcher.PollInterval = "30s"
	}
	if strings.HasPrefix(c.Watcher.WatchDir, "~/") {
		home, err := os.UserHomeDir()
		if err == nil {
			c.Watcher.WatchDir = filepath.Join(home, c.Watcher.WatchDir[2:])
		}
	}

	// OpenAI defaults.
	if c.OpenAI.Model == "" {
		if m := os.Getenv("OPENAI_MODEL"); m != "" {
			c.OpenAI.Model = m
		} else {
			c.OpenAI.Model = "gpt-5.2"
		}
	}
	if c.OpenAI.ChatModel == "" {
		if m := os.Getenv("OPENAI_CHAT_MODEL"); m != "" {
			c.OpenAI.ChatModel = m
		} else {
			c.OpenAI.ChatModel = c.OpenAI.Model
		}
	}
	if c.OpenAI.EmbeddingModel == "" {
		c.OpenAI.EmbeddingModel = "text-embedding-3-small"
	}
	if c.OpenAI.OCRPrompt == "" {
		if p := os.Getenv("OPENAI_OCR_PROMPT"); p != "" {
			c.OpenAI.OCRPrompt = p
		} else {
			c.OpenAI.OCRPrompt = "Transcribe the handwritten text in this image. Return only the transcribed text, no commentary."
		}
	}

	// OpenAI API key: file > env var.
	if c.OpenAI.APIKeyFile != "" {
		key, err := readSecretFile(c.OpenAI.APIKeyFile)
		if err != nil {
			return err
		}
		c.openaiAPIKey = key
	} else if key := os.Getenv("OPENAI_API_KEY"); key != "" {
		c.openaiAPIKey = key
	}

	// WhatsApp: env var fallbacks for non-secret fields.
	if c.WhatsApp.VerifyToken == "" {
		if t := os.Getenv("WHATSAPP_VERIFY_TOKEN"); t != "" {
			c.WhatsApp.VerifyToken = t
		}
	}
	if c.WhatsApp.PhoneNumberID == "" {
		if id := os.Getenv("WHATSAPP_PHONE_NUMBER_ID"); id != "" {
			c.WhatsApp.PhoneNumberID = id
		}
	}

	// WhatsApp app secret: file > env var.
	if c.WhatsApp.AppSecretFile != "" {
		secret, err := readSecretFile(c.WhatsApp.AppSecretFile)
		if err != nil {
			return err
		}
		c.whatsappAppSecret = secret
	} else if s := os.Getenv("WHATSAPP_APP_SECRET"); s != "" {
		c.whatsappAppSecret = s
	}

	// WhatsApp access token: file > env var.
	if c.WhatsApp.AccessTokenFile != "" {
		token, err := readSecretFile(c.WhatsApp.AccessTokenFile)
		if err != nil {
			return err
		}
		c.whatsappAccessToken = token
	} else if t := os.Getenv("WHATSAPP_ACCESS_TOKEN"); t != "" {
		c.whatsappAccessToken = t
	}

	// Auto-enable WhatsApp if verify token is set.
	if c.WhatsApp.VerifyToken != "" {
		c.WhatsApp.Enabled = true
	}

	return nil
}

// APIKey returns the resolved OpenAI API key.
func (c *ServerConfig) APIKey() string { return c.openaiAPIKey }

// WhatsAppAppSecret returns the resolved WhatsApp app secret.
func (c *ServerConfig) WhatsAppAppSecret() string { return c.whatsappAppSecret }

// WhatsAppAccessToken returns the resolved WhatsApp access token.
func (c *ServerConfig) WhatsAppAccessToken() string { return c.whatsappAccessToken }

// readSecretFile reads a file and returns its contents trimmed of whitespace.
func readSecretFile(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("reading secret file %s: %w", path, err)
	}
	return strings.TrimSpace(string(data)), nil
}
