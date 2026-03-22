package main

import (
	"context"
	"flag"
	"fmt"
	"log"
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

	fmt.Printf("watch_dir: %s\n", cfg.WatchDir)
	fmt.Printf("state_path: %s\n", cfg.StatePath)
	fmt.Printf("poll_interval: %s\n", cfg.PollInterval)
	fmt.Printf("hooks: %d configured\n", len(cfg.Hooks))
	for i, h := range cfg.Hooks {
		fmt.Printf("  [%d] %s %s\n", i, h.Command, strings.Join(h.Args, " "))
	}

	st, err := state.Load(cfg.StatePath)
	if err != nil {
		log.Fatalf("error loading state: %v", err)
	}

	ctx, stop := signal.NotifyContext(
		context.Background(),
		syscall.SIGINT, syscall.SIGTERM,
	)
	defer stop()

	w := watcher.New(cfg.WatchDir, cfg.PollInterval)
	ch, err := w.Start(ctx)
	if err != nil {
		log.Fatalf("error starting watcher: %v", err)
	}

	log.Printf("watching %s for .note files", cfg.WatchDir)

	runner := hooks.New(cfg.Hooks)

	for path := range ch {
		if st.IsProcessed(path) {
			log.Printf("skipping already-processed file: %s", path)
			continue
		}
		log.Printf("detected new file: %s", path)

		if err := runner.Run(ctx, path); err != nil {
			log.Printf("hooks failed for %s: %v", path, err)
			continue
		}

		hash, err := state.ComputeHash(path)
		if err != nil {
			log.Printf("error hashing %s: %v", path, err)
			continue
		}
		st.MarkProcessed(path, hash)
		if err := st.Save(); err != nil {
			log.Printf("error saving state: %v", err)
		}
		log.Printf("processed %s successfully", path)
	}

	log.Println("shutting down")
}
