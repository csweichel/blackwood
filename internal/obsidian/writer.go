package obsidian

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/csweichel/blackwood/internal/config"
)

// PageResult holds the OCR output for a single page.
type PageResult struct {
	PageID string
	Image  []byte // PNG data
	Text   string // recognized text (may be empty if OCR failed)
}

// NoteEntry represents a processed note ready to be written.
type NoteEntry struct {
	NoteID     string
	NoteName   string
	CreateTime time.Time
	Pages      []PageResult
}

// Writer handles writing processed notes to Obsidian daily notes.
type Writer struct {
	cfg config.ObsidianConfig
}

// New creates a Writer for the given Obsidian configuration.
func New(cfg config.ObsidianConfig) *Writer {
	return &Writer{cfg: cfg}
}

// Write appends a note entry to the appropriate daily note file.
func (w *Writer) Write(entry NoteEntry) error {
	dateStr := entry.CreateTime.Format(w.cfg.DailyFormat)
	dailyDir := filepath.Join(w.cfg.VaultPath, w.cfg.DailyNotesDir)
	dailyPath := filepath.Join(dailyDir, dateStr+".md")
	attachDir := filepath.Join(w.cfg.VaultPath, w.cfg.AttachmentsDir)

	// Ensure directories exist.
	if err := os.MkdirAll(dailyDir, 0o755); err != nil {
		return fmt.Errorf("creating daily notes directory: %w", err)
	}
	if err := os.MkdirAll(attachDir, 0o755); err != nil {
		return fmt.Errorf("creating attachments directory: %w", err)
	}

	// Copy page images to attachments.
	for i, p := range entry.Pages {
		imgName := fmt.Sprintf("blackwood-%s-p%d.png", entry.NoteID, i+1)
		imgPath := filepath.Join(attachDir, imgName)
		if err := os.WriteFile(imgPath, p.Image, 0o644); err != nil {
			return fmt.Errorf("writing page image %s: %w", imgName, err)
		}
	}

	// Read existing file content (if any) to check for Viwoods heading.
	existing, err := os.ReadFile(dailyPath)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("reading daily note: %w", err)
	}

	fileExists := err == nil
	hasViwoodsSection := fileExists && strings.Contains(string(existing), "## Viwoods Notes")

	f, err := os.OpenFile(dailyPath, os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0o644)
	if err != nil {
		return fmt.Errorf("opening daily note: %w", err)
	}
	defer f.Close()

	var b strings.Builder

	// If the file is new, write the date header.
	if !fileExists {
		b.WriteString("# ")
		b.WriteString(dateStr)
		b.WriteString("\n\n")
	}

	// Add the Viwoods section heading only if not already present.
	if !hasViwoodsSection {
		b.WriteString("\n## Viwoods Notes\n")
	}

	// Write the note subsection.
	b.WriteString("\n### ")
	b.WriteString(entry.NoteName)
	b.WriteString("\n")
	b.WriteString(fmt.Sprintf("*Captured at %s*\n", entry.CreateTime.Format("15:04")))

	for i, p := range entry.Pages {
		imgName := fmt.Sprintf("blackwood-%s-p%d.png", entry.NoteID, i+1)
		b.WriteString("\n![[")
		b.WriteString(imgName)
		b.WriteString("]]\n\n")

		if p.Text == "" {
			b.WriteString("> *[no text recognized]*\n")
		} else {
			lines := strings.Split(p.Text, "\n")
			for _, line := range lines {
				b.WriteString("> ")
				b.WriteString(line)
				b.WriteString("\n")
			}
		}
	}

	if _, err := f.WriteString(b.String()); err != nil {
		return fmt.Errorf("writing to daily note: %w", err)
	}

	return nil
}
