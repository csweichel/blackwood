package state

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"time"
)

// FileRecord holds metadata about a processed file.
type FileRecord struct {
	Path        string    `json:"path"`
	Hash        string    `json:"hash"`
	ProcessedAt time.Time `json:"processed_at"`
}

// State tracks which files have been processed.
type State struct {
	Files    map[string]FileRecord `json:"files"`
	filePath string
}

// Load reads the state file at path. If the file does not exist (first run),
// it returns an empty state without error.
func Load(path string) (*State, error) {
	s := &State{
		Files:    make(map[string]FileRecord),
		filePath: path,
	}

	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return s, nil
		}
		return nil, fmt.Errorf("reading state file: %w", err)
	}

	if err := json.Unmarshal(data, &s.Files); err != nil {
		return nil, fmt.Errorf("parsing state file: %w", err)
	}

	return s, nil
}

// Save writes the current state to disk as indented JSON.
func (s *State) Save() error {
	data, err := json.MarshalIndent(s.Files, "", "  ")
	if err != nil {
		return fmt.Errorf("marshalling state: %w", err)
	}

	if err := os.WriteFile(s.filePath, data, 0644); err != nil {
		return fmt.Errorf("writing state file: %w", err)
	}

	return nil
}

// IsProcessed reports whether filePath has been recorded in the state.
func (s *State) IsProcessed(filePath string) bool {
	_, ok := s.Files[filePath]
	return ok
}

// MarkProcessed adds or updates the record for filePath with the given hash.
func (s *State) MarkProcessed(filePath, hash string) {
	s.Files[filePath] = FileRecord{
		Path:        filePath,
		Hash:        hash,
		ProcessedAt: time.Now(),
	}
}

// ComputeHash returns the SHA-256 hex digest of the file at filePath.
func ComputeHash(filePath string) (string, error) {
	f, err := os.Open(filePath)
	if err != nil {
		return "", fmt.Errorf("opening file for hashing: %w", err)
	}
	defer f.Close() //nolint:errcheck

	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", fmt.Errorf("hashing file: %w", err)
	}

	return hex.EncodeToString(h.Sum(nil)), nil
}
