package main

import (
	"context"
	"flag"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/csweichel/blackwood/gen/blackwood/v1/blackwoodv1connect"
	"github.com/csweichel/blackwood/internal/api"
	"github.com/csweichel/blackwood/internal/config"
	"github.com/csweichel/blackwood/internal/describe"
	"github.com/csweichel/blackwood/internal/index"
	"github.com/csweichel/blackwood/internal/ocr"
	"github.com/csweichel/blackwood/internal/rag"
	"github.com/csweichel/blackwood/internal/storage"
	"github.com/csweichel/blackwood/internal/transcribe"
	"github.com/csweichel/blackwood/internal/whatsapp"
)

// Version is set by goreleaser via ldflags.
var Version = "dev"

func main() {
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

	slog.Info("starting blackwood-server", "addr", addr, "data-dir", dataDir)

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

	// Register the daily notes service.
	dnPath, dnHandler := blackwoodv1connect.NewDailyNotesServiceHandler(api.NewDailyNotesHandler(store))
	srv.Handle(dnPath, dnHandler)

	// Set up OCR recognizer if OpenAI API key is configured.
	var recognizer ocr.Recognizer
	if apiKey := cfg.APIKey(); apiKey != "" {
		recognizer = ocr.NewOpenAI(apiKey, cfg.OpenAI.Model, cfg.OpenAI.OCRPrompt)
		slog.Info("OCR recognizer enabled", "model", cfg.OpenAI.Model)
	}

	// Register the import service.
	importPath, importHandler := blackwoodv1connect.NewImportServiceHandler(api.NewImportHandler(store, recognizer))
	srv.Handle(importPath, importHandler)

	// Set up the RAG chat service if OpenAI API key is configured.
	if apiKey := cfg.APIKey(); apiKey != "" {
		embClient := index.NewOpenAIEmbeddingClient(apiKey)

		idx, err := index.New(store.DB(), embClient)
		if err != nil {
			slog.Error("create index", "error", err)
			os.Exit(1)
		}

		ragEngine := rag.New(idx, store, apiKey, cfg.OpenAI.ChatModel)

		chatPath, chatHandler := blackwoodv1connect.NewChatServiceHandler(api.NewChatHandler(ragEngine, store))
		srv.Handle(chatPath, chatHandler)
		slog.Info("chat service enabled", "model", cfg.OpenAI.ChatModel)
	}

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

		waHandler := whatsapp.NewWebhookHandler(waCfg, store, t, d)
		srv.Handle("/api/webhooks/whatsapp", waHandler)
		slog.Info("WhatsApp webhook enabled")
	}

	// Serve attachment files.
	srv.Handle("GET /api/attachments/{id}", api.ServeAttachment(store))

	// Serve web UI: filesystem first (development), then embedded (release binary).
	if info, err := os.Stat("web/dist"); err == nil && info.IsDir() {
		srv.Handle("/", http.FileServer(http.Dir("web/dist")))
		slog.Info("serving web UI from filesystem", "path", "web/dist")
	} else if sfs, err := staticFS(); err == nil {
		srv.Handle("/", http.FileServer(http.FS(sfs)))
		slog.Info("serving embedded web UI")
	}

	httpServer := &http.Server{
		Addr:              addr,
		Handler:           srv.Handler(),
		ReadHeaderTimeout: 10 * time.Second,
	}

	// Graceful shutdown on SIGINT/SIGTERM.
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	errCh := make(chan error, 1)
	go func() {
		errCh <- httpServer.ListenAndServe()
	}()

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
