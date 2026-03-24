package api

import (
	"fmt"
	"log/slog"
	"mime"
	"net/http"
	"os"
	"path/filepath"

	pdfgen "github.com/csweichel/blackwood/internal/pdf"
	"github.com/csweichel/blackwood/internal/storage"
)

// ServePDF returns an HTTP handler that generates a PDF for a daily note.
func ServePDF(store *storage.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		date := r.PathValue("date")
		if date == "" {
			http.Error(w, "missing date", http.StatusBadRequest)
			return
		}

		note, err := store.GetDailyNoteByDate(r.Context(), date)
		if err != nil {
			http.Error(w, "daily note not found", http.StatusNotFound)
			return
		}

		if note.Content == "" {
			http.Error(w, "daily note is empty", http.StatusNotFound)
			return
		}

		opts := pdfgen.Options{
			Date: date,
			ResolveAttachment: func(id string) ([]byte, string, error) {
				att, err := store.GetAttachment(r.Context(), id)
				if err != nil {
					return nil, "", err
				}
				data, err := store.GetAttachmentData(r.Context(), id)
				if err != nil {
					return nil, "", err
				}
				return data, att.ContentType, nil
			},
			ResolveFile: func(filename string) ([]byte, string, error) {
				path, err := store.AttachmentPath(date, filename)
				if err != nil {
					return nil, "", err
				}
				data, err := os.ReadFile(path)
				if err != nil {
					return nil, "", err
				}
				ct := mime.TypeByExtension(filepath.Ext(filename))
				if ct == "" {
					ct = http.DetectContentType(data)
				}
				return data, ct, nil
			},
		}

		w.Header().Set("Content-Type", "application/pdf")
		w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s.pdf"`, date))

		if err := pdfgen.Generate(w, note.Content, opts); err != nil {
			slog.Error("generate pdf", "date", date, "error", err)
			// Headers already sent, can't change status code
		}
	}
}
