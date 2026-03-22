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
)

func main() {
	homeDir, _ := os.UserHomeDir()
	defaultDataDir := filepath.Join(homeDir, ".blackwood")

	addr := flag.String("addr", ":8080", "listen address")
	dataDir := flag.String("data-dir", defaultDataDir, "data directory")
	flag.Parse()

	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, nil)))
	slog.Info("starting blackwood-server", "addr", *addr, "data-dir", *dataDir)

	srv := api.NewServer(*addr)

	// Register the health service.
	path, handler := blackwoodv1connect.NewHealthServiceHandler(&api.HealthHandler{})
	srv.Handle(path, handler)

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
