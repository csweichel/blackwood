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

func buildNoteZIP(t *testing.T, dir string, name string, info noteFileInfo, pages []pageInfo) string {
	t.Helper()

	path := filepath.Join(dir, name+".note")
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close() //nolint:errcheck

	w := zip.NewWriter(f)
	defer w.Close() //nolint:errcheck

	// Write NoteFileInfo.json
	writeJSON(t, w, name+"_NoteFileInfo.json", info)

	// Write PageListFileInfo.json
	writeJSON(t, w, name+"_PageListFileInfo.json", pages)

	// Write screenshot PNGs for each page
	for _, p := range pages {
		writePNG(t, w, "screenshotBmp_"+p.ID+".png")
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
	info := noteFileInfo{
		ID:           "test-note-id",
		FileName:     "TestNote",
		CreationTime: 1774015890054,
	}
	pages := []pageInfo{
		{ID: "page-b", Order: 1, CreationTime: 1774015890055},
		{ID: "page-a", Order: 0, CreationTime: 1774015890055},
	}
	path := buildNoteZIP(t, dir, "TestNote", info, pages)

	note, err := Parse(path)
	if err != nil {
		t.Fatalf("Parse() error: %v", err)
	}

	if note.ID != "test-note-id" {
		t.Errorf("ID = %q, want %q", note.ID, "test-note-id")
	}
	if note.Name != "TestNote" {
		t.Errorf("Name = %q, want %q", note.Name, "TestNote")
	}
	if len(note.Pages) != 2 {
		t.Fatalf("len(Pages) = %d, want 2", len(note.Pages))
	}
	// Should be sorted by order: page-a (order 0) first
	if note.Pages[0].ID != "page-a" {
		t.Errorf("Pages[0].ID = %q, want %q", note.Pages[0].ID, "page-a")
	}
	if note.Pages[1].ID != "page-b" {
		t.Errorf("Pages[1].ID = %q, want %q", note.Pages[1].ID, "page-b")
	}
	for i, p := range note.Pages {
		if len(p.Image) == 0 {
			t.Errorf("Pages[%d].Image is empty", i)
		}
	}
}

func TestParse_CreateTime(t *testing.T) {
	dir := t.TempDir()
	info := noteFileInfo{
		ID:           "time-test",
		FileName:     "TimeNote",
		CreationTime: 1774015890054,
	}
	pages := []pageInfo{
		{ID: "page-1", Order: 0, CreationTime: 1774015890055},
	}
	path := buildNoteZIP(t, dir, "TimeNote", info, pages)

	note, err := Parse(path)
	if err != nil {
		t.Fatalf("Parse() error: %v", err)
	}

	want := time.UnixMilli(1774015890054)
	if !note.CreateTime.Equal(want) {
		t.Errorf("CreateTime = %v, want %v", note.CreateTime, want)
	}
}

func TestParse_InvalidFile(t *testing.T) {
	dir := t.TempDir()
	badPath := filepath.Join(dir, "bad.note")
	if err := os.WriteFile(badPath, []byte("not a zip"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Parse(badPath); err == nil {
		t.Error("Parse(invalid file) should return error")
	}
	if _, err := Parse(filepath.Join(dir, "missing.note")); err == nil {
		t.Error("Parse(missing file) should return error")
	}
}
