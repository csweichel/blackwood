package api

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"strings"

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
