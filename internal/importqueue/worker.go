package importqueue

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/csweichel/blackwood/internal/index"
	"github.com/csweichel/blackwood/internal/noteparser"
	"github.com/csweichel/blackwood/internal/ocr"
	"github.com/csweichel/blackwood/internal/storage"
)

// Worker processes import jobs from the queue sequentially.
type Worker struct {
	store      *storage.Store
	recognizer ocr.Recognizer
	indexer    index.Indexer
	dataDir    string
	notify     chan struct{}
}

// New creates a new import queue worker.
func New(store *storage.Store, recognizer ocr.Recognizer, indexer index.Indexer, dataDir string) *Worker {
	return &Worker{
		store:      store,
		recognizer: recognizer,
		indexer:    indexer,
		dataDir:    dataDir,
		notify:     make(chan struct{}, 1),
	}
}

// Enqueue signals the worker that a new job is available.
func (w *Worker) Enqueue() {
	select {
	case w.notify <- struct{}{}:
	default:
		// Already signalled, worker will pick it up.
	}
}

// Start begins the background processing loop.
// It resets any stale processing jobs to pending, then loops:
// claim → process → update status → wait for signal or poll.
func (w *Worker) Start(ctx context.Context) {
	if err := w.store.ResetProcessingJobs(ctx); err != nil {
		slog.Error("reset processing jobs on startup", "error", err)
	}

	for {
		job, err := w.store.ClaimNextPendingJob(ctx)
		if err != nil {
			slog.Error("claim next pending job", "error", err)
			select {
			case <-ctx.Done():
				return
			case <-time.After(5 * time.Second):
				continue
			}
		}

		if job != nil {
			w.processJob(ctx, job)
			// Immediately try the next job without waiting.
			continue
		}

		// No jobs available — wait for a signal or poll.
		select {
		case <-ctx.Done():
			return
		case <-w.notify:
		case <-time.After(5 * time.Second):
		}
	}
}

func (w *Worker) processJob(ctx context.Context, job *storage.ImportJob) {
	slog.Info("processing import job", "id", job.ID, "type", job.FileType, "file", job.Filename)

	var err error
	switch job.FileType {
	case "viwoods":
		err = w.processViwoods(ctx, job)
	case "obsidian":
		err = w.processObsidian(ctx, job)
	default:
		err = fmt.Errorf("unknown file type: %s", job.FileType)
	}

	if err != nil {
		slog.Error("import job failed", "id", job.ID, "error", err)
		_ = w.store.UpdateImportJobResult(ctx, job.ID, fmt.Sprintf(`{"error":%q}`, err.Error()))
		_ = w.store.UpdateImportJobStatus(ctx, job.ID, "error")
		return
	}

	_ = w.store.UpdateImportJobStatus(ctx, job.ID, "done")
	slog.Info("import job completed", "id", job.ID)

	// Clean up staging directory for upload-sourced jobs.
	if job.Source == "upload" {
		stagingDir := fmt.Sprintf("%s/import-staging/%s", w.dataDir, job.ID)
		if err := os.RemoveAll(stagingDir); err != nil {
			slog.Warn("failed to clean staging dir", "dir", stagingDir, "error", err)
		}
	}
}

func (w *Worker) processViwoods(ctx context.Context, job *storage.ImportJob) error {
	note, err := noteparser.Parse(job.FilePath)
	if err != nil {
		return fmt.Errorf("parse note file: %w", err)
	}

	date := note.CreateTime.UTC().Format("2006-01-02")
	dailyNote, err := w.store.GetOrCreateDailyNote(ctx, date)
	if err != nil {
		return fmt.Errorf("get or create daily note: %w", err)
	}

	totalPages := len(note.Pages)
	_ = w.store.UpdateImportJobProgress(ctx, job.ID, 0, totalPages)

	// Create a placeholder entry first so we can attach images to it.
	entry := &storage.Entry{
		DailyNoteID: dailyNote.ID,
		Type:        "viwoods",
		Content:     "",
		RawContent:  "",
		Source:      "import",
	}
	if err := w.store.CreateEntry(ctx, entry); err != nil {
		return fmt.Errorf("create entry: %w", err)
	}

	// OCR each page, store the image as an attachment, and build markdown
	// that includes both the page image and the OCR text.
	var md strings.Builder
	md.WriteString("## " + note.Name + "\n")

	for i, page := range note.Pages {
		fmt.Fprintf(&md, "\n### Page %d\n\n", i+1)

		// Store the page image as an attachment.
		att := &storage.Attachment{
			EntryID:     entry.ID,
			Filename:    fmt.Sprintf("page_%d.png", i+1),
			ContentType: "image/png",
		}
		if err := w.store.CreateAttachment(ctx, att, page.Image, date); err != nil {
			slog.Warn("failed to store page attachment", "page", i+1, "error", err)
		} else {
			fmt.Fprintf(&md, "![Page %d](%s)\n\n", i+1, filepath.Base(att.StoragePath))
		}

		// OCR the page text.
		if w.recognizer != nil {
			text, err := w.recognizer.Recognize(ctx, page.Image)
			if err != nil {
				slog.Warn("OCR failed for page", "page", i+1, "jobID", job.ID, "error", err)
			} else {
				for _, line := range strings.Split(text, "\n") {
					md.WriteString("> " + line + "\n")
				}
			}
		}

		_ = w.store.UpdateImportJobProgress(ctx, job.ID, i+1, totalPages)
	}

	content := md.String()

	// Update the entry with the final content.
	if err := w.store.UpdateEntryContent(ctx, entry.ID, content); err != nil {
		return fmt.Errorf("update entry content: %w", err)
	}

	// Write content to the daily note.
	if dailyNote.Content != "" {
		separator := "\n\n---\n*Imported from Viwoods*\n\n"
		if err := w.store.AppendDailyNoteContent(ctx, dailyNote.ID, separator+content); err != nil {
			return fmt.Errorf("append daily note content: %w", err)
		}
	} else {
		if err := w.store.UpdateDailyNoteContent(ctx, dailyNote.ID, content); err != nil {
			return fmt.Errorf("update daily note content: %w", err)
		}
	}

	// Index if available.
	if w.indexer != nil && entry.Content != "" {
		if err := w.indexer.IndexEntry(ctx, entry.ID, entry.Content); err != nil {
			slog.Warn("failed to index viwoods entry", "entry_id", entry.ID, "error", err)
		}
	}

	result, _ := json.Marshal(map[string]any{
		"daily_note_id":   dailyNote.ID,
		"entry_id":        entry.ID,
		"date":            date,
		"pages_processed": totalPages,
	})
	_ = w.store.UpdateImportJobResult(ctx, job.ID, string(result))

	return nil
}

func (w *Worker) processObsidian(ctx context.Context, job *storage.ImportJob) error {
	data, err := os.ReadFile(job.FilePath)
	if err != nil {
		return fmt.Errorf("read file: %w", err)
	}

	date, err := ParseDateFromFilename(job.Filename)
	if err != nil {
		return fmt.Errorf("parse date from filename: %w", err)
	}

	dailyNote, err := w.store.GetOrCreateDailyNote(ctx, date)
	if err != nil {
		return fmt.Errorf("get or create daily note: %w", err)
	}

	_ = w.store.UpdateImportJobProgress(ctx, job.ID, 0, 1)

	content := string(data)

	if dailyNote.Content != "" {
		separator := "\n\n---\n*Imported from Obsidian*\n\n"
		if err := w.store.AppendDailyNoteContent(ctx, dailyNote.ID, separator+content); err != nil {
			return fmt.Errorf("append daily note content: %w", err)
		}
	} else {
		if err := w.store.UpdateDailyNoteContent(ctx, dailyNote.ID, content); err != nil {
			return fmt.Errorf("update daily note content: %w", err)
		}
	}

	entry := &storage.Entry{
		DailyNoteID: dailyNote.ID,
		Type:        "text",
		Content:     content,
		RawContent:  content,
		Source:      "import",
	}
	if err := w.store.CreateEntry(ctx, entry); err != nil {
		return fmt.Errorf("create entry: %w", err)
	}

	if w.indexer != nil && content != "" {
		if err := w.indexer.IndexEntry(ctx, entry.ID, content); err != nil {
			slog.Warn("failed to index obsidian entry", "entry_id", entry.ID, "error", err)
		}
	}

	_ = w.store.UpdateImportJobProgress(ctx, job.ID, 1, 1)

	result, _ := json.Marshal(map[string]any{
		"date":     date,
		"imported": 1,
	})
	_ = w.store.UpdateImportJobResult(ctx, job.ID, string(result))

	return nil
}

// ParseDateFromFilename extracts a YYYY-MM-DD date string from an Obsidian daily note filename.
// Exported so it can be reused by the API handler and worker.
func ParseDateFromFilename(filename string) (string, error) {
	name := strings.TrimSuffix(filename, ".md")

	// Try YYYY-MM-DD (with optional suffix after space, e.g. "2025-01-15 Wed")
	if len(name) >= 10 {
		candidate := name[:10]
		if _, err := time.Parse("2006-01-02", candidate); err == nil {
			return candidate, nil
		}
	}

	// Try YYYY_MM_DD
	if len(name) >= 10 {
		candidate := strings.ReplaceAll(name[:10], "_", "-")
		if _, err := time.Parse("2006-01-02", candidate); err == nil {
			return candidate, nil
		}
	}

	// Try DD-MM-YYYY
	if len(name) >= 10 {
		parts := strings.SplitN(name[:10], "-", 3)
		if len(parts) == 3 {
			candidate := parts[2] + "-" + parts[1] + "-" + parts[0]
			if _, err := time.Parse("2006-01-02", candidate); err == nil {
				return candidate, nil
			}
		}
	}

	return "", fmt.Errorf("cannot parse date from filename: %s", filename)
}
