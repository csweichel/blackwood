package api

import (
	"context"
	"log/slog"
	"strings"
	"time"

	"github.com/csweichel/blackwood/internal/rag"
	"github.com/csweichel/blackwood/internal/storage"
)

// StartNightlyDigest runs a background goroutine that generates a summary for
// the day that just ended at midnight in the user's configured timezone.
// It skips notes that are empty or already have a "# Summary" section.
func StartNightlyDigest(ctx context.Context, store *storage.Store, engine *rag.Engine) {
	for {
		loc := UserTimezone(ctx, store)
		now := time.Now().In(loc)
		// Next midnight in the user's timezone.
		tomorrow := time.Date(now.Year(), now.Month(), now.Day()+1, 0, 0, 0, 0, loc)
		delay := time.Until(tomorrow)

		slog.Info("nightly digest scheduled", "next_run", tomorrow.Format(time.RFC3339), "timezone", loc.String(), "delay", delay)

		select {
		case <-ctx.Done():
			slog.Info("nightly digest stopped")
			return
		case <-time.After(delay):
		}

		// Summarize the day that just ended (in the user's timezone).
		loc = UserTimezone(ctx, store) // re-read in case it changed
		yesterday := time.Now().In(loc).Add(-1 * time.Second).Format("2006-01-02")
		generateDigest(ctx, store, engine, yesterday)
	}
}

func generateDigest(ctx context.Context, store *storage.Store, engine *rag.Engine, date string) {
	note, err := store.GetDailyNoteByDate(ctx, date)
	if err != nil {
		slog.Debug("nightly digest: no note for date", "date", date)
		return
	}

	content := strings.TrimSpace(note.Content)
	if content == "" {
		slog.Debug("nightly digest: empty note", "date", date)
		return
	}

	// Skip if already summarized.
	if strings.Contains(content, "# Summary") {
		slog.Debug("nightly digest: already has summary", "date", date)
		return
	}

	summary, err := engine.Summarize(ctx, content)
	if err != nil {
		slog.Error("nightly digest: summarize failed", "date", date, "error", err)
		return
	}

	if err := store.SetSection(ctx, note.ID, "# Summary", summary+"\n"); err != nil {
		slog.Error("nightly digest: set section failed", "date", date, "error", err)
		return
	}

	slog.Info("nightly digest: generated summary", "date", date)
}
