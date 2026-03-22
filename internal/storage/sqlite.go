package storage

import (
	"context"
	"crypto/rand"
	"database/sql"
	_ "embed"
	"fmt"
	"os"
	"path/filepath"
	"time"

	_ "modernc.org/sqlite"
)

//go:embed schema.sql
var schema string

// Store provides SQLite-backed storage for daily notes, entries, and attachments.
type Store struct {
	db      *sql.DB
	dataDir string
}

// DailyNote represents a single day's note container.
type DailyNote struct {
	ID        string
	Date      string // YYYY-MM-DD
	CreatedAt time.Time
	UpdatedAt time.Time
}

// Entry represents a piece of content within a daily note.
type Entry struct {
	ID          string
	DailyNoteID string
	Type        string // "text", "audio", "photo", "viwoods", "webclip"
	Content     string // markdown
	RawContent  string // original input
	Source      string // "web", "telegram", "whatsapp", "api", "import"
	Metadata    string // JSON
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

// Attachment represents a file attached to an entry.
type Attachment struct {
	ID          string
	EntryID     string
	Filename    string
	ContentType string
	Size        int64
	StoragePath string
	CreatedAt   time.Time
}

func newUUID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}

// New opens (or creates) a SQLite database at dbPath, runs migrations,
// and returns a ready-to-use Store. dataDir is used for attachment file storage.
func New(dbPath string, dataDir string) (*Store, error) {
	db, err := sql.Open("sqlite", dbPath+"?_pragma=foreign_keys(1)")
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}
	if _, err := db.Exec(schema); err != nil {
		db.Close()
		return nil, fmt.Errorf("run migrations: %w", err)
	}
	return &Store{db: db, dataDir: dataDir}, nil
}

// Close closes the underlying database connection.
func (s *Store) Close() error {
	return s.db.Close()
}

// --- Daily notes ---

// CreateDailyNote inserts a new daily note for the given date (YYYY-MM-DD).
func (s *Store) CreateDailyNote(ctx context.Context, date string) (*DailyNote, error) {
	now := time.Now().UTC()
	id := newUUID()
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO daily_notes (id, date, created_at, updated_at) VALUES (?, ?, ?, ?)`,
		id, date, now, now,
	)
	if err != nil {
		return nil, fmt.Errorf("create daily note: %w", err)
	}
	return &DailyNote{ID: id, Date: date, CreatedAt: now, UpdatedAt: now}, nil
}

// GetDailyNote retrieves a daily note by its ID.
func (s *Store) GetDailyNote(ctx context.Context, id string) (*DailyNote, error) {
	var n DailyNote
	err := s.db.QueryRowContext(ctx,
		`SELECT id, date, created_at, updated_at FROM daily_notes WHERE id = ?`, id,
	).Scan(&n.ID, &n.Date, &n.CreatedAt, &n.UpdatedAt)
	if err != nil {
		return nil, fmt.Errorf("get daily note: %w", err)
	}
	return &n, nil
}

// GetDailyNoteByDate retrieves a daily note by its date string.
func (s *Store) GetDailyNoteByDate(ctx context.Context, date string) (*DailyNote, error) {
	var n DailyNote
	err := s.db.QueryRowContext(ctx,
		`SELECT id, date, created_at, updated_at FROM daily_notes WHERE date = ?`, date,
	).Scan(&n.ID, &n.Date, &n.CreatedAt, &n.UpdatedAt)
	if err != nil {
		return nil, fmt.Errorf("get daily note by date: %w", err)
	}
	return &n, nil
}

// GetOrCreateDailyNote returns the existing daily note for the given date,
// or creates one if it does not exist.
func (s *Store) GetOrCreateDailyNote(ctx context.Context, date string) (*DailyNote, error) {
	n, err := s.GetDailyNoteByDate(ctx, date)
	if err == nil {
		return n, nil
	}
	return s.CreateDailyNote(ctx, date)
}

// ListDailyNotes returns daily notes ordered by date descending.
func (s *Store) ListDailyNotes(ctx context.Context, limit, offset int) ([]DailyNote, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, date, created_at, updated_at FROM daily_notes ORDER BY date DESC LIMIT ? OFFSET ?`,
		limit, offset,
	)
	if err != nil {
		return nil, fmt.Errorf("list daily notes: %w", err)
	}
	defer rows.Close()

	var notes []DailyNote
	for rows.Next() {
		var n DailyNote
		if err := rows.Scan(&n.ID, &n.Date, &n.CreatedAt, &n.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan daily note: %w", err)
		}
		notes = append(notes, n)
	}
	return notes, rows.Err()
}

// --- Entries ---

// CreateEntry inserts a new entry, generating a UUID for its ID.
func (s *Store) CreateEntry(ctx context.Context, e *Entry) error {
	now := time.Now().UTC()
	e.ID = newUUID()
	e.CreatedAt = now
	e.UpdatedAt = now
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO entries (id, daily_note_id, type, content, raw_content, source, metadata, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		e.ID, e.DailyNoteID, e.Type, e.Content, e.RawContent, e.Source, e.Metadata, e.CreatedAt, e.UpdatedAt,
	)
	if err != nil {
		return fmt.Errorf("create entry: %w", err)
	}
	return nil
}

// GetEntry retrieves an entry by its ID.
func (s *Store) GetEntry(ctx context.Context, id string) (*Entry, error) {
	var e Entry
	err := s.db.QueryRowContext(ctx,
		`SELECT id, daily_note_id, type, content, raw_content, source, metadata, created_at, updated_at
		 FROM entries WHERE id = ?`, id,
	).Scan(&e.ID, &e.DailyNoteID, &e.Type, &e.Content, &e.RawContent, &e.Source, &e.Metadata, &e.CreatedAt, &e.UpdatedAt)
	if err != nil {
		return nil, fmt.Errorf("get entry: %w", err)
	}
	return &e, nil
}

// ListEntries returns all entries for a given daily note, ordered by creation time.
func (s *Store) ListEntries(ctx context.Context, dailyNoteID string) ([]Entry, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, daily_note_id, type, content, raw_content, source, metadata, created_at, updated_at
		 FROM entries WHERE daily_note_id = ? ORDER BY created_at`, dailyNoteID,
	)
	if err != nil {
		return nil, fmt.Errorf("list entries: %w", err)
	}
	defer rows.Close()

	var entries []Entry
	for rows.Next() {
		var e Entry
		if err := rows.Scan(&e.ID, &e.DailyNoteID, &e.Type, &e.Content, &e.RawContent, &e.Source, &e.Metadata, &e.CreatedAt, &e.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan entry: %w", err)
		}
		entries = append(entries, e)
	}
	return entries, rows.Err()
}

// UpdateEntry updates an existing entry's mutable fields.
func (s *Store) UpdateEntry(ctx context.Context, e *Entry) error {
	e.UpdatedAt = time.Now().UTC()
	res, err := s.db.ExecContext(ctx,
		`UPDATE entries SET type = ?, content = ?, raw_content = ?, source = ?, metadata = ?, updated_at = ?
		 WHERE id = ?`,
		e.Type, e.Content, e.RawContent, e.Source, e.Metadata, e.UpdatedAt, e.ID,
	)
	if err != nil {
		return fmt.Errorf("update entry: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("update entry: not found")
	}
	return nil
}

// DeleteEntry removes an entry and its attachments (via CASCADE).
func (s *Store) DeleteEntry(ctx context.Context, id string) error {
	// Remove attachment files before deleting DB rows.
	attachments, err := s.ListAttachments(ctx, id)
	if err != nil {
		return fmt.Errorf("delete entry: list attachments: %w", err)
	}
	for _, a := range attachments {
		_ = os.Remove(a.StoragePath)
	}

	res, err := s.db.ExecContext(ctx, `DELETE FROM entries WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("delete entry: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("delete entry: not found")
	}
	return nil
}

// --- Attachments ---

// CreateAttachment stores the file data on disk and inserts a metadata row.
func (s *Store) CreateAttachment(ctx context.Context, a *Attachment, data []byte) error {
	a.ID = newUUID()
	a.CreatedAt = time.Now().UTC()
	a.Size = int64(len(data))

	dir := filepath.Join(s.dataDir, "attachments")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create attachment dir: %w", err)
	}
	a.StoragePath = filepath.Join(dir, a.ID+"_"+a.Filename)
	if err := os.WriteFile(a.StoragePath, data, 0o644); err != nil {
		return fmt.Errorf("write attachment file: %w", err)
	}

	_, err := s.db.ExecContext(ctx,
		`INSERT INTO attachments (id, entry_id, filename, content_type, size, storage_path, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`,
		a.ID, a.EntryID, a.Filename, a.ContentType, a.Size, a.StoragePath, a.CreatedAt,
	)
	if err != nil {
		os.Remove(a.StoragePath)
		return fmt.Errorf("create attachment: %w", err)
	}
	return nil
}

// GetAttachment retrieves attachment metadata by ID.
func (s *Store) GetAttachment(ctx context.Context, id string) (*Attachment, error) {
	var a Attachment
	err := s.db.QueryRowContext(ctx,
		`SELECT id, entry_id, filename, content_type, size, storage_path, created_at
		 FROM attachments WHERE id = ?`, id,
	).Scan(&a.ID, &a.EntryID, &a.Filename, &a.ContentType, &a.Size, &a.StoragePath, &a.CreatedAt)
	if err != nil {
		return nil, fmt.Errorf("get attachment: %w", err)
	}
	return &a, nil
}

// GetAttachmentData reads the attachment file from disk.
func (s *Store) GetAttachmentData(ctx context.Context, id string) ([]byte, error) {
	a, err := s.GetAttachment(ctx, id)
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(a.StoragePath)
	if err != nil {
		return nil, fmt.Errorf("read attachment file: %w", err)
	}
	return data, nil
}

// ListAttachments returns all attachments for a given entry.
func (s *Store) ListAttachments(ctx context.Context, entryID string) ([]Attachment, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, entry_id, filename, content_type, size, storage_path, created_at
		 FROM attachments WHERE entry_id = ? ORDER BY created_at`, entryID,
	)
	if err != nil {
		return nil, fmt.Errorf("list attachments: %w", err)
	}
	defer rows.Close()

	var attachments []Attachment
	for rows.Next() {
		var a Attachment
		if err := rows.Scan(&a.ID, &a.EntryID, &a.Filename, &a.ContentType, &a.Size, &a.StoragePath, &a.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan attachment: %w", err)
		}
		attachments = append(attachments, a)
	}
	return attachments, rows.Err()
}
