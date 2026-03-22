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
	"github.com/csweichel/blackwood/internal/ocr"
	"github.com/csweichel/blackwood/internal/storage"
)

func main() {
	homeDir, _ := os.UserHomeDir()
	defaultDataDir := filepath.Join(homeDir, ".blackwood")

	addr := flag.String("addr", ":8080", "listen address")
	dataDir := flag.String("data-dir", defaultDataDir, "data directory")
	flag.Parse()

	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, nil)))
	slog.Info("starting blackwood-server", "addr", *addr, "data-dir", *dataDir)

	// Ensure data directory exists.
	if err := os.MkdirAll(*dataDir, 0o755); err != nil {
		slog.Error("create data dir", "error", err)
		os.Exit(1)
	}

	// Open the storage layer.
	dbPath := filepath.Join(*dataDir, "blackwood.db")
	store, err := storage.New(dbPath, *dataDir)
	if err != nil {
		slog.Error("open storage", "error", err)
		os.Exit(1)
	}
	defer store.Close()

	srv := api.NewServer(*addr)

	// Register the health service.
	path, handler := blackwoodv1connect.NewHealthServiceHandler(&api.HealthHandler{})
	srv.Handle(path, handler)

	// Register the daily notes service.
	dnPath, dnHandler := blackwoodv1connect.NewDailyNotesServiceHandler(api.NewDailyNotesHandler(store))
	srv.Handle(dnPath, dnHandler)

	// Set up OCR recognizer if OPENAI_API_KEY is configured.
	var recognizer ocr.Recognizer
	if apiKey := os.Getenv("OPENAI_API_KEY"); apiKey != "" {
		model := os.Getenv("OPENAI_MODEL")
		if model == "" {
			model = "gpt-4o"
		}
		prompt := os.Getenv("OPENAI_OCR_PROMPT")
		if prompt == "" {
			prompt = "Transcribe the handwritten text in this image. Return only the transcribed text, no commentary."
		}
		recognizer = ocr.NewOpenAI(apiKey, model, prompt)
		slog.Info("OCR recognizer enabled", "model", model)
	}

	// Register the import service.
	importPath, importHandler := blackwoodv1connect.NewImportServiceHandler(api.NewImportHandler(store, recognizer))
	srv.Handle(importPath, importHandler)

	// Serve attachment files.
	srv.Handle("GET /api/attachments/{id}", api.ServeAttachment(store))

	// Serve static files from web/dist/ if the directory exists (for future web UI).
	if info, err := os.Stat("web/dist"); err == nil && info.IsDir() {
		srv.Handle("/", http.FileServer(http.Dir("web/dist")))
	}

	httpServer := &http.Server{
		Addr:              *addr,
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
