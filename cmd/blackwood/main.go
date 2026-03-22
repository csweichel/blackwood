package main

import (
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/csweichel/blackwood/internal/config"
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
}
