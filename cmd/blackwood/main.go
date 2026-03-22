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
	"github.com/csweichel/blackwood/internal/noteparser"
	"github.com/csweichel/blackwood/internal/obsidian"
	"github.com/csweichel/blackwood/internal/ocr"
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

	// Conditionally set up the OCR + Obsidian pipeline.
	var recognizer ocr.Recognizer
	var obsWriter *obsidian.Writer

	if cfg.Obsidian.VaultPath != "" {
		apiKey := os.Getenv(cfg.LLM.APIKeyEnv)
		recognizer = ocr.NewOpenAI(apiKey, cfg.LLM.Model, cfg.LLM.Prompt)
		slog.Info("OCR pipeline enabled", slog.String("model", cfg.LLM.Model))

		obsWriter = obsidian.New(cfg.Obsidian)
		slog.Info("Obsidian output enabled", slog.String("vault", cfg.Obsidian.VaultPath))
	}

	// Log startup summary of active processing modes.
	pipelineEnabled := obsWriter != nil
	hooksEnabled := len(cfg.Hooks) > 0
	slog.Info("processing modes",
		slog.Bool("pipeline", pipelineEnabled),
		slog.Bool("hooks", hooksEnabled),
	)

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

		processingFailed := false

		// Run the built-in pipeline if configured.
		if obsWriter != nil {
			if err := runPipeline(ctx, path, recognizer, obsWriter); err != nil {
				slog.Error("pipeline failed", slog.String("file", path), slog.String("error", err.Error()))
				processingFailed = true
			}
		}

		// Run hooks independently (after the pipeline, not instead of).
		if len(cfg.Hooks) > 0 {
			if err := runner.Run(ctx, path); err != nil {
				slog.Error("hooks failed", slog.String("file", path), slog.String("error", err.Error()))
				processingFailed = true
			}
		}

		if processingFailed {
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

// runPipeline parses a .note file, runs OCR on each page, and writes the result to Obsidian.
func runPipeline(ctx context.Context, path string, recognizer ocr.Recognizer, writer *obsidian.Writer) error {
	note, err := noteparser.Parse(path)
	if err != nil {
		return fmt.Errorf("parsing note: %w", err)
	}
	slog.Info("parsed note",
		slog.String("note_id", note.ID),
		slog.String("name", note.Name),
		slog.Int("pages", len(note.Pages)),
	)

	pages := make([]obsidian.PageResult, 0, len(note.Pages))
	for _, p := range note.Pages {
		text, err := recognizer.Recognize(ctx, p.Image)
		if err != nil {
			// Log OCR failure but continue with empty text.
			slog.Error("OCR failed for page",
				slog.String("note_id", note.ID),
				slog.String("page_id", p.ID),
				slog.String("error", err.Error()),
			)
			text = ""
		}
		slog.Info("OCR complete",
			slog.String("page_id", p.ID),
			slog.Int("text_len", len(text)),
		)
		pages = append(pages, obsidian.PageResult{
			PageID: p.ID,
			Image:  p.Image,
			Text:   text,
		})
	}

	entry := obsidian.NoteEntry{
		NoteID:     note.ID,
		NoteName:   note.Name,
		CreateTime: note.CreateTime,
		Pages:      pages,
	}

	if err := writer.Write(entry); err != nil {
		return fmt.Errorf("writing to obsidian: %w", err)
	}
	slog.Info("written to Obsidian", slog.String("note_id", note.ID))

	return nil
}
