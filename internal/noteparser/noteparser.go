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

// notesBean maps the *_NotesBean.json structure.
type notesBean struct {
	NoteID     string `json:"noteId"`
	NoteName   string `json:"noteName"`
	CreateTime int64  `json:"createTime"`
}

// noteListEntry maps a single entry in *_NoteList.json.
type noteListEntry struct {
	PageID    string `json:"pageId"`
	PathOrder int    `json:"pathOrder"`
}

// Parse opens a .note file (a ZIP archive) and extracts metadata and page images.
func Parse(path string) (*Note, error) {
	r, err := zip.OpenReader(path)
	if err != nil {
		return nil, fmt.Errorf("opening note file: %w", err)
	}
	defer r.Close() //nolint:errcheck

	// Index ZIP entries by name for fast lookup.
	files := make(map[string]*zip.File, len(r.File))
	for _, f := range r.File {
		files[f.Name] = f
	}

	// Find *_NotesBean.json
	bean, err := findAndParse[notesBean](files, "_NotesBean.json")
	if err != nil {
		return nil, fmt.Errorf("reading notes bean: %w", err)
	}

	// Find *_NoteList.json
	var noteList []noteListEntry
	for name, f := range files {
		if strings.HasSuffix(name, "_NoteList.json") {
			data, err := readZipFile(f)
			if err != nil {
				return nil, fmt.Errorf("reading note list: %w", err)
			}
			if err := json.Unmarshal(data, &noteList); err != nil {
				return nil, fmt.Errorf("parsing note list: %w", err)
			}
			break
		}
	}
	if noteList == nil {
		return nil, fmt.Errorf("no *_NoteList.json found in archive")
	}

	// Sort pages by PathOrder.
	sort.Slice(noteList, func(i, j int) bool {
		return noteList[i].PathOrder < noteList[j].PathOrder
	})

	// Extract page images.
	pages := make([]Page, 0, len(noteList))
	for _, entry := range noteList {
		imgName := entry.PageID + ".png"
		imgFile, ok := files[imgName]
		if !ok {
			return nil, fmt.Errorf("page image %s not found in archive", imgName)
		}
		imgData, err := readZipFile(imgFile)
		if err != nil {
			return nil, fmt.Errorf("reading page image %s: %w", imgName, err)
		}
		pages = append(pages, Page{
			ID:    entry.PageID,
			Order: entry.PathOrder,
			Image: imgData,
		})
	}

	return &Note{
		ID:         bean.NoteID,
		Name:       bean.NoteName,
		CreateTime: time.UnixMilli(bean.CreateTime),
		Pages:      pages,
	}, nil
}

// findAndParse locates a ZIP entry by suffix and JSON-decodes it into T.
func findAndParse[T any](files map[string]*zip.File, suffix string) (*T, error) {
	for name, f := range files {
		if strings.HasSuffix(name, suffix) {
			data, err := readZipFile(f)
			if err != nil {
				return nil, err
			}
			var v T
			if err := json.Unmarshal(data, &v); err != nil {
				return nil, fmt.Errorf("parsing %s: %w", name, err)
			}
			return &v, nil
		}
	}
	return nil, fmt.Errorf("no *%s found in archive", suffix)
}

func readZipFile(f *zip.File) ([]byte, error) {
	rc, err := f.Open()
	if err != nil {
		return nil, err
	}
	defer rc.Close() //nolint:errcheck
	return io.ReadAll(rc)
}
