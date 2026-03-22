package noteparser

import (
	"archive/zip"
	"encoding/json"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// buildNoteZIP creates a synthetic .note ZIP file and returns its path.
// The caller is responsible for cleaning up the temp directory.
func buildNoteZIP(t *testing.T, dir string, pages []noteListEntry) string {
	t.Helper()

	const (
		noteID   = "17471854767311408"
		noteName = "TestNote"
	)
	createTime := int64(1747185476731)

	path := filepath.Join(dir, "test.note")
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close() //nolint:errcheck

	w := zip.NewWriter(f)
	defer w.Close() //nolint:errcheck

	// Write NotesBean.json
	bean := notesBean{
		NoteID:     noteID,
		NoteName:   noteName,
		CreateTime: createTime,
	}
	writeJSON(t, w, noteName+"_NotesBean.json", bean)

	// Write NoteList.json
	writeJSON(t, w, noteName+"_NoteList.json", pages)

	// Write page PNGs (1x1 pixel)
	for _, p := range pages {
		writePNG(t, w, p.PageID+".png")
	}

	return path
}

func writeJSON(t *testing.T, w *zip.Writer, name string, v any) {
	t.Helper()
	fw, err := w.Create(name)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.NewEncoder(fw).Encode(v); err != nil {
		t.Fatal(err)
	}
}

func writePNG(t *testing.T, w *zip.Writer, name string) {
	t.Helper()
	fw, err := w.Create(name)
	if err != nil {
		t.Fatal(err)
	}
	img := image.NewRGBA(image.Rect(0, 0, 1, 1))
	img.Set(0, 0, color.RGBA{R: 255, A: 255})
	if err := png.Encode(fw, img); err != nil {
		t.Fatal(err)
	}
}

func TestParse_TwoPages(t *testing.T) {
	dir := t.TempDir()
	pages := []noteListEntry{
		{PageID: "page_b", PathOrder: 2000},
		{PageID: "page_a", PathOrder: 1000},
	}
	path := buildNoteZIP(t, dir, pages)

	note, err := Parse(path)
	if err != nil {
		t.Fatalf("Parse() error: %v", err)
	}

	if note.ID != "17471854767311408" {
		t.Errorf("ID = %q, want %q", note.ID, "17471854767311408")
	}
	if note.Name != "TestNote" {
		t.Errorf("Name = %q, want %q", note.Name, "TestNote")
	}

	if len(note.Pages) != 2 {
		t.Fatalf("len(Pages) = %d, want 2", len(note.Pages))
	}

	// Pages should be sorted by PathOrder ascending.
	if note.Pages[0].ID != "page_a" {
		t.Errorf("Pages[0].ID = %q, want %q", note.Pages[0].ID, "page_a")
	}
	if note.Pages[1].ID != "page_b" {
		t.Errorf("Pages[1].ID = %q, want %q", note.Pages[1].ID, "page_b")
	}

	// Each page should have non-empty image data.
	for i, p := range note.Pages {
		if len(p.Image) == 0 {
			t.Errorf("Pages[%d].Image is empty", i)
		}
	}
}

func TestParse_CreateTime(t *testing.T) {
	dir := t.TempDir()
	pages := []noteListEntry{
		{PageID: "page_1", PathOrder: 1},
	}
	path := buildNoteZIP(t, dir, pages)

	note, err := Parse(path)
	if err != nil {
		t.Fatalf("Parse() error: %v", err)
	}

	want := time.UnixMilli(1747185476731)
	if !note.CreateTime.Equal(want) {
		t.Errorf("CreateTime = %v, want %v", note.CreateTime, want)
	}
}

func TestParse_SinglePage(t *testing.T) {
	dir := t.TempDir()
	pages := []noteListEntry{
		{PageID: "only_page", PathOrder: 100},
	}
	path := buildNoteZIP(t, dir, pages)

	note, err := Parse(path)
	if err != nil {
		t.Fatalf("Parse() error: %v", err)
	}

	if len(note.Pages) != 1 {
		t.Fatalf("len(Pages) = %d, want 1", len(note.Pages))
	}
	if note.Pages[0].ID != "only_page" {
		t.Errorf("Pages[0].ID = %q, want %q", note.Pages[0].ID, "only_page")
	}
}

func TestParse_InvalidFile(t *testing.T) {
	dir := t.TempDir()

	// Not a ZIP file.
	badPath := filepath.Join(dir, "bad.note")
	if err := os.WriteFile(badPath, []byte("not a zip"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Parse(badPath); err == nil {
		t.Error("Parse(invalid file) should return error")
	}

	// Non-existent file.
	if _, err := Parse(filepath.Join(dir, "missing.note")); err == nil {
		t.Error("Parse(missing file) should return error")
	}
}
