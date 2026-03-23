package noteparser

import (
	"archive/zip"
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strings"
	"time"
)

// Note represents a parsed .note file with its metadata and pages.
type Note struct {
	ID         string
	Name       string
	CreateTime time.Time
	Pages      []Page
}

// Page represents a single page within a note.
type Page struct {
	ID    string
	Order int
	Image []byte // PNG data
}

// noteFileInfo is the notebook-level metadata from *_NoteFileInfo.json.
type noteFileInfo struct {
	ID               string `json:"id"`
	FileName         string `json:"fileName"`
	CreationTime     int64  `json:"creationTime"`
	LastModifiedTime int64  `json:"lastModifiedTime"`
}

// pageInfo is a single page entry from *_PageListFileInfo.json.
type pageInfo struct {
	ID           string `json:"id"`
	FileName     string `json:"fileName"`
	Order        int    `json:"order"`
	CreationTime int64  `json:"creationTime"`
}

// Parse opens a .note file (a ZIP archive) and extracts metadata and page screenshots.
func Parse(path string) (*Note, error) {
	r, err := zip.OpenReader(path)
	if err != nil {
		return nil, fmt.Errorf("open zip: %w", err)
	}
	defer r.Close() //nolint:errcheck

	// Index all files by name for quick lookup.
	files := make(map[string]*zip.File, len(r.File))
	for _, f := range r.File {
		files[f.Name] = f
	}

	// Find the notebook name prefix by looking for *_NoteFileInfo.json.
	var prefix string
	for name := range files {
		if strings.HasSuffix(name, "_NoteFileInfo.json") {
			prefix = strings.TrimSuffix(name, "_NoteFileInfo.json")
			break
		}
	}
	if prefix == "" {
		return nil, fmt.Errorf("no *_NoteFileInfo.json found in archive")
	}

	// Parse notebook metadata.
	var info noteFileInfo
	if err := readJSON(files, prefix+"_NoteFileInfo.json", &info); err != nil {
		return nil, fmt.Errorf("read NoteFileInfo: %w", err)
	}

	// Parse page list.
	var pages []pageInfo
	if err := readJSON(files, prefix+"_PageListFileInfo.json", &pages); err != nil {
		return nil, fmt.Errorf("read PageListFileInfo: %w", err)
	}

	// Sort pages by order.
	sort.Slice(pages, func(i, j int) bool {
		return pages[i].Order < pages[j].Order
	})

	// Extract screenshot images for each page.
	var notePages []Page
	for _, p := range pages {
		// Screenshot files are named screenshotBmp_{pageId}.png
		screenshotName := "screenshotBmp_" + p.ID + ".png"
		imgData, err := readFile(files, screenshotName)
		if err != nil {
			// Skip pages with no screenshot available.
			continue
		}

		notePages = append(notePages, Page{
			ID:    p.ID,
			Order: p.Order,
			Image: imgData,
		})
	}

	return &Note{
		ID:         info.ID,
		Name:       info.FileName,
		CreateTime: time.UnixMilli(info.CreationTime),
		Pages:      notePages,
	}, nil
}

func readJSON(files map[string]*zip.File, name string, v any) error {
	f, ok := files[name]
	if !ok {
		return fmt.Errorf("file %q not found in archive", name)
	}
	rc, err := f.Open()
	if err != nil {
		return err
	}
	defer rc.Close() //nolint:errcheck
	return json.NewDecoder(rc).Decode(v)
}

func readFile(files map[string]*zip.File, name string) ([]byte, error) {
	f, ok := files[name]
	if !ok {
		return nil, fmt.Errorf("file %q not found in archive", name)
	}
	rc, err := f.Open()
	if err != nil {
		return nil, err
	}
	defer rc.Close() //nolint:errcheck
	return io.ReadAll(rc)
}
