package api

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"time"

	"connectrpc.com/connect"

	blackwoodv1 "github.com/csweichel/blackwood/gen/blackwood/v1"
	"github.com/csweichel/blackwood/internal/noteparser"
	"github.com/csweichel/blackwood/internal/ocr"
	"github.com/csweichel/blackwood/internal/storage"
)

// ImportHandler implements the ImportService Connect handler.
type ImportHandler struct {
	store      *storage.Store
	recognizer ocr.Recognizer // may be nil if no LLM config
}

// NewImportHandler creates a new ImportHandler backed by the given store and optional OCR recognizer.
func NewImportHandler(store *storage.Store, recognizer ocr.Recognizer) *ImportHandler {
	return &ImportHandler{store: store, recognizer: recognizer}
}

// ImportViwoods parses an uploaded .note file, runs OCR on each page, and stores the result.
func (h *ImportHandler) ImportViwoods(ctx context.Context, req *connect.Request[blackwoodv1.ImportViwoodsRequest]) (*connect.Response[blackwoodv1.ImportViwoodsResponse], error) {
	if len(req.Msg.NoteFile) == 0 {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("note_file is required"))
	}

	// Write uploaded bytes to a temp file so noteparser.Parse can open it.
	tmpFile, err := os.CreateTemp("", "viwoods-import-*.note")
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("create temp file: %w", err))
	}
	defer os.Remove(tmpFile.Name())

	if _, err := tmpFile.Write(req.Msg.NoteFile); err != nil {
		tmpFile.Close()
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("write temp file: %w", err))
	}
	tmpFile.Close()

	// Parse the .note ZIP archive.
	note, err := noteparser.Parse(tmpFile.Name())
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("parse note file: %w", err))
	}

	// Get or create the daily note for the note's creation date.
	date := note.CreateTime.UTC().Format("2006-01-02")
	dailyNote, err := h.store.GetOrCreateDailyNote(ctx, date)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("get or create daily note: %w", err))
	}

	// Run OCR on each page and build markdown content.
	var md strings.Builder
	md.WriteString("## " + note.Name + "\n")

	for i, page := range note.Pages {
		md.WriteString(fmt.Sprintf("\n### Page %d\n", i+1))

		if h.recognizer != nil {
			text, err := h.recognizer.Recognize(ctx, page.Image)
			if err != nil {
				slog.Warn("OCR failed for page", "page", i+1, "noteID", note.ID, "error", err)
			} else {
				// Format recognized text as blockquote lines.
				for _, line := range strings.Split(text, "\n") {
					md.WriteString("> " + line + "\n")
				}
			}
		}
	}

	content := md.String()

	// Create the entry.
	entry := &storage.Entry{
		DailyNoteID: dailyNote.ID,
		Type:        "viwoods",
		Content:     content,
		RawContent:  content,
		Source:      "import",
	}
	if err := h.store.CreateEntry(ctx, entry); err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("create entry: %w", err))
	}

	// Store each page PNG as an attachment.
	for i, page := range note.Pages {
		att := &storage.Attachment{
			EntryID:     entry.ID,
			Filename:    fmt.Sprintf("page_%d.png", i+1),
			ContentType: "image/png",
		}
		if err := h.store.CreateAttachment(ctx, att, page.Image); err != nil {
			slog.Warn("failed to store page attachment", "page", i+1, "error", err)
		}
	}

	return connect.NewResponse(&blackwoodv1.ImportViwoodsResponse{
		DailyNoteId:    dailyNote.ID,
		EntryId:        entry.ID,
		PagesProcessed: int32(len(note.Pages)),
	}), nil
}

// ImportObsidian imports Obsidian daily note markdown files.
func (h *ImportHandler) ImportObsidian(ctx context.Context, req *connect.Request[blackwoodv1.ImportObsidianRequest]) (*connect.Response[blackwoodv1.ImportObsidianResponse], error) {
	var imported, skipped int32
	var errors []string

	for _, f := range req.Msg.Files {
		date, err := parseDateFromFilename(f.Filename)
		if err != nil {
			errors = append(errors, fmt.Sprintf("%s: %v", f.Filename, err))
			skipped++
			continue
		}

		dailyNote, err := h.store.GetOrCreateDailyNote(ctx, date)
		if err != nil {
			errors = append(errors, fmt.Sprintf("%s: %v", f.Filename, err))
			skipped++
			continue
		}

		content := string(f.Content)

		if dailyNote.Content != "" {
			separator := "\n\n---\n*Imported from Obsidian*\n\n"
			if err := h.store.AppendDailyNoteContent(ctx, dailyNote.ID, separator+content); err != nil {
				errors = append(errors, fmt.Sprintf("%s: %v", f.Filename, err))
				skipped++
				continue
			}
		} else {
			if err := h.store.UpdateDailyNoteContent(ctx, dailyNote.ID, content); err != nil {
				errors = append(errors, fmt.Sprintf("%s: %v", f.Filename, err))
				skipped++
				continue
			}
		}

		// Create an entry for audit trail.
		entry := &storage.Entry{
			DailyNoteID: dailyNote.ID,
			Type:        "text",
			Content:     content,
			RawContent:  content,
			Source:      "import",
		}
		if err := h.store.CreateEntry(ctx, entry); err != nil {
			slog.Warn("failed to create audit entry for obsidian import", "file", f.Filename, "error", err)
		}

		imported++
	}

	return connect.NewResponse(&blackwoodv1.ImportObsidianResponse{
		Imported: imported,
		Skipped:  skipped,
		Errors:   errors,
	}), nil
}

// parseDateFromFilename extracts a YYYY-MM-DD date string from an Obsidian daily note filename.
func parseDateFromFilename(filename string) (string, error) {
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
