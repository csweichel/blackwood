package main

import (
	"context"
	"database/sql"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	osexec "os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/charmbracelet/huh"
	"github.com/csweichel/blackwood/gen/blackwood/v1/blackwoodv1connect"
	"github.com/csweichel/blackwood/internal/api"
	"github.com/csweichel/blackwood/internal/auth"
	"github.com/csweichel/blackwood/internal/config"
	"github.com/csweichel/blackwood/internal/describe"
	"github.com/csweichel/blackwood/internal/granola"
	"github.com/csweichel/blackwood/internal/importqueue"
	"github.com/csweichel/blackwood/internal/index"
	"github.com/csweichel/blackwood/internal/ocr"
	"github.com/csweichel/blackwood/internal/rag"
	"github.com/csweichel/blackwood/internal/state"
	"github.com/csweichel/blackwood/internal/storage"
	"github.com/csweichel/blackwood/internal/telegram"
	"github.com/csweichel/blackwood/internal/transcribe"
	"github.com/csweichel/blackwood/internal/watcher"
	"github.com/csweichel/blackwood/internal/whatsapp"
)

// Version is set by goreleaser via ldflags.
var Version = "dev"

func main() {
	// Handle subcommands before flag.Parse() since flag doesn't support them.
	if len(os.Args) > 1 && os.Args[1] == "setup" {
		runSetup()
		return
	}
	if len(os.Args) > 1 && os.Args[1] == "granola-login" {
		runGranolaLogin()
		return
	}

	configFile := flag.String("config", "", "path to config file")
	addrFlag := flag.String("addr", "", "listen address (overrides config)")
	dataDirFlag := flag.String("data-dir", "", "data directory (overrides config)")
	flag.Parse()

	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, nil)))

	cfg, err := config.LoadServerConfig(*configFile)
	if err != nil {
		slog.Error("load config", "error", err)
		os.Exit(1)
	}
	if err := cfg.Resolve(); err != nil {
		slog.Error("resolve config", "error", err)
		os.Exit(1)
	}

	// CLI flags override config file values.
	if *addrFlag != "" {
		cfg.Server.Addr = *addrFlag
	}
	if *dataDirFlag != "" {
		cfg.Server.DataDir = *dataDirFlag
	}

	if *configFile != "" {
		slog.Info("loaded config", "file", *configFile)
	} else {
		slog.Info("no config file, using env vars and defaults")
	}

	addr := cfg.Server.Addr
	dataDir := cfg.Server.DataDir

	slog.Info("starting blackwood", "version", Version, "addr", addr, "data-dir", dataDir)

	// Ensure data directory exists.
	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		slog.Error("create data dir", "error", err)
		os.Exit(1)
	}

	// Open the storage layer.
	dbPath := filepath.Join(dataDir, "blackwood.db")
	store, err := storage.New(dbPath, dataDir)
	if err != nil {
		slog.Error("open storage", "error", err)
		os.Exit(1)
	}
	defer func() { _ = store.Close() }()

	srv := api.NewServer(addr)

	// Register the health service.
	path, handler := blackwoodv1connect.NewHealthServiceHandler(&api.HealthHandler{})
	srv.Handle(path, handler)

	// Set up OCR recognizer and audio transcriber if OpenAI API key is configured.
	var recognizer ocr.Recognizer
	var audioTranscriber transcribe.Transcriber
	if apiKey := cfg.APIKey(); apiKey != "" {
		recognizer = ocr.NewOpenAI(apiKey, cfg.OpenAI.Model, cfg.OpenAI.OCRPrompt)
		slog.Info("OCR recognizer enabled", "model", cfg.OpenAI.Model)
		audioTranscriber = transcribe.NewWhisper(apiKey)
		slog.Info("audio transcriber enabled")
	}

	// Set up the semantic index and RAG engine if OpenAI API key is configured.
	var semanticIndex *index.Index
	var ragEngine *rag.Engine
	if apiKey := cfg.APIKey(); apiKey != "" {
		embClient := index.NewOpenAIEmbeddingClient(apiKey).WithModel(cfg.OpenAI.EmbeddingModel)

		var err error
		semanticIndex, err = index.New(store.DB(), store.WriteDB(), embClient)
		if err != nil {
			slog.Error("create index", "error", err)
			os.Exit(1)
		}
		if err := reindexExistingEntries(context.Background(), store.DB(), semanticIndex); err != nil {
			slog.Warn("initial reindex failed", "error", err)
		}

		ragEngine = rag.New(semanticIndex, store, apiKey, cfg.OpenAI.ChatModel)
		slog.Info("chat service enabled", "model", cfg.OpenAI.ChatModel)
	} else {
		slog.Warn("chat and indexing disabled: no OpenAI API key configured")
	}

	// Register the daily notes service.
	dnPath, dnHandler := blackwoodv1connect.NewDailyNotesServiceHandler(api.NewDailyNotesHandler(store, audioTranscriber, semanticIndex))
	srv.Handle(dnPath, dnHandler)

	// Create the background import worker (started after context is created below).
	worker := importqueue.New(store, recognizer, semanticIndex, dataDir)

	// Register the import service.
	importPath, importHandler := blackwoodv1connect.NewImportServiceHandler(api.NewImportHandler(store, recognizer, semanticIndex, worker, dataDir))
	srv.Handle(importPath, importHandler)
	chatPath, chatHandler := blackwoodv1connect.NewChatServiceHandler(api.NewChatHandler(ragEngine, store))
	srv.Handle(chatPath, chatHandler)

	// Register the preferences service.
	prefsPath, prefsHandler := blackwoodv1connect.NewPreferencesServiceHandler(api.NewPreferencesHandler(store))
	srv.Handle(prefsPath, prefsHandler)

	// WhatsApp webhook.
	if cfg.WhatsApp.Enabled {
		waCfg := whatsapp.WebhookConfig{
			VerifyToken:   cfg.WhatsApp.VerifyToken,
			AppSecret:     cfg.WhatsAppAppSecret(),
			AccessToken:   cfg.WhatsAppAccessToken(),
			PhoneNumberID: cfg.WhatsApp.PhoneNumberID,
		}

		var t transcribe.Transcriber
		var d describe.Describer
		if apiKey := cfg.APIKey(); apiKey != "" {
			t = transcribe.NewWhisper(apiKey)
			d = describe.NewVision(apiKey, cfg.OpenAI.Model)
		}

		waHandler := whatsapp.NewWebhookHandler(waCfg, store, t, d, semanticIndex)
		srv.Handle("/api/webhooks/whatsapp", waHandler)
		slog.Info("WhatsApp webhook enabled")
	}

	// Serve attachment files.
	srv.Handle("GET /api/attachments/{id}", api.ServeAttachment(store))
	srv.Handle("GET /api/daily-notes/{date}/attachments/{filename}", api.ServeAttachmentByFilename(store))

	// PDF export for daily notes.
	srv.Handle("GET /api/daily-notes/{date}/pdf", api.ServePDF(store))

	// Summarize endpoint (requires RAG engine).
	if ragEngine != nil {
		srv.Handle("POST /api/daily-notes/{date}/summarize", api.ServeSummarize(store, ragEngine))
	}

	// Range summaries for weekly/monthly views.
	srv.Handle("GET /api/daily-notes/range", api.ServeRangeSummaries(store))

	// Search endpoint (requires semantic index).
	if semanticIndex != nil {
		srv.Handle("GET /api/search", api.ServeSearch(store, semanticIndex))
	}

	// Location tagging endpoint.
	srv.Handle("POST /api/entries/location", api.ServeSetLocation(store))

	// Web clipping endpoint.
	srv.Handle("POST /api/clip", api.NewClipHandler(store))

	// --- Authentication ---
	// Must be registered before the SPA catch-all handler so Connect
	// routes like /blackwood.v1.AuthService/* are matched first.
	if cfg.Auth.TOTP.Enabled {
		authMiddleware, err := auth.Setup(auth.SetupConfig{
			Store:  store,
			UseTLS: cfg.TLSEnabled(),
		}, srv.Mux())
		if err != nil {
			slog.Error("auth setup", "error", err)
			os.Exit(1)
		}
		srv.SetAuthMiddleware(authMiddleware)
		slog.Info("TOTP authentication enabled")
	} else {
		if err := auth.Cleanup(store); err != nil {
			slog.Error("auth cleanup", "error", err)
		}
	}

	// Serve web UI: filesystem first (development), then embedded (release binary).
	// Uses an SPA fallback handler so client-side routes (e.g. /day/2025-01-15)
	// serve index.html instead of 404.
	// Registered after all API/Connect routes so the catch-all doesn't shadow them.
	if info, err := os.Stat("web/dist"); err == nil && info.IsDir() {
		srv.Handle("/", spaHandler(http.Dir("web/dist")))
		slog.Info("serving web UI from filesystem", "path", "web/dist")
	} else if sfs, err := staticFS(); err == nil {
		srv.Handle("/", spaHandler(http.FS(sfs)))
		slog.Info("serving embedded web UI")
	}

	// Graceful shutdown on SIGINT/SIGTERM.
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	// Start the background import worker.
	go worker.Start(ctx)

	// Start the nightly digest if RAG is available.
	if ragEngine != nil {
		go api.StartNightlyDigest(ctx, store, ragEngine)
	}

	// Telegram bot.
	if cfg.Telegram.Enabled {
		tgCfg := telegram.BotConfig{
			Token:          cfg.TelegramBotToken(),
			AllowedChatIDs: cfg.Telegram.AllowedChatIDs,
		}

		var t transcribe.Transcriber
		var d describe.Describer
		if apiKey := cfg.APIKey(); apiKey != "" {
			t = transcribe.NewWhisper(apiKey)
			d = describe.NewVision(apiKey, cfg.OpenAI.Model)
		}

		tgBot := telegram.NewBot(tgCfg, store, t, d, semanticIndex)
		go tgBot.Start(ctx)
		slog.Info("Telegram bot enabled")
	}

	// Granola meeting notes sync.
	if cfg.Granola.Enabled {
		pollInterval, err := time.ParseDuration(cfg.Granola.PollInterval)
		if err != nil {
			pollInterval = 1 * time.Hour
		}

		var ts *granola.TokenSource
		if cfg.Granola.OAuthTokenFile != "" {
			pt, err := granola.LoadToken(cfg.Granola.OAuthTokenFile)
			if err != nil {
				slog.Error("failed to load Granola token file", "path", cfg.Granola.OAuthTokenFile, "error", err)
			} else {
				ts = granola.NewTokenSource(pt, cfg.Granola.OAuthTokenFile)
			}
		}
		if ts == nil {
			// Fallback: plain token from env var (no refresh support).
			pt := &granola.PersistedToken{AccessToken: cfg.GranolaOAuthToken()}
			ts = granola.NewTokenSource(pt, "")
		}

		gs := granola.New(ts, store, semanticIndex, pollInterval)
		go gs.Start(ctx)
		slog.Info("Granola sync enabled", "interval", pollInterval)
	}

	// Start the file watcher if configured.
	if len(cfg.Watcher.WatchDirs) > 0 {
		pollInterval, err := time.ParseDuration(cfg.Watcher.PollInterval)
		if err != nil {
			pollInterval = 30 * time.Second
		}

		w := watcher.New(cfg.Watcher.WatchDirs, pollInterval)
		watchCh, err := w.Start(ctx)
		if err != nil {
			slog.Error("start watcher", "error", err)
			os.Exit(1)
		}

		go func() {
			slog.Info("starting file watcher", "dirs", cfg.Watcher.WatchDirs, "interval", pollInterval)
			for filePath := range watchCh {
				// Compute hash for deduplication.
				hash, err := state.ComputeHash(filePath)
				if err != nil {
					slog.Warn("hash file", "file", filePath, "error", err)
					continue
				}

				// Check if already processed with same hash.
				existing, _ := store.GetWatchedFile(context.Background(), filePath)
				if existing != nil && existing.Hash == hash {
					continue
				}

				// Determine file type.
				var fileType string
				lower := strings.ToLower(filePath)
				if strings.HasSuffix(lower, ".note") {
					fileType = "viwoods"
				} else if strings.HasSuffix(lower, ".md") {
					fileType = "obsidian"
				} else {
					continue
				}
				// Create import job.
				jobID := storage.NewUUID()
				job := &storage.ImportJob{
					ID:       jobID,
					Status:   "pending",
					Filename: filepath.Base(filePath),
					FileType: fileType,
					FilePath: filePath,
					Source:   "watcher",
				}
				if err := store.CreateImportJob(context.Background(), job); err != nil {
					slog.Error("create watcher import job", "file", filePath, "error", err)
					continue
				}

				// Record in watched_files.
				wf := &storage.WatchedFile{
					Path:  filePath,
					Hash:  hash,
					JobID: jobID,
				}
				if err := store.UpsertWatchedFile(context.Background(), wf); err != nil {
					slog.Warn("upsert watched file", "file", filePath, "error", err)
				}

				// Signal the worker.
				worker.Enqueue()
				slog.Info("enqueued watched file", "file", filePath, "type", fileType)
			}
		}()
	}

	httpServer := &http.Server{
		Addr:              addr,
		Handler:           srv.Handler(),
		ReadHeaderTimeout: 10 * time.Second,
	}

	errCh := make(chan error, 1)
	if cfg.TLSEnabled() {
		certFile := cfg.Server.TLS.CertFile
		keyFile := cfg.Server.TLS.KeyFile

		// Fail fast if cert/key files don't exist.
		if _, err := os.Stat(certFile); err != nil {
			slog.Error("TLS cert file not found", "path", certFile, "error", err)
			os.Exit(1)
		}
		if _, err := os.Stat(keyFile); err != nil {
			slog.Error("TLS key file not found", "path", keyFile, "error", err)
			os.Exit(1)
		}

		slog.Info("starting server with TLS", "addr", addr, "cert", certFile)
		go func() {
			errCh <- httpServer.ListenAndServeTLS(certFile, keyFile)
		}()
	} else {
		go func() {
			errCh <- httpServer.ListenAndServe()
		}()
	}

	select {
	case err := <-errCh:
		if err != nil && err != http.ErrServerClosed {
			slog.Error("server error", "error", err)
			os.Exit(1)
		}
	case <-ctx.Done():
		slog.Info("shutting down")
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := httpServer.Shutdown(shutdownCtx); err != nil {
			slog.Error("shutdown error", "error", err)
			os.Exit(1)
		}
	}
}

// spaHandler serves static files from fs, falling back to index.html for
// paths that don't match a real file (client-side routing support).
func spaHandler(fsys http.FileSystem) http.Handler {
	fileServer := http.FileServer(fsys)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Try to open the requested path as a file.
		path := r.URL.Path
		f, err := fsys.Open(path)
		if err == nil {
			_ = f.Close()
			fileServer.ServeHTTP(w, r)
			return
		}
		// Fall back to index.html for SPA routes.
		r.URL.Path = "/"
		fileServer.ServeHTTP(w, r)
	})
}

func runGranolaLogin() {
	home, err := os.UserHomeDir()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: cannot determine home directory: %v\n", err)
		os.Exit(1)
	}

	secretsDir := filepath.Join(home, ".blackwood", "secrets")
	if err := os.MkdirAll(secretsDir, 0o700); err != nil {
		fmt.Fprintf(os.Stderr, "Error: cannot create secrets directory: %v\n", err)
		os.Exit(1)
	}
	tokenPath := filepath.Join(secretsDir, "granola-oauth-token")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	login, err := granola.OAuthLogin(ctx)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("Open this URL in your browser to authorize Blackwood:")
	fmt.Printf("\n  %s\n\n", login.AuthURL)

	// Try to open the browser.
	for _, cmd := range []string{"open", "xdg-open", "sensible-browser"} {
		if p := osexec.Command(cmd, login.AuthURL); p.Start() == nil {
			break
		}
	}

	fmt.Println("After authorizing, your browser will redirect to a URL starting with")
	fmt.Printf("  %s?code=...\n\n", login.RedirectURI)
	fmt.Println("Paste the full redirect URL (or just the code) here:")
	fmt.Print("> ")

	var input string
	if _, err := fmt.Scanln(&input); err != nil {
		fmt.Fprintf(os.Stderr, "Error reading input: %v\n", err)
		os.Exit(1)
	}

	code, err := granola.ExtractCodeFromURL(input)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	token, err := login.Exchange(ctx, code)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error exchanging code: %v\n", err)
		os.Exit(1)
	}

	pt := &granola.PersistedToken{
		AccessToken:  token.AccessToken,
		RefreshToken: token.RefreshToken,
		ClientID:     login.ClientID,
		ClientSecret: login.ClientSecret,
	}
	if token.ExpiresIn > 0 {
		pt.ExpiresAt = time.Now().Add(time.Duration(token.ExpiresIn) * time.Second)
	}

	if err := granola.SaveToken(tokenPath, pt); err != nil {
		fmt.Fprintf(os.Stderr, "Error saving token: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("\n✓ Granola OAuth token saved to %s\n", tokenPath)
	if token.RefreshToken != "" {
		fmt.Println("  (includes refresh token — will auto-renew)")
	}
	fmt.Println("\nAdd this to your blackwood config:")
	fmt.Println("  granola:")
	fmt.Printf("    oauth_token_file: %s\n", tokenPath)
}

func runSetup() {
	home, err := os.UserHomeDir()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: cannot determine home directory: %v\n", err)
		os.Exit(1)
	}

	defaultDir := filepath.Join(home, ".blackwood")
	configPath := filepath.Join(defaultDir, "config.yaml")

	// Check for existing config.
	if _, err := os.Stat(configPath); err == nil {
		var overwrite bool
		err := huh.NewConfirm().
			Title("Config file already exists at " + configPath).
			Description("Do you want to overwrite it?").
			Affirmative("Yes").
			Negative("No").
			Value(&overwrite).
			Run()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
		if !overwrite {
			fmt.Println("Setup cancelled.")
			return
		}
	}

	var (
		dataDir       = defaultDir
		listenAddr    = ":8080"
		openaiKey     string
		setupTG       bool
		telegramToken string
		setupTLS      bool
		tlsCertFile   string
		tlsKeyFile    string
	)

	// Collect server settings and OpenAI key.
	err = huh.NewForm(
		huh.NewGroup(
			huh.NewNote().
				Title("Blackwood Setup").
				Description("Configure your Blackwood instance."),

			huh.NewInput().
				Title("Data directory").
				Value(&dataDir).
				Placeholder(defaultDir),

			huh.NewInput().
				Title("Listen address").
				Value(&listenAddr).
				Placeholder(":8080"),

			huh.NewInput().
				Title("OpenAI API key").
				Description("Required for transcription, vision, and chat.").
				Value(&openaiKey).
				EchoMode(huh.EchoModePassword).
				Validate(func(s string) error {
					if strings.TrimSpace(s) == "" {
						return fmt.Errorf("API key is required")
					}
					return nil
				}),

			huh.NewConfirm().
				Title("Set up Telegram bot?").
				Affirmative("Yes").
				Negative("No").
				Value(&setupTG),
		),
	).Run()
	if err != nil {
		if err == huh.ErrUserAborted {
			fmt.Println("Setup cancelled.")
			return
		}
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	// Collect Telegram token if requested.
	if setupTG {
		err = huh.NewInput().
			Title("Telegram bot token").
			Description("From @BotFather").
			Value(&telegramToken).
			EchoMode(huh.EchoModePassword).
			Validate(func(s string) error {
				if strings.TrimSpace(s) == "" {
					return fmt.Errorf("bot token is required")
				}
				return nil
			}).
			Run()
		if err != nil {
			if err == huh.ErrUserAborted {
				fmt.Println("Setup cancelled.")
				return
			}
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
	}

	// Ask about TLS.
	err = huh.NewConfirm().
		Title("Configure TLS?").
		Description("Serve over HTTPS with your own certificate.").
		Affirmative("Yes").
		Negative("No").
		Value(&setupTLS).
		Run()
	if err != nil {
		if err == huh.ErrUserAborted {
			fmt.Println("Setup cancelled.")
			return
		}
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	if setupTLS {
		err = huh.NewForm(
			huh.NewGroup(
				huh.NewInput().
					Title("TLS certificate file path").
					Value(&tlsCertFile).
					Validate(func(s string) error {
						if strings.TrimSpace(s) == "" {
							return fmt.Errorf("cert file path is required")
						}
						return nil
					}),
				huh.NewInput().
					Title("TLS private key file path").
					Value(&tlsKeyFile).
					Validate(func(s string) error {
						if strings.TrimSpace(s) == "" {
							return fmt.Errorf("key file path is required")
						}
						return nil
					}),
			),
		).Run()
		if err != nil {
			if err == huh.ErrUserAborted {
				fmt.Println("Setup cancelled.")
				return
			}
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
		tlsCertFile = strings.TrimSpace(tlsCertFile)
		tlsKeyFile = strings.TrimSpace(tlsKeyFile)
	}

	openaiKey = strings.TrimSpace(openaiKey)
	telegramToken = strings.TrimSpace(telegramToken)

	// Resolve ~ in dataDir.
	if strings.HasPrefix(dataDir, "~/") {
		dataDir = filepath.Join(home, dataDir[2:])
	}

	// Create directories.
	secretsDir := filepath.Join(dataDir, "secrets")
	if err := os.MkdirAll(secretsDir, 0o755); err != nil {
		fmt.Fprintf(os.Stderr, "Error creating directories: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("✓ Created %s\n", secretsDir)

	// Write OpenAI API key.
	openaiKeyPath := filepath.Join(secretsDir, "openai-api-key")
	if err := os.WriteFile(openaiKeyPath, []byte(openaiKey), 0o600); err != nil {
		fmt.Fprintf(os.Stderr, "Error writing API key: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("✓ Saved OpenAI API key to %s\n", openaiKeyPath)

	// Write Telegram bot token if configured.
	telegramTokenPath := filepath.Join(secretsDir, "telegram-bot-token")
	if setupTG {
		if err := os.WriteFile(telegramTokenPath, []byte(telegramToken), 0o600); err != nil {
			fmt.Fprintf(os.Stderr, "Error writing Telegram token: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("✓ Saved Telegram bot token to %s\n", telegramTokenPath)
	}

	// Build config YAML. Use ~ paths for portability.
	displayDir := dataDir
	if strings.HasPrefix(dataDir, home) {
		displayDir = "~" + dataDir[len(home):]
	}

	var cfgBuf strings.Builder
	cfgBuf.WriteString("server:\n")
	fmt.Fprintf(&cfgBuf, "  addr: %q\n", listenAddr)
	fmt.Fprintf(&cfgBuf, "  data_dir: %s\n", displayDir)
	if setupTLS {
		cfgBuf.WriteString("  tls:\n")
		fmt.Fprintf(&cfgBuf, "    cert_file: %s\n", tlsCertFile)
		fmt.Fprintf(&cfgBuf, "    key_file: %s\n", tlsKeyFile)
	} else {
		cfgBuf.WriteString("  # tls:\n")
		cfgBuf.WriteString("  #   cert_file: /path/to/cert.pem\n")
		cfgBuf.WriteString("  #   key_file: /path/to/key.pem\n")
	}
	cfgBuf.WriteString("\n")
	cfgBuf.WriteString("openai:\n")
	fmt.Fprintf(&cfgBuf, "  api_key_file: %s/secrets/openai-api-key\n", displayDir)
	cfgBuf.WriteString("\n")

	if setupTG {
		cfgBuf.WriteString("telegram:\n")
		fmt.Fprintf(&cfgBuf, "  bot_token_file: %s/secrets/telegram-bot-token\n", displayDir)
	} else {
		cfgBuf.WriteString("# telegram:\n")
		fmt.Fprintf(&cfgBuf, "#   bot_token_file: %s/secrets/telegram-bot-token\n", displayDir)
	}

	// Resolve configPath relative to dataDir (in case user changed it).
	configPath = filepath.Join(dataDir, "config.yaml")
	if err := os.WriteFile(configPath, []byte(cfgBuf.String()), 0o644); err != nil {
		fmt.Fprintf(os.Stderr, "Error writing config: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("✓ Config written to %s\n", configPath)

	fmt.Println()
	fmt.Println("To start Blackwood:")
	fmt.Printf("  blackwood --config %s\n", configPath)
}
func reindexExistingEntries(ctx context.Context, db *sql.DB, idx *index.Index) error {
	rows, err := db.QueryContext(ctx, `SELECT id, content FROM entries`)
	if err != nil {
		return fmt.Errorf("query entries for reindex: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var entries []index.EntryForIndex
	for rows.Next() {
		var e index.EntryForIndex
		if err := rows.Scan(&e.ID, &e.Text); err != nil {
			return fmt.Errorf("scan entry for reindex: %w", err)
		}
		entries = append(entries, e)
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterate entries for reindex: %w", err)
	}

	if len(entries) == 0 {
		return nil
	}
	if err := idx.Reindex(ctx, entries); err != nil {
		return fmt.Errorf("reindex entries: %w", err)
	}
	slog.Info("reindexed entries", "count", len(entries))
	return nil
}
