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
	Telegram TelegramSettings `yaml:"telegram"`
	Watcher  WatcherSettings  `yaml:"watcher"`
	Granola  GranolaSettings  `yaml:"granola"`
	Auth     AuthSettings     `yaml:"auth"`

	// Resolved secrets (not serialized).
	openaiAPIKey        string
	whatsappAppSecret   string
	whatsappAccessToken string
	telegramBotToken    string
	granolaOAuthToken   string
}

// AuthSettings holds authentication configuration.
type AuthSettings struct {
	TOTP TOTPSettings `yaml:"totp"`
}

// TOTPSettings holds TOTP authenticator configuration.
type TOTPSettings struct {
	Enabled bool `yaml:"enabled"`
}

// TLS holds TLS certificate configuration.
type TLS struct {
	CertFile string `yaml:"cert_file"`
	KeyFile  string `yaml:"key_file"`
}

// WatcherSettings holds configuration for the file watcher.
type WatcherSettings struct {
	WatchDir     string   `yaml:"watch_dir"`      // backward compat (singular)
	WatchDirs    []string `yaml:"watch_dirs"`      // list of directories to watch
	PollInterval string   `yaml:"poll_interval"`   // e.g. "30s"
}

// ServerSettings holds general server settings.
type ServerSettings struct {
	Addr    string `yaml:"addr"`
	DataDir string `yaml:"data_dir"`
	TLS     TLS    `yaml:"tls"`
}

// OpenAISettings holds OpenAI-related configuration.
type OpenAISettings struct {
	APIKeyFile     string `yaml:"api_key_file"`
	Model          string `yaml:"model"`
	ChatModel      string `yaml:"chat_model"`
	EmbeddingModel string `yaml:"embedding_model"`
	OCRPrompt      string `yaml:"ocr_prompt"`
}

// TelegramSettings holds Telegram bot configuration.
type TelegramSettings struct {
	Enabled        bool    `yaml:"enabled"`
	BotTokenFile   string  `yaml:"bot_token_file"`
	AllowedChatIDs []int64 `yaml:"allowed_chat_ids"`
}

// GranolaSettings holds Granola meeting notes sync configuration.
// Uses the Granola MCP server (https://mcp.granola.ai/mcp) with OAuth.
type GranolaSettings struct {
	Enabled        bool   `yaml:"enabled"`
	OAuthTokenFile string `yaml:"oauth_token_file"`
	PollInterval   string `yaml:"poll_interval"` // e.g. "1h"
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

	// TLS validation: both or neither must be set.
	hasCert := c.Server.TLS.CertFile != ""
	hasKey := c.Server.TLS.KeyFile != ""
	if hasCert != hasKey {
		return fmt.Errorf("TLS config incomplete: both cert_file and key_file must be set")
	}
	// Expand ~ in TLS paths.
	if strings.HasPrefix(c.Server.TLS.CertFile, "~/") {
		if home, err := os.UserHomeDir(); err == nil {
			c.Server.TLS.CertFile = filepath.Join(home, c.Server.TLS.CertFile[2:])
		}
	}
	if strings.HasPrefix(c.Server.TLS.KeyFile, "~/") {
		if home, err := os.UserHomeDir(); err == nil {
			c.Server.TLS.KeyFile = filepath.Join(home, c.Server.TLS.KeyFile[2:])
		}
	}

	// Watcher defaults.
	if c.Watcher.PollInterval == "" {
		c.Watcher.PollInterval = "30s"
	}
	// Backward compat: merge singular watch_dir into watch_dirs.
	if c.Watcher.WatchDir != "" {
		found := false
		for _, d := range c.Watcher.WatchDirs {
			if d == c.Watcher.WatchDir {
				found = true
				break
			}
		}
		if !found {
			c.Watcher.WatchDirs = append(c.Watcher.WatchDirs, c.Watcher.WatchDir)
		}
	}
	// Expand ~ in all watch dirs.
	for i, d := range c.Watcher.WatchDirs {
		if strings.HasPrefix(d, "~/") {
			if home, err := os.UserHomeDir(); err == nil {
				c.Watcher.WatchDirs[i] = filepath.Join(home, d[2:])
			}
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

	// Telegram bot token: file > env var.
	if c.Telegram.BotTokenFile != "" {
		token, err := readSecretFile(c.Telegram.BotTokenFile)
		if err != nil {
			return err
		}
		c.telegramBotToken = token
	} else if t := os.Getenv("TELEGRAM_BOT_TOKEN"); t != "" {
		c.telegramBotToken = t
	}

	// Auto-enable Telegram if bot token is resolved.
	if c.telegramBotToken != "" {
		c.Telegram.Enabled = true
	}

	// Granola: auto-enable if a token file exists or env var is set.
	if c.Granola.OAuthTokenFile != "" {
		if _, err := os.Stat(c.Granola.OAuthTokenFile); err == nil {
			c.Granola.Enabled = true
		}
	}
	if token := os.Getenv("GRANOLA_OAUTH_TOKEN"); token != "" {
		c.granolaOAuthToken = token
		c.Granola.Enabled = true
	}

	// Granola poll interval default.
	if c.Granola.PollInterval == "" {
		c.Granola.PollInterval = "1h"
	}

	return nil
}

// APIKey returns the resolved OpenAI API key.
func (c *ServerConfig) APIKey() string { return c.openaiAPIKey }

// WhatsAppAppSecret returns the resolved WhatsApp app secret.
func (c *ServerConfig) WhatsAppAppSecret() string { return c.whatsappAppSecret }

// WhatsAppAccessToken returns the resolved WhatsApp access token.
func (c *ServerConfig) WhatsAppAccessToken() string { return c.whatsappAccessToken }

// TelegramBotToken returns the resolved Telegram bot token.
func (c *ServerConfig) TelegramBotToken() string { return c.telegramBotToken }

// GranolaOAuthToken returns the resolved Granola OAuth token.
func (c *ServerConfig) GranolaOAuthToken() string { return c.granolaOAuthToken }

// TLSEnabled returns true when both CertFile and KeyFile are configured.
func (c *ServerConfig) TLSEnabled() bool {
	return c.Server.TLS.CertFile != "" && c.Server.TLS.KeyFile != ""
}

// readSecretFile reads a file and returns its contents trimmed of whitespace.
func readSecretFile(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("reading secret file %s: %w", path, err)
	}
	return strings.TrimSpace(string(data)), nil
}
