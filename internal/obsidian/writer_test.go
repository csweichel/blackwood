package obsidian

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/csweichel/blackwood/internal/config"
)

func testConfig(t *testing.T) config.ObsidianConfig {
	t.Helper()
	vault := t.TempDir()
	return config.ObsidianConfig{
		VaultPath:      vault,
		DailyNotesDir:  "Daily Notes",
		DailyFormat:    "2006-01-02",
		AttachmentsDir: "attachments",
	}
}

func testEntry() NoteEntry {
	return NoteEntry{
		NoteID:     "abc123",
		NoteName:   "Meeting Notes",
		CreateTime: time.Date(2025, 1, 15, 14, 30, 0, 0, time.UTC),
		Pages: []PageResult{
			{PageID: "p1", Image: []byte("png-data-1"), Text: "Hello world"},
			{PageID: "p2", Image: []byte("png-data-2"), Text: "Second page"},
		},
	}
}

func TestWriteNewDailyNote(t *testing.T) {
	cfg := testConfig(t)
	w := New(cfg)
	entry := testEntry()

	if err := w.Write(entry); err != nil {
		t.Fatalf("Write: %v", err)
	}

	dailyPath := filepath.Join(cfg.VaultPath, "Daily Notes", "2025-01-15.md")
	data, err := os.ReadFile(dailyPath)
	if err != nil {
		t.Fatalf("reading daily note: %v", err)
	}
	content := string(data)

	// Check header.
	if !strings.HasPrefix(content, "# 2025-01-15\n\n") {
		t.Errorf("missing date header, got:\n%s", content)
	}
	// Check Viwoods section heading.
	if !strings.Contains(content, "## Viwoods Notes") {
		t.Error("missing ## Viwoods Notes heading")
	}
	// Check note subsection.
	if !strings.Contains(content, "### Meeting Notes") {
		t.Error("missing ### Meeting Notes heading")
	}
	// Check capture time.
	if !strings.Contains(content, "*Captured at 14:30*") {
		t.Error("missing capture time")
	}
	// Check image embeds.
	if !strings.Contains(content, "![[blackwood-abc123-p1.png]]") {
		t.Error("missing page 1 image embed")
	}
	if !strings.Contains(content, "![[blackwood-abc123-p2.png]]") {
		t.Error("missing page 2 image embed")
	}
	// Check blockquoted text.
	if !strings.Contains(content, "> Hello world") {
		t.Error("missing blockquoted text for page 1")
	}
	if !strings.Contains(content, "> Second page") {
		t.Error("missing blockquoted text for page 2")
	}
}

func TestWriteExistingDailyNoteNoDuplicateHeading(t *testing.T) {
	cfg := testConfig(t)
	w := New(cfg)
	entry1 := testEntry()

	if err := w.Write(entry1); err != nil {
		t.Fatalf("Write first: %v", err)
	}

	entry2 := NoteEntry{
		NoteID:     "def456",
		NoteName:   "Afternoon Notes",
		CreateTime: time.Date(2025, 1, 15, 16, 0, 0, 0, time.UTC),
		Pages: []PageResult{
			{PageID: "p3", Image: []byte("png-data-3"), Text: "Afternoon text"},
		},
	}

	if err := w.Write(entry2); err != nil {
		t.Fatalf("Write second: %v", err)
	}

	dailyPath := filepath.Join(cfg.VaultPath, "Daily Notes", "2025-01-15.md")
	data, err := os.ReadFile(dailyPath)
	if err != nil {
		t.Fatalf("reading daily note: %v", err)
	}
	content := string(data)

	// The "## Viwoods Notes" heading should appear exactly once.
	count := strings.Count(content, "## Viwoods Notes")
	if count != 1 {
		t.Errorf("expected 1 '## Viwoods Notes' heading, got %d", count)
	}

	// Both notes should be present.
	if !strings.Contains(content, "### Meeting Notes") {
		t.Error("missing first note")
	}
	if !strings.Contains(content, "### Afternoon Notes") {
		t.Error("missing second note")
	}
}

func TestMultipleNotesSameDay(t *testing.T) {
	cfg := testConfig(t)
	w := New(cfg)

	for i, name := range []string{"Note A", "Note B", "Note C"} {
		entry := NoteEntry{
			NoteID:     fmt.Sprintf("note%d", i),
			NoteName:   name,
			CreateTime: time.Date(2025, 3, 10, 9+i, 0, 0, 0, time.UTC),
			Pages: []PageResult{
				{PageID: "px", Image: []byte("img"), Text: fmt.Sprintf("text %d", i)},
			},
		}
		if err := w.Write(entry); err != nil {
			t.Fatalf("Write %q: %v", name, err)
		}
	}

	dailyPath := filepath.Join(cfg.VaultPath, "Daily Notes", "2025-03-10.md")
	data, err := os.ReadFile(dailyPath)
	if err != nil {
		t.Fatalf("reading daily note: %v", err)
	}
	content := string(data)

	for _, name := range []string{"Note A", "Note B", "Note C"} {
		if !strings.Contains(content, "### "+name) {
			t.Errorf("missing note %q", name)
		}
	}

	// Still only one Viwoods heading.
	if c := strings.Count(content, "## Viwoods Notes"); c != 1 {
		t.Errorf("expected 1 Viwoods heading, got %d", c)
	}
}

func TestPageImagesCopiedToAttachments(t *testing.T) {
	cfg := testConfig(t)
	w := New(cfg)
	entry := testEntry()

	if err := w.Write(entry); err != nil {
		t.Fatalf("Write: %v", err)
	}

	for i, p := range entry.Pages {
		imgName := fmt.Sprintf("blackwood-%s-p%d.png", entry.NoteID, i+1)
		imgPath := filepath.Join(cfg.VaultPath, "attachments", imgName)
		data, err := os.ReadFile(imgPath)
		if err != nil {
			t.Fatalf("reading attachment %s: %v", imgName, err)
		}
		if string(data) != string(p.Image) {
			t.Errorf("attachment %s content mismatch", imgName)
		}
	}
}

func TestEmptyOCRTextPlaceholder(t *testing.T) {
	cfg := testConfig(t)
	w := New(cfg)
	entry := NoteEntry{
		NoteID:     "empty1",
		NoteName:   "Blank Page",
		CreateTime: time.Date(2025, 2, 1, 10, 0, 0, 0, time.UTC),
		Pages: []PageResult{
			{PageID: "pe", Image: []byte("img"), Text: ""},
		},
	}

	if err := w.Write(entry); err != nil {
		t.Fatalf("Write: %v", err)
	}

	dailyPath := filepath.Join(cfg.VaultPath, "Daily Notes", "2025-02-01.md")
	data, err := os.ReadFile(dailyPath)
	if err != nil {
		t.Fatalf("reading daily note: %v", err)
	}
	content := string(data)

	if !strings.Contains(content, "> *[no text recognized]*") {
		t.Error("missing placeholder for empty OCR text")
	}
}

func TestDirectoriesCreatedIfMissing(t *testing.T) {
	cfg := testConfig(t)
	// Use nested subdirectories that don't exist yet.
	cfg.DailyNotesDir = "notes/daily/sub"
	cfg.AttachmentsDir = "files/attach/sub"

	w := New(cfg)
	entry := NoteEntry{
		NoteID:     "dir1",
		NoteName:   "Dir Test",
		CreateTime: time.Date(2025, 4, 1, 8, 0, 0, 0, time.UTC),
		Pages: []PageResult{
			{PageID: "pd", Image: []byte("img"), Text: "text"},
		},
	}

	if err := w.Write(entry); err != nil {
		t.Fatalf("Write: %v", err)
	}

	// Verify daily note was created.
	dailyPath := filepath.Join(cfg.VaultPath, "notes/daily/sub", "2025-04-01.md")
	if _, err := os.Stat(dailyPath); err != nil {
		t.Errorf("daily note not created: %v", err)
	}

	// Verify attachment was created.
	imgPath := filepath.Join(cfg.VaultPath, "files/attach/sub", "blackwood-dir1-p1.png")
	if _, err := os.Stat(imgPath); err != nil {
		t.Errorf("attachment not created: %v", err)
	}
}
