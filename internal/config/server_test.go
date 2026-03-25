package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadServerConfig(t *testing.T) {
	content := `
server:
  addr: ":9090"
  data_dir: /tmp/blackwood-test
openai:
  api_key_file: /tmp/test-key
  model: gpt-5.2
  chat_model: gpt-5.2
  embedding_model: text-embedding-3-large
  ocr_prompt: "custom prompt"
whatsapp:
  verify_token: test-token
  app_secret_file: /tmp/test-secret
  access_token_file: /tmp/test-access
  phone_number_id: "12345"
`
	f := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(f, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg, err := LoadServerConfig(f)
	if err != nil {
		t.Fatal(err)
	}

	if cfg.Server.Addr != ":9090" {
		t.Errorf("addr = %q, want %q", cfg.Server.Addr, ":9090")
	}
	if cfg.Server.DataDir != "/tmp/blackwood-test" {
		t.Errorf("data_dir = %q, want %q", cfg.Server.DataDir, "/tmp/blackwood-test")
	}
	if cfg.OpenAI.Model != "gpt-5.2" {
		t.Errorf("model = %q, want %q", cfg.OpenAI.Model, "gpt-5.2")
	}
	if cfg.OpenAI.ChatModel != "gpt-5.2" {
		t.Errorf("chat_model = %q, want %q", cfg.OpenAI.ChatModel, "gpt-5.2")
	}
	if cfg.OpenAI.EmbeddingModel != "text-embedding-3-large" {
		t.Errorf("embedding_model = %q, want %q", cfg.OpenAI.EmbeddingModel, "text-embedding-3-large")
	}
	if cfg.WhatsApp.VerifyToken != "test-token" {
		t.Errorf("verify_token = %q, want %q", cfg.WhatsApp.VerifyToken, "test-token")
	}
	if cfg.WhatsApp.PhoneNumberID != "12345" {
		t.Errorf("phone_number_id = %q, want %q", cfg.WhatsApp.PhoneNumberID, "12345")
	}
}

func TestLoadServerConfigEmpty(t *testing.T) {
	cfg, err := LoadServerConfig("")
	if err != nil {
		t.Fatal(err)
	}
	if cfg == nil {
		t.Fatal("expected non-nil config")
	}
}

func TestResolveDefaults(t *testing.T) {
	// Clear env vars that could interfere.
	for _, k := range []string{"OPENAI_API_KEY", "OPENAI_MODEL", "OPENAI_CHAT_MODEL", "OPENAI_OCR_PROMPT", "WHATSAPP_VERIFY_TOKEN", "WHATSAPP_APP_SECRET", "WHATSAPP_ACCESS_TOKEN", "WHATSAPP_PHONE_NUMBER_ID"} {
		t.Setenv(k, "")
	}

	cfg := &ServerConfig{}
	if err := cfg.Resolve(); err != nil {
		t.Fatal(err)
	}

	if cfg.Server.Addr != ":8080" {
		t.Errorf("default addr = %q, want %q", cfg.Server.Addr, ":8080")
	}
	if cfg.OpenAI.Model != "gpt-5.2" {
		t.Errorf("default model = %q, want %q", cfg.OpenAI.Model, "gpt-5.2")
	}
	if cfg.OpenAI.ChatModel != "gpt-5.2" {
		t.Errorf("default chat_model = %q, want %q", cfg.OpenAI.ChatModel, "gpt-5.2")
	}
	if cfg.OpenAI.EmbeddingModel != "text-embedding-3-small" {
		t.Errorf("default embedding_model = %q, want %q", cfg.OpenAI.EmbeddingModel, "text-embedding-3-small")
	}
	if cfg.OpenAI.OCRPrompt == "" {
		t.Error("expected non-empty default OCR prompt")
	}
}

func TestResolveSecretFiles(t *testing.T) {
	dir := t.TempDir()

	keyFile := filepath.Join(dir, "api-key")
	if err := os.WriteFile(keyFile, []byte("  sk-test-key  \n"), 0o600); err != nil {
		t.Fatal(err)
	}
	secretFile := filepath.Join(dir, "app-secret")
	if err := os.WriteFile(secretFile, []byte("wa-secret\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	tokenFile := filepath.Join(dir, "access-token")
	if err := os.WriteFile(tokenFile, []byte("wa-token"), 0o600); err != nil {
		t.Fatal(err)
	}

	cfg := &ServerConfig{
		OpenAI: OpenAISettings{
			APIKeyFile: keyFile,
		},
		WhatsApp: WhatsAppSettings{
			VerifyToken:     "vt",
			AppSecretFile:   secretFile,
			AccessTokenFile: tokenFile,
		},
	}

	if err := cfg.Resolve(); err != nil {
		t.Fatal(err)
	}

	if cfg.APIKey() != "sk-test-key" {
		t.Errorf("api key = %q, want %q", cfg.APIKey(), "sk-test-key")
	}
	if cfg.WhatsAppAppSecret() != "wa-secret" {
		t.Errorf("app secret = %q, want %q", cfg.WhatsAppAppSecret(), "wa-secret")
	}
	if cfg.WhatsAppAccessToken() != "wa-token" {
		t.Errorf("access token = %q, want %q", cfg.WhatsAppAccessToken(), "wa-token")
	}
}

func TestResolveEnvVarFallback(t *testing.T) {
	t.Setenv("OPENAI_API_KEY", "env-key")
	t.Setenv("OPENAI_MODEL", "env-model")
	t.Setenv("OPENAI_CHAT_MODEL", "env-chat-model")
	t.Setenv("WHATSAPP_VERIFY_TOKEN", "env-vt")
	t.Setenv("WHATSAPP_APP_SECRET", "env-secret")
	t.Setenv("WHATSAPP_ACCESS_TOKEN", "env-token")
	t.Setenv("WHATSAPP_PHONE_NUMBER_ID", "env-phone")

	cfg := &ServerConfig{}
	if err := cfg.Resolve(); err != nil {
		t.Fatal(err)
	}

	if cfg.APIKey() != "env-key" {
		t.Errorf("api key = %q, want %q", cfg.APIKey(), "env-key")
	}
	if cfg.OpenAI.Model != "env-model" {
		t.Errorf("model = %q, want %q", cfg.OpenAI.Model, "env-model")
	}
	if cfg.OpenAI.ChatModel != "env-chat-model" {
		t.Errorf("chat_model = %q, want %q", cfg.OpenAI.ChatModel, "env-chat-model")
	}
	if cfg.WhatsApp.VerifyToken != "env-vt" {
		t.Errorf("verify_token = %q, want %q", cfg.WhatsApp.VerifyToken, "env-vt")
	}
	if cfg.WhatsAppAppSecret() != "env-secret" {
		t.Errorf("app secret = %q, want %q", cfg.WhatsAppAppSecret(), "env-secret")
	}
	if cfg.WhatsAppAccessToken() != "env-token" {
		t.Errorf("access token = %q, want %q", cfg.WhatsAppAccessToken(), "env-token")
	}
	if cfg.WhatsApp.PhoneNumberID != "env-phone" {
		t.Errorf("phone_number_id = %q, want %q", cfg.WhatsApp.PhoneNumberID, "env-phone")
	}
	if !cfg.WhatsApp.Enabled {
		t.Error("expected WhatsApp to be enabled when verify token is set")
	}
}

func TestResolveTildeExpansion(t *testing.T) {
	for _, k := range []string{"OPENAI_API_KEY", "OPENAI_MODEL", "OPENAI_CHAT_MODEL", "OPENAI_OCR_PROMPT", "WHATSAPP_VERIFY_TOKEN", "WHATSAPP_APP_SECRET", "WHATSAPP_ACCESS_TOKEN", "WHATSAPP_PHONE_NUMBER_ID"} {
		t.Setenv(k, "")
	}

	cfg := &ServerConfig{
		Server: ServerSettings{
			DataDir: "~/test-blackwood",
		},
	}
	if err := cfg.Resolve(); err != nil {
		t.Fatal(err)
	}

	home, _ := os.UserHomeDir()
	want := filepath.Join(home, "test-blackwood")
	if cfg.Server.DataDir != want {
		t.Errorf("data_dir = %q, want %q", cfg.Server.DataDir, want)
	}
}

func TestResolveSecretFileMissing(t *testing.T) {
	cfg := &ServerConfig{
		OpenAI: OpenAISettings{
			APIKeyFile: "/nonexistent/path/key",
		},
	}
	if err := cfg.Resolve(); err == nil {
		t.Error("expected error for missing secret file")
	}
}

func TestConfigFileOverridesEnvVar(t *testing.T) {
	t.Setenv("OPENAI_MODEL", "env-model")

	// When config file sets a model, env var should NOT override it.
	cfg := &ServerConfig{
		OpenAI: OpenAISettings{
			Model: "config-model",
		},
	}
	if err := cfg.Resolve(); err != nil {
		t.Fatal(err)
	}

	if cfg.OpenAI.Model != "config-model" {
		t.Errorf("model = %q, want %q (config should override env)", cfg.OpenAI.Model, "config-model")
	}
}

func TestGranolaConfigFromFile(t *testing.T) {
	dir := t.TempDir()
	tokenFile := filepath.Join(dir, "granola-token")
	if err := os.WriteFile(tokenFile, []byte(`{"access_token":"test"}`), 0o600); err != nil {
		t.Fatal(err)
	}

	cfg := &ServerConfig{
		Granola: GranolaSettings{
			OAuthTokenFile: tokenFile,
			PollInterval:   "30m",
		},
	}
	if err := cfg.Resolve(); err != nil {
		t.Fatal(err)
	}

	if !cfg.Granola.Enabled {
		t.Error("expected Granola to be auto-enabled when token file exists")
	}
	if cfg.Granola.PollInterval != "30m" {
		t.Errorf("poll_interval = %q, want %q", cfg.Granola.PollInterval, "30m")
	}
}

func TestGranolaConfigFromEnvVar(t *testing.T) {
	t.Setenv("GRANOLA_OAUTH_TOKEN", "env-granola-token")

	cfg := &ServerConfig{}
	if err := cfg.Resolve(); err != nil {
		t.Fatal(err)
	}

	if cfg.GranolaOAuthToken() != "env-granola-token" {
		t.Errorf("granola oauth token = %q, want %q", cfg.GranolaOAuthToken(), "env-granola-token")
	}
	if !cfg.Granola.Enabled {
		t.Error("expected Granola to be auto-enabled from env var")
	}
	if cfg.Granola.PollInterval != "1h" {
		t.Errorf("default poll_interval = %q, want %q", cfg.Granola.PollInterval, "1h")
	}
}

func TestGranolaConfigDisabledByDefault(t *testing.T) {
	t.Setenv("GRANOLA_OAUTH_TOKEN", "")

	cfg := &ServerConfig{}
	if err := cfg.Resolve(); err != nil {
		t.Fatal(err)
	}

	if cfg.Granola.Enabled {
		t.Error("expected Granola to be disabled when no OAuth token is set")
	}
}

func TestGranolaConfigDisabledWhenFileMissing(t *testing.T) {
	cfg := &ServerConfig{
		Granola: GranolaSettings{
			OAuthTokenFile: "/nonexistent/path/token",
		},
	}
	if err := cfg.Resolve(); err != nil {
		t.Fatal(err)
	}

	if cfg.Granola.Enabled {
		t.Error("expected Granola to be disabled when token file doesn't exist")
	}
}

func TestGranolaConfigYAMLParsing(t *testing.T) {
	content := `
server:
  addr: ":8080"
granola:
  oauth_token_file: /tmp/granola-token
  poll_interval: 2h
`
	f := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(f, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg, err := LoadServerConfig(f)
	if err != nil {
		t.Fatal(err)
	}

	if cfg.Granola.OAuthTokenFile != "/tmp/granola-token" {
		t.Errorf("oauth_token_file = %q, want %q", cfg.Granola.OAuthTokenFile, "/tmp/granola-token")
	}
	if cfg.Granola.PollInterval != "2h" {
		t.Errorf("poll_interval = %q, want %q", cfg.Granola.PollInterval, "2h")
	}
}
