package api

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"connectrpc.com/connect"

	blackwoodv1 "github.com/csweichel/blackwood/gen/blackwood/v1"
	"github.com/csweichel/blackwood/internal/importqueue"
	"github.com/csweichel/blackwood/internal/noteparser"
	"github.com/csweichel/blackwood/internal/ocr"
	"github.com/csweichel/blackwood/internal/storage"
)

// ImportHandler implements the ImportService Connect handler.
type ImportHandler struct {
	store      *storage.Store
	recognizer ocr.Recognizer // may be nil if no LLM config
	indexer    EntryIndexer   // may be nil if chat index is disabled
	worker     *importqueue.Worker
	dataDir    string
}

// NewImportHandler creates a new ImportHandler backed by the given store and optional OCR recognizer.
func NewImportHandler(store *storage.Store, recognizer ocr.Recognizer, indexer EntryIndexer, worker *importqueue.Worker, dataDir string) *ImportHandler {
	return &ImportHandler{store: store, recognizer: recognizer, indexer: indexer, worker: worker, dataDir: dataDir}
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
	defer func() { _ = os.Remove(tmpFile.Name()) }()

	if _, err := tmpFile.Write(req.Msg.NoteFile); err != nil {
		_ = tmpFile.Close()
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("write temp file: %w", err))
	}
	if err := tmpFile.Close(); err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("close temp file: %w", err))
	}

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

	// Create a placeholder entry so we can attach page images to it.
	entry := &storage.Entry{
		DailyNoteID: dailyNote.ID,
		Type:        "viwoods",
		Content:     "",
		RawContent:  "",
		Source:      "import",
	}
	if err := h.store.CreateEntry(ctx, entry); err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("create entry: %w", err))
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
		if err := h.store.CreateAttachment(ctx, att, page.Image, date); err != nil {
			slog.Warn("failed to store page attachment", "page", i+1, "error", err)
		} else {
			fmt.Fprintf(&md, "![Page %d](%s)\n\n", i+1, filepath.Base(att.StoragePath))
		}

		if h.recognizer != nil {
			text, err := h.recognizer.Recognize(ctx, page.Image)
			if err != nil {
				slog.Warn("OCR failed for page", "page", i+1, "noteID", note.ID, "error", err)
			} else {
				md.WriteString(text + "\n")
			}
		}
	}

	content := md.String()

	// Update the entry with the final content.
	if err := h.store.UpdateEntryContent(ctx, entry.ID, content); err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("update entry content: %w", err))
	}

	// Write content to the daily note file so it appears in calendar and daily note view.
	if dailyNote.Content != "" {
		separator := "\n\n---\n*Imported from Viwoods*\n\n"
		if err := h.store.AppendDailyNoteContent(ctx, dailyNote.ID, separator+content); err != nil {
			return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("append daily note content: %w", err))
		}
	} else {
		if err := h.store.UpdateDailyNoteContent(ctx, dailyNote.ID, content); err != nil {
			return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("update daily note content: %w", err))
		}
	}

	if h.indexer != nil && content != "" {
		if err := h.indexer.IndexEntry(ctx, entry.ID, entry.Content); err != nil {
			slog.Warn("index viwoods entry failed", "entry_id", entry.ID, "error", err)
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
		} else if h.indexer != nil {
			if err := h.indexer.IndexEntry(ctx, entry.ID, entry.Content); err != nil {
				slog.Warn("index obsidian entry failed", "entry_id", entry.ID, "error", err)
			}
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
	return importqueue.ParseDateFromFilename(filename)
}

// SubmitImport accepts files for background import processing.
func (h *ImportHandler) SubmitImport(ctx context.Context, req *connect.Request[blackwoodv1.SubmitImportRequest]) (*connect.Response[blackwoodv1.SubmitImportResponse], error) {
	if len(req.Msg.Files) == 0 {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("at least one file is required"))
	}

	var jobIDs []string
	for _, f := range req.Msg.Files {
		// Determine file type from extension.
		var fileType string
		if strings.HasSuffix(f.Filename, ".note") {
			fileType = "viwoods"
		} else if strings.HasSuffix(f.Filename, ".md") {
			fileType = "obsidian"
		} else {
			return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("unsupported file type: %s", f.Filename))
		}

		// Create staging directory and write file before inserting the job,
		// so the job row already has the correct file_path.
		jobID := storage.NewUUID()
		stagingDir := filepath.Join(h.dataDir, "import-staging", jobID)
		if err := os.MkdirAll(stagingDir, 0o755); err != nil {
			return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("create staging dir: %w", err))
		}
		filePath := filepath.Join(stagingDir, f.Filename)
		if err := os.WriteFile(filePath, f.Content, 0o644); err != nil {
			return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("write staging file: %w", err))
		}

		job := &storage.ImportJob{
			ID:       jobID,
			Status:   "pending",
			Filename: f.Filename,
			FileType: fileType,
			FilePath: filePath,
			Source:   "upload",
		}
		if err := h.store.CreateImportJob(ctx, job); err != nil {
			return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("create import job: %w", err))
		}

		jobIDs = append(jobIDs, job.ID)
	}

	// Wake the worker.
	h.worker.Enqueue()

	return connect.NewResponse(&blackwoodv1.SubmitImportResponse{
		JobIds: jobIDs,
	}), nil
}

// GetImportJobs returns the status of import jobs.
func (h *ImportHandler) GetImportJobs(ctx context.Context, req *connect.Request[blackwoodv1.GetImportJobsRequest]) (*connect.Response[blackwoodv1.GetImportJobsResponse], error) {
	jobs, err := h.store.ListImportJobs(ctx, req.Msg.Ids, req.Msg.ActiveOnly)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("list import jobs: %w", err))
	}

	pbJobs := make([]*blackwoodv1.ImportJobStatus, len(jobs))
	for i, j := range jobs {
		pbJobs[i] = &blackwoodv1.ImportJobStatus{
			Id:         j.ID,
			Status:     j.Status,
			Filename:   j.Filename,
			FileType:   j.FileType,
			Source:     j.Source,
			Progress:   int32(j.Progress),
			TotalSteps: int32(j.TotalSteps),
			ResultJson: j.Result,
			CreatedAt:  j.CreatedAt.Format(time.RFC3339),
			UpdatedAt:  j.UpdatedAt.Format(time.RFC3339),
		}
	}

	return connect.NewResponse(&blackwoodv1.GetImportJobsResponse{
		Jobs: pbJobs,
	}), nil
}

// DeleteImportJob removes an import job and its staging file.
func (h *ImportHandler) DeleteImportJob(ctx context.Context, req *connect.Request[blackwoodv1.DeleteImportJobRequest]) (*connect.Response[blackwoodv1.DeleteImportJobResponse], error) {
	if req.Msg.Id == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("id is required"))
	}
	if err := h.store.DeleteImportJob(ctx, req.Msg.Id); err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("delete import job: %w", err))
	}
	return connect.NewResponse(&blackwoodv1.DeleteImportJobResponse{}), nil
}
