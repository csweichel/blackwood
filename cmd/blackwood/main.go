package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/csweichel/blackwood/internal/config"
	"github.com/csweichel/blackwood/internal/hooks"
	"github.com/csweichel/blackwood/internal/state"
	"github.com/csweichel/blackwood/internal/watcher"
)

func main() {
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stderr, nil)))

	configPath := flag.String("config", "", "path to the configuration file")
	watchDir := flag.String("watch-dir", "", "directory to watch for new notes files (overrides config)")
	flag.Parse()

	if *configPath == "" {
		fmt.Fprintln(os.Stderr, "--config is required")
		flag.Usage()
		os.Exit(1)
	}

	cfg, err := config.Load(*configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error loading config: %v\n", err)
		os.Exit(1)
	}

	// CLI flag overrides config file value
	if *watchDir != "" {
		cfg.WatchDir = *watchDir
	}

	if err := cfg.Validate(); err != nil {
		fmt.Fprintf(os.Stderr, "invalid config: %v\n", err)
		os.Exit(1)
	}

	slog.Info("configuration loaded",
		slog.String("watch_dir", cfg.WatchDir),
		slog.String("state_path", cfg.StatePath),
		slog.String("poll_interval", cfg.PollInterval.String()),
		slog.Int("hooks", len(cfg.Hooks)),
	)
	for i, h := range cfg.Hooks {
		slog.Info("hook registered",
			slog.Int("index", i),
			slog.String("command", h.Command),
			slog.String("args", strings.Join(h.Args, " ")),
		)
	}

	st, err := state.Load(cfg.StatePath)
	if err != nil {
		slog.Error("error loading state", slog.String("error", err.Error()))
		os.Exit(1)
	}

	ctx, stop := signal.NotifyContext(
		context.Background(),
		syscall.SIGINT, syscall.SIGTERM,
	)
	defer stop()

	w := watcher.New(cfg.WatchDir, cfg.PollInterval)
	ch, err := w.Start(ctx)
	if err != nil {
		slog.Error("error starting watcher", slog.String("error", err.Error()))
		os.Exit(1)
	}

	slog.Info("watching for .note files", slog.String("dir", cfg.WatchDir))

	runner := hooks.New(cfg.Hooks)

	for path := range ch {
		if st.IsProcessed(path) {
			slog.Info("skipping already-processed file", slog.String("file", path))
			continue
		}
		slog.Info("detected new file", slog.String("file", path))

		if err := runner.Run(ctx, path); err != nil {
			slog.Error("hooks failed", slog.String("file", path), slog.String("error", err.Error()))
			continue
		}

		hash, err := state.ComputeHash(path)
		if err != nil {
			slog.Error("error hashing file", slog.String("file", path), slog.String("error", err.Error()))
			continue
		}
		st.MarkProcessed(path, hash)
		if err := st.Save(); err != nil {
			slog.Error("error saving state", slog.String("error", err.Error()))
		}
		slog.Info("processed file", slog.String("file", path))
	}

	slog.Info("shutting down")
}
