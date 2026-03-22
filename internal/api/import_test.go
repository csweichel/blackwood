package api

import (
	"testing"
)

func TestParseDateFromFilename(t *testing.T) {
	tests := []struct {
		filename string
		want     string
		wantErr  bool
	}{
		// YYYY-MM-DD.md
		{"2025-01-15.md", "2025-01-15", false},
		// YYYY-MM-DD with day-of-week suffix
		{"2025-01-15 Wed.md", "2025-01-15", false},
		// YYYY_MM_DD.md (underscore variant)
		{"2025_01_15.md", "2025-01-15", false},
		// DD-MM-YYYY.md (European format)
		{"15-01-2025.md", "2025-01-15", false},
		// No .md extension still works
		{"2025-01-15", "2025-01-15", false},
		// Unparseable filenames
		{"random-notes.md", "", true},
		{"notes.md", "", true},
		{"abc", "", true},
		// Invalid date values
		{"2025-13-01.md", "", true},
		{"2025-00-15.md", "", true},
	}

	for _, tt := range tests {
		t.Run(tt.filename, func(t *testing.T) {
			got, err := parseDateFromFilename(tt.filename)
			if (err != nil) != tt.wantErr {
				t.Errorf("parseDateFromFilename(%q) error = %v, wantErr %v", tt.filename, err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("parseDateFromFilename(%q) = %q, want %q", tt.filename, got, tt.want)
			}
		})
	}
}
