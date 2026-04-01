package storage

import (
	"context"
	"crypto/rand"
	"database/sql"
	_ "embed"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

//go:embed schema.sql
var schema string

// Store provides SQLite-backed storage for daily notes, entries, and attachments.
// It uses separate connection pools for reads and writes to avoid SQLITE_BUSY
// contention. The write pool is limited to a single connection so all writes
// are serialized in-process.
type Store struct {
	readDB  *sql.DB
	writeDB *sql.DB
	dataDir string
}

// DailyNote represents a single day's note container.
type DailyNote struct {
	ID        string
	Date      string // YYYY-MM-DD
	Content   string
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

// Conversation represents a chat conversation.
type Conversation struct {
	ID        string
	Title     string
	Messages  []Message
	CreatedAt time.Time
	UpdatedAt time.Time
}

// Message represents a single message in a conversation.
type Message struct {
	ID             string
	ConversationID string
	Role           string
	Content        string
	Sources        string // JSON array of source references
	CreatedAt      time.Time
}

// NewUUID generates a random UUID v4 string.
func NewUUID() string {
	return newUUID()
}

func newUUID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}

// dsn builds the connection string with WAL mode, busy_timeout, and foreign keys.
func dsn(dbPath string) string {
	return dbPath + "?_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)&_pragma=foreign_keys(1)"
}

// New opens (or creates) a SQLite database at dbPath, runs migrations,
// and returns a ready-to-use Store. dataDir is used for attachment file storage.
//
// Two connection pools are created: a single-connection write pool (serializes
// all writes) and a multi-connection read pool (allows concurrent reads).
// Both use WAL mode and a 5-second busy_timeout.
func New(dbPath string, dataDir string) (*Store, error) {
	connStr := dsn(dbPath)

	writeDB, err := sql.Open("sqlite", connStr)
	if err != nil {
		return nil, fmt.Errorf("open write database: %w", err)
	}
	writeDB.SetMaxOpenConns(1)

	readDB, err := sql.Open("sqlite", connStr)
	if err != nil {
		_ = writeDB.Close()
		return nil, fmt.Errorf("open read database: %w", err)
	}
	readDB.SetMaxOpenConns(4)

	// Run migrations on the write connection.
	if _, err := writeDB.Exec(schema); err != nil {
		_ = writeDB.Close()
		_ = readDB.Close()
		return nil, fmt.Errorf("run migrations: %w", err)
	}

	return &Store{readDB: readDB, writeDB: writeDB, dataDir: dataDir}, nil
}

// Close closes both the read and write database connections.
func (s *Store) Close() error {
	wErr := s.writeDB.Close()
	rErr := s.readDB.Close()
	if wErr != nil {
		return wErr
	}
	return rErr
}

// DB returns the read database connection for use by other packages
// that need read access to the database (e.g., the embeddings index search).
func (s *Store) DB() *sql.DB {
	return s.readDB
}

// WriteDB returns the write database connection for use by other packages
// that need write access to the database (e.g., the embeddings index).
func (s *Store) WriteDB() *sql.DB {
	return s.writeDB
}

// exec wraps a write operation with retry-on-busy.
func (s *Store) exec(ctx context.Context, query string, args ...any) (sql.Result, error) {
	var res sql.Result
	err := RetryOnBusy(ctx, func() error {
		var e error
		res, e = s.writeDB.ExecContext(ctx, query, args...)
		return e
	})
	return res, err
}

// queryRow wraps a single-row read.
func (s *Store) queryRow(ctx context.Context, query string, args ...any) *sql.Row {
	return s.readDB.QueryRowContext(ctx, query, args...)
}

// query wraps a multi-row read.
func (s *Store) query(ctx context.Context, query string, args ...any) (*sql.Rows, error) {
	return s.readDB.QueryContext(ctx, query, args...)
}

// dayDir returns the filesystem path for a daily note's folder.
// Date format is "YYYY-MM-DD", stored as notes/YYYY/MM/DD/.
func (s *Store) dayDir(date string) string {
	parts := strings.Split(date, "-")
	if len(parts) != 3 {
		return filepath.Join(s.dataDir, "notes", date)
	}
	return filepath.Join(s.dataDir, "notes", parts[0], parts[1], parts[2])
}

// AttachmentPath returns the absolute path to an attachment file within the
// day directory for the given date. It rejects filenames that could escape the
// directory (path separators, "..", null bytes).
func (s *Store) AttachmentPath(date, filename string) (string, error) {
	if strings.ContainsAny(filename, "/\\") || strings.Contains(filename, "..") || strings.ContainsRune(filename, 0) {
		return "", fmt.Errorf("invalid attachment filename")
	}
	clean := filepath.Base(filename)
	if clean == "." || clean == string(filepath.Separator) {
		return "", fmt.Errorf("invalid attachment filename")
	}
	return filepath.Join(s.dayDir(date), clean), nil
}

// notePath returns the filesystem path for a daily note's markdown file.
// Date format is "YYYY-MM-DD", stored as notes/YYYY/MM/DD/index.md.
func (s *Store) notePath(date string) string {
	return filepath.Join(s.dayDir(date), "index.md")
}

func (s *Store) readNoteContent(date string) string {
	data, err := os.ReadFile(s.notePath(date))
	if err != nil {
		return ""
	}
	return string(data)
}

func (s *Store) writeNoteContent(date, content string) error {
	path := s.notePath(date)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(content), 0o644)
}

// getNoteDate looks up the date for a daily note by its ID.
func (s *Store) getNoteDate(ctx context.Context, id string) (string, error) {
	var date string
	err := s.queryRow(ctx, `SELECT date FROM daily_notes WHERE id = ?`, id).Scan(&date)
	if err != nil {
		return "", fmt.Errorf("get note date: %w", err)
	}
	return date, nil
}

// --- Daily notes ---

// CreateDailyNote inserts a new daily note for the given date (YYYY-MM-DD).
func (s *Store) CreateDailyNote(ctx context.Context, date string) (*DailyNote, error) {
	now := time.Now().UTC()
	id := newUUID()
	_, err := s.exec(ctx,
		`INSERT INTO daily_notes (id, date, content, created_at, updated_at) VALUES (?, ?, '', ?, ?)`,
		id, date, now, now,
	)
	if err != nil {
		return nil, fmt.Errorf("create daily note: %w", err)
	}
	return &DailyNote{ID: id, Date: date, Content: "", CreatedAt: now, UpdatedAt: now}, nil
}

// GetDailyNote retrieves a daily note by its ID.
func (s *Store) GetDailyNote(ctx context.Context, id string) (*DailyNote, error) {
	var n DailyNote
	err := s.queryRow(ctx,
		`SELECT id, date, created_at, updated_at FROM daily_notes WHERE id = ?`, id,
	).Scan(&n.ID, &n.Date, &n.CreatedAt, &n.UpdatedAt)
	if err != nil {
		return nil, fmt.Errorf("get daily note: %w", err)
	}
	n.Content = s.readNoteContent(n.Date)
	return &n, nil
}

// GetDailyNoteByDate retrieves a daily note by its date string.
func (s *Store) GetDailyNoteByDate(ctx context.Context, date string) (*DailyNote, error) {
	var n DailyNote
	err := s.queryRow(ctx,
		`SELECT id, date, created_at, updated_at FROM daily_notes WHERE date = ?`, date,
	).Scan(&n.ID, &n.Date, &n.CreatedAt, &n.UpdatedAt)
	if err != nil {
		return nil, fmt.Errorf("get daily note by date: %w", err)
	}
	n.Content = s.readNoteContent(n.Date)
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
	rows, err := s.query(ctx,
		`SELECT id, date, created_at, updated_at FROM daily_notes ORDER BY date DESC LIMIT ? OFFSET ?`,
		limit, offset,
	)
	if err != nil {
		return nil, fmt.Errorf("list daily notes: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var notes []DailyNote
	for rows.Next() {
		var n DailyNote
		if err := rows.Scan(&n.ID, &n.Date, &n.CreatedAt, &n.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan daily note: %w", err)
		}
		n.Content = s.readNoteContent(n.Date)
		notes = append(notes, n)
	}
	return notes, rows.Err()
}

// UpdateDailyNoteContent replaces the content of a daily note.
func (s *Store) UpdateDailyNoteContent(ctx context.Context, id string, content string) error {
	date, err := s.getNoteDate(ctx, id)
	if err != nil {
		return err
	}
	if err := s.writeNoteContent(date, content); err != nil {
		return fmt.Errorf("write note file: %w", err)
	}
	_, err = s.exec(ctx,
		`UPDATE daily_notes SET updated_at = ? WHERE id = ?`,
		time.Now().UTC(), id,
	)
	return err
}

// AppendDailyNoteContent appends text to the content of a daily note.
func (s *Store) AppendDailyNoteContent(ctx context.Context, id string, text string) error {
	date, err := s.getNoteDate(ctx, id)
	if err != nil {
		return err
	}
	existing := s.readNoteContent(date)
	if err := s.writeNoteContent(date, existing+text); err != nil {
		return fmt.Errorf("write note file: %w", err)
	}
	_, err = s.exec(ctx,
		`UPDATE daily_notes SET updated_at = ? WHERE id = ?`,
		time.Now().UTC(), id,
	)
	return err
}

// defaultSections is the initial structure for new daily notes.
const defaultSections = "# Summary\n\n# Notes\n\n# Links\n"

// AppendToSection appends text under the given markdown heading (e.g. "# Notes" or "# Links").
// If the section doesn't exist, it creates it. Content is appended at the end of the section
// (before the next top-level heading or end of file).
func (s *Store) AppendToSection(ctx context.Context, id string, section string, text string) error {
	date, err := s.getNoteDate(ctx, id)
	if err != nil {
		return err
	}

	content := s.readNoteContent(date)

	// If the note is empty, initialize with the default structure.
	// If it has legacy content (no section headings), prepend it before the structure.
	if !strings.Contains(content, "# Notes") && !strings.Contains(content, "# Links") {
		if strings.TrimSpace(content) == "" {
			content = defaultSections
		} else {
			if !strings.HasSuffix(content, "\n") {
				content += "\n"
			}
			// Only add sections that don't already exist.
			var missing string
			if !strings.Contains(content, "# Summary") {
				missing += "# Summary\n\n"
			}
			if !strings.Contains(content, "# Notes") {
				missing += "# Notes\n\n"
			}
			if !strings.Contains(content, "# Links") {
				missing += "# Links\n"
			}
			if missing != "" {
				content += "\n" + missing
			}
		}
	}

	content = insertIntoSection(content, section, text)

	if err := s.writeNoteContent(date, content); err != nil {
		return fmt.Errorf("write note file: %w", err)
	}
	_, err = s.exec(ctx,
		`UPDATE daily_notes SET updated_at = ? WHERE id = ?`,
		time.Now().UTC(), id,
	)
	return err
}

// insertIntoSection inserts text at the end of the named section within content.
// If the section heading is not found, it appends the heading and text at the end.
func insertIntoSection(content, section, text string) string {
	// Find the section heading at the start of a line.
	sectionIdx := -1
	if strings.HasPrefix(content, section+"\n") {
		sectionIdx = 0
	}
	if sectionIdx < 0 {
		if idx := strings.Index(content, "\n"+section+"\n"); idx >= 0 {
			sectionIdx = idx + 1
		}
	}

	if sectionIdx < 0 {
		// Section not found — append it at the end.
		if content != "" && !strings.HasSuffix(content, "\n") {
			content += "\n"
		}
		return content + "\n" + section + "\n" + text
	}

	// Find the end of the heading line.
	afterHeading := sectionIdx + len(section) + 1

	// Find the next top-level heading (# ) after this section.
	nextSection := -1
	if idx := strings.Index(content[afterHeading:], "\n# "); idx >= 0 {
		nextSection = afterHeading + idx + 1 // position of '#'
	}

	if nextSection >= 0 {
		// Trim trailing blank lines between section body and next heading.
		bodyEnd := nextSection
		for bodyEnd > afterHeading && (content[bodyEnd-1] == '\n' || content[bodyEnd-1] == '\r') {
			bodyEnd--
		}
		// If there's actual content in the section, preserve one trailing newline.
		if bodyEnd > afterHeading && bodyEnd < nextSection {
			bodyEnd++
		}

		after := content[nextSection:]
		return content[:bodyEnd] + text + "\n" + after
	}

	// No next section — append at end.
	return content + text + "\n"
}

// SetSection replaces the body of a markdown section (e.g. "# Summary") with new text.
// If the section doesn't exist, it is inserted before "# Notes" (for "# Summary") or
// appended at the end.
func (s *Store) SetSection(ctx context.Context, id string, section string, text string) error {
	date, err := s.getNoteDate(ctx, id)
	if err != nil {
		return err
	}

	content := s.readNoteContent(date)
	content = replaceSection(content, section, text)

	if err := s.writeNoteContent(date, content); err != nil {
		return fmt.Errorf("write note file: %w", err)
	}
	_, err = s.exec(ctx,
		`UPDATE daily_notes SET updated_at = ? WHERE id = ?`,
		time.Now().UTC(), id,
	)
	return err
}

// replaceSection replaces the body of a section heading in content. If the section
// doesn't exist, it inserts it. For "# Summary", it inserts before "# Notes".
func replaceSection(content, section, text string) string {
	// Ensure text ends with newline.
	if text != "" && !strings.HasSuffix(text, "\n") {
		text += "\n"
	}

	// Find the section heading.
	sectionIdx := -1
	if strings.HasPrefix(content, section+"\n") {
		sectionIdx = 0
	}
	if sectionIdx < 0 {
		if idx := strings.Index(content, "\n"+section+"\n"); idx >= 0 {
			sectionIdx = idx + 1
		}
	}

	if sectionIdx >= 0 {
		// Found — replace the body between this heading and the next.
		afterHeading := sectionIdx + len(section) + 1

		nextSection := -1
		if idx := strings.Index(content[afterHeading:], "\n# "); idx >= 0 {
			nextSection = afterHeading + idx + 1
		}

		if nextSection >= 0 {
			return content[:afterHeading] + text + "\n" + content[nextSection:]
		}
		return content[:afterHeading] + text
	}

	// Section not found — insert it.
	body := section + "\n" + text + "\n"

	// For "# Summary", insert before "# Notes" so it appears at the top.
	if section == "# Summary" {
		notesIdx := -1
		if strings.HasPrefix(content, "# Notes\n") {
			notesIdx = 0
		} else if idx := strings.Index(content, "\n# Notes\n"); idx >= 0 {
			notesIdx = idx + 1
		}
		if notesIdx >= 0 {
			return content[:notesIdx] + body + content[notesIdx:]
		}
	}

	// Append at end.
	if content != "" && !strings.HasSuffix(content, "\n") {
		content += "\n"
	}
	return content + "\n" + body
}

// ListDatesWithContent returns dates that have non-empty content within the given range.
func (s *Store) ListDatesWithContent(ctx context.Context, startDate, endDate string) ([]string, error) {
	rows, err := s.query(ctx,
		`SELECT date FROM daily_notes WHERE date >= ? AND date <= ? ORDER BY date`,
		startDate, endDate,
	)
	if err != nil {
		return nil, fmt.Errorf("list dates with content: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var dates []string
	for rows.Next() {
		var d string
		if err := rows.Scan(&d); err != nil {
			return nil, fmt.Errorf("scan date: %w", err)
		}
		// Only include dates whose note file exists and is non-empty.
		if content := s.readNoteContent(d); content != "" {
			dates = append(dates, d)
		}
	}
	return dates, rows.Err()
}

// DailyNoteSummary holds a date and its extracted summary text.
type DailyNoteSummary struct {
	Date    string
	Summary string
}

// ListSummariesInRange returns daily note summaries for dates in [startDate, endDate].
// It extracts the "# Summary" section from each note's content.
func (s *Store) ListSummariesInRange(ctx context.Context, startDate, endDate string) ([]DailyNoteSummary, error) {
	rows, err := s.query(ctx,
		`SELECT date FROM daily_notes WHERE date >= ? AND date <= ? ORDER BY date`,
		startDate, endDate,
	)
	if err != nil {
		return nil, fmt.Errorf("list summaries in range: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var summaries []DailyNoteSummary
	for rows.Next() {
		var d string
		if err := rows.Scan(&d); err != nil {
			return nil, fmt.Errorf("scan date: %w", err)
		}
		content := s.readNoteContent(d)
		if content == "" {
			continue
		}
		summary := extractSection(content, "# Summary")
		summaries = append(summaries, DailyNoteSummary{Date: d, Summary: summary})
	}
	return summaries, rows.Err()
}

// extractSection returns the body text under a markdown heading, or empty string
// if the heading is not found. The body extends until the next "# " heading or EOF.
func extractSection(content, heading string) string {
	sectionIdx := -1
	if strings.HasPrefix(content, heading+"\n") {
		sectionIdx = 0
	}
	if sectionIdx < 0 {
		if idx := strings.Index(content, "\n"+heading+"\n"); idx >= 0 {
			sectionIdx = idx + 1
		}
	}
	if sectionIdx < 0 {
		return ""
	}

	afterHeading := sectionIdx + len(heading) + 1
	nextSection := -1
	if idx := strings.Index(content[afterHeading:], "\n# "); idx >= 0 {
		nextSection = afterHeading + idx
	}

	var body string
	if nextSection >= 0 {
		body = content[afterHeading:nextSection]
	} else {
		body = content[afterHeading:]
	}
	return strings.TrimSpace(body)
}

// --- Entries ---

// CreateEntry inserts a new entry, generating a UUID for its ID.
func (s *Store) CreateEntry(ctx context.Context, e *Entry) error {
	now := time.Now().UTC()
	e.ID = newUUID()
	e.CreatedAt = now
	e.UpdatedAt = now
	_, err := s.exec(ctx,
		`INSERT INTO entries (id, daily_note_id, type, content, raw_content, source, metadata, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		e.ID, e.DailyNoteID, e.Type, e.Content, e.RawContent, e.Source, e.Metadata, e.CreatedAt, e.UpdatedAt,
	)
	if err != nil {
		return fmt.Errorf("create entry: %w", err)
	}
	return nil
}

// GetEntryByClientRequestID retrieves a previously created entry for an
// idempotent CreateEntry client request ID.
func (s *Store) GetEntryByClientRequestID(ctx context.Context, requestID string) (*Entry, error) {
	var e Entry
	err := s.queryRow(ctx,
		`SELECT e.id, e.daily_note_id, e.type, e.content, e.raw_content, e.source, e.metadata, e.created_at, e.updated_at
		 FROM create_entry_requests cer
		 JOIN entries e ON e.id = cer.entry_id
		 WHERE cer.request_id = ?`,
		requestID,
	).Scan(&e.ID, &e.DailyNoteID, &e.Type, &e.Content, &e.RawContent, &e.Source, &e.Metadata, &e.CreatedAt, &e.UpdatedAt)
	if err != nil {
		return nil, fmt.Errorf("get entry by client request ID: %w", err)
	}
	return &e, nil
}

// RecordCreateEntryRequest stores an idempotency mapping for a client request.
func (s *Store) RecordCreateEntryRequest(ctx context.Context, requestID, entryID string) error {
	_, err := s.exec(ctx,
		`INSERT OR IGNORE INTO create_entry_requests (request_id, entry_id, created_at)
		 VALUES (?, ?, ?)`,
		requestID, entryID, time.Now().UTC(),
	)
	if err != nil {
		return fmt.Errorf("record create entry request: %w", err)
	}
	return nil
}

// GetEntry retrieves an entry by its ID.
func (s *Store) GetEntry(ctx context.Context, id string) (*Entry, error) {
	var e Entry
	err := s.queryRow(ctx,
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
	rows, err := s.query(ctx,
		`SELECT id, daily_note_id, type, content, raw_content, source, metadata, created_at, updated_at
		 FROM entries WHERE daily_note_id = ? ORDER BY created_at`, dailyNoteID,
	)
	if err != nil {
		return nil, fmt.Errorf("list entries: %w", err)
	}
	defer func() { _ = rows.Close() }()

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
	res, err := s.exec(ctx,
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

// UpdateEntryContent updates only the content field of an entry.
func (s *Store) UpdateEntryContent(ctx context.Context, id, content string) error {
	_, err := s.exec(ctx, `UPDATE entries SET content = ?, raw_content = ?, updated_at = ? WHERE id = ?`, content, content, time.Now().UTC(), id)
	if err != nil {
		return fmt.Errorf("update entry content: %w", err)
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

	res, err := s.exec(ctx, `DELETE FROM entries WHERE id = ?`, id)
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
// The date parameter (YYYY-MM-DD) determines the day folder where the file is stored.
func (s *Store) CreateAttachment(ctx context.Context, a *Attachment, data []byte, date string) error {
	a.ID = newUUID()
	a.CreatedAt = time.Now().UTC()
	a.Size = int64(len(data))

	dir := s.dayDir(date)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create day dir: %w", err)
	}
	ext := filepath.Ext(a.Filename)
	base := strings.TrimSuffix(a.Filename, ext)
	a.StoragePath = filepath.Join(dir, base+"-"+a.ID[:8]+ext)
	if err := os.WriteFile(a.StoragePath, data, 0o644); err != nil {
		return fmt.Errorf("write attachment file: %w", err)
	}

	_, err := s.exec(ctx,
		`INSERT INTO attachments (id, entry_id, filename, content_type, size, storage_path, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`,
		a.ID, a.EntryID, a.Filename, a.ContentType, a.Size, a.StoragePath, a.CreatedAt,
	)
	if err != nil {
		_ = os.Remove(a.StoragePath)
		return fmt.Errorf("create attachment: %w", err)
	}
	return nil
}

// GetAttachment retrieves attachment metadata by ID.
func (s *Store) GetAttachment(ctx context.Context, id string) (*Attachment, error) {
	var a Attachment
	err := s.queryRow(ctx,
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
	rows, err := s.query(ctx,
		`SELECT id, entry_id, filename, content_type, size, storage_path, created_at
		 FROM attachments WHERE entry_id = ? ORDER BY created_at`, entryID,
	)
	if err != nil {
		return nil, fmt.Errorf("list attachments: %w", err)
	}
	defer func() { _ = rows.Close() }()

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

// --- Conversations ---

// CreateConversation inserts a new conversation.
func (s *Store) CreateConversation(ctx context.Context, title string) (*Conversation, error) {
	now := time.Now().UTC()
	id := newUUID()
	_, err := s.exec(ctx,
		`INSERT INTO conversations (id, title, created_at, updated_at) VALUES (?, ?, ?, ?)`,
		id, title, now, now,
	)
	if err != nil {
		return nil, fmt.Errorf("create conversation: %w", err)
	}
	return &Conversation{ID: id, Title: title, CreatedAt: now, UpdatedAt: now}, nil
}

// GetConversation retrieves a conversation by ID, including its messages.
func (s *Store) GetConversation(ctx context.Context, id string) (*Conversation, error) {
	var c Conversation
	err := s.queryRow(ctx,
		`SELECT id, title, created_at, updated_at FROM conversations WHERE id = ?`, id,
	).Scan(&c.ID, &c.Title, &c.CreatedAt, &c.UpdatedAt)
	if err != nil {
		return nil, fmt.Errorf("get conversation: %w", err)
	}

	rows, err := s.query(ctx,
		`SELECT id, conversation_id, role, content, sources, created_at
		 FROM messages WHERE conversation_id = ? ORDER BY created_at`, id,
	)
	if err != nil {
		return nil, fmt.Errorf("get conversation messages: %w", err)
	}
	defer func() { _ = rows.Close() }()

	for rows.Next() {
		var m Message
		if err := rows.Scan(&m.ID, &m.ConversationID, &m.Role, &m.Content, &m.Sources, &m.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan message: %w", err)
		}
		c.Messages = append(c.Messages, m)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate messages: %w", err)
	}
	return &c, nil
}

// ListConversations returns conversations ordered by updated_at descending, without messages.
func (s *Store) ListConversations(ctx context.Context, limit, offset int) ([]Conversation, error) {
	if limit <= 0 {
		limit = 50
	}
	rows, err := s.query(ctx,
		`SELECT id, title, created_at, updated_at FROM conversations ORDER BY updated_at DESC LIMIT ? OFFSET ?`,
		limit, offset,
	)
	if err != nil {
		return nil, fmt.Errorf("list conversations: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var convos []Conversation
	for rows.Next() {
		var c Conversation
		if err := rows.Scan(&c.ID, &c.Title, &c.CreatedAt, &c.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan conversation: %w", err)
		}
		convos = append(convos, c)
	}
	return convos, rows.Err()
}

// AddMessage inserts a message into a conversation and updates the conversation's updated_at.
func (s *Store) AddMessage(ctx context.Context, conversationID, role, content, sourcesJSON string) (*Message, error) {
	now := time.Now().UTC()
	id := newUUID()

	if sourcesJSON == "" {
		sourcesJSON = "[]"
	}
	// Validate JSON
	if !json.Valid([]byte(sourcesJSON)) {
		sourcesJSON = "[]"
	}

	_, err := s.exec(ctx,
		`INSERT INTO messages (id, conversation_id, role, content, sources, created_at) VALUES (?, ?, ?, ?, ?, ?)`,
		id, conversationID, role, content, sourcesJSON, now,
	)
	if err != nil {
		return nil, fmt.Errorf("add message: %w", err)
	}

	_, err = s.exec(ctx,
		`UPDATE conversations SET updated_at = ? WHERE id = ?`, now, conversationID,
	)
	if err != nil {
		return nil, fmt.Errorf("update conversation timestamp: %w", err)
	}

	return &Message{
		ID:             id,
		ConversationID: conversationID,
		Role:           role,
		Content:        content,
		Sources:        sourcesJSON,
		CreatedAt:      now,
	}, nil
}

// UpdateConversationTitle updates the title of a conversation.
func (s *Store) UpdateConversationTitle(ctx context.Context, id, title string) error {
	res, err := s.exec(ctx,
		`UPDATE conversations SET title = ?, updated_at = ? WHERE id = ?`,
		title, time.Now().UTC(), id,
	)
	if err != nil {
		return fmt.Errorf("update conversation title: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("update conversation title: not found")
	}
	return nil
}

// ImportJob represents a background import job.
type ImportJob struct {
	ID         string
	Status     string // pending, processing, done, error
	Filename   string
	FileType   string // viwoods, obsidian
	FilePath   string
	Source     string // upload, watcher
	Progress   int
	TotalSteps int
	Result     string // JSON
	CreatedAt  time.Time
	UpdatedAt  time.Time
}

// CreateImportJob inserts a new import job into the database.
func (s *Store) CreateImportJob(ctx context.Context, job *ImportJob) error {
	if job.ID == "" {
		job.ID = newUUID()
	}
	now := time.Now().UTC()
	job.CreatedAt = now
	job.UpdatedAt = now
	if job.Result == "" {
		job.Result = "{}"
	}
	_, err := s.exec(ctx,
		`INSERT INTO import_jobs (id, status, filename, file_type, file_path, source, progress, total_steps, result, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		job.ID, job.Status, job.Filename, job.FileType, job.FilePath, job.Source,
		job.Progress, job.TotalSteps, job.Result, job.CreatedAt, job.UpdatedAt,
	)
	if err != nil {
		return fmt.Errorf("create import job: %w", err)
	}
	return nil
}

// GetImportJob retrieves a single import job by ID.
func (s *Store) GetImportJob(ctx context.Context, id string) (*ImportJob, error) {
	row := s.queryRow(ctx,
		`SELECT id, status, filename, file_type, file_path, source, progress, total_steps, result, created_at, updated_at
		 FROM import_jobs WHERE id = ?`, id)
	return scanImportJob(row)
}

// ListImportJobs returns import jobs filtered by IDs and/or active status.
// If ids is non-empty, only those jobs are returned.
// If activeOnly is true, only pending/processing jobs are returned.
// Otherwise returns the most recent 50 jobs.
func (s *Store) ListImportJobs(ctx context.Context, ids []string, activeOnly bool) ([]*ImportJob, error) {
	var query strings.Builder
	var args []any

	query.WriteString(`SELECT id, status, filename, file_type, file_path, source, progress, total_steps, result, created_at, updated_at FROM import_jobs`)

	var conditions []string
	if len(ids) > 0 {
		placeholders := make([]string, len(ids))
		for i, id := range ids {
			placeholders[i] = "?"
			args = append(args, id)
		}
		conditions = append(conditions, "id IN ("+strings.Join(placeholders, ",")+")")
	}
	if activeOnly {
		conditions = append(conditions, "status IN ('pending', 'processing')")
	}
	if len(conditions) > 0 {
		query.WriteString(" WHERE ")
		query.WriteString(strings.Join(conditions, " AND "))
	}
	query.WriteString(" ORDER BY created_at DESC")
	if len(ids) == 0 && !activeOnly {
		query.WriteString(" LIMIT 50")
	}

	rows, err := s.query(ctx, query.String(), args...)
	if err != nil {
		return nil, fmt.Errorf("list import jobs: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var jobs []*ImportJob
	for rows.Next() {
		var j ImportJob
		if err := rows.Scan(&j.ID, &j.Status, &j.Filename, &j.FileType, &j.FilePath, &j.Source,
			&j.Progress, &j.TotalSteps, &j.Result, &j.CreatedAt, &j.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan import job: %w", err)
		}
		jobs = append(jobs, &j)
	}
	return jobs, rows.Err()
}

// UpdateImportJobStatus sets the status of an import job.
func (s *Store) UpdateImportJobStatus(ctx context.Context, id, status string) error {
	res, err := s.exec(ctx,
		`UPDATE import_jobs SET status = ?, updated_at = ? WHERE id = ?`,
		status, time.Now().UTC(), id,
	)
	if err != nil {
		return fmt.Errorf("update import job status: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("update import job status: not found")
	}
	return nil
}

// UpdateImportJobProgress updates the progress counters of an import job.
func (s *Store) UpdateImportJobProgress(ctx context.Context, id string, progress, totalSteps int) error {
	res, err := s.exec(ctx,
		`UPDATE import_jobs SET progress = ?, total_steps = ?, updated_at = ? WHERE id = ?`,
		progress, totalSteps, time.Now().UTC(), id,
	)
	if err != nil {
		return fmt.Errorf("update import job progress: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("update import job progress: not found")
	}
	return nil
}

// UpdateImportJobResult sets the result JSON of an import job.
func (s *Store) UpdateImportJobResult(ctx context.Context, id, result string) error {
	res, err := s.exec(ctx,
		`UPDATE import_jobs SET result = ?, updated_at = ? WHERE id = ?`,
		result, time.Now().UTC(), id,
	)
	if err != nil {
		return fmt.Errorf("update import job result: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("update import job result: not found")
	}
	return nil
}

// DeleteImportJob removes an import job from the database and cleans up its staging file.
func (s *Store) DeleteImportJob(ctx context.Context, id string) error {
	// Look up the file path so we can clean up the staging file.
	var filePath string
	err := s.queryRow(ctx, `SELECT file_path FROM import_jobs WHERE id = ?`, id).Scan(&filePath)
	if err != nil {
		if err == sql.ErrNoRows {
			return fmt.Errorf("delete import job: not found")
		}
		return fmt.Errorf("delete import job: %w", err)
	}

	res, err := s.exec(ctx, `DELETE FROM import_jobs WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("delete import job: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("delete import job: not found")
	}

	// Clean up the staging file and its parent directory.
	if filePath != "" {
		_ = os.Remove(filePath)
		dir := filepath.Dir(filePath)
		_ = os.Remove(dir) // only succeeds if empty
	}

	return nil
}

// ClaimNextPendingJob atomically transitions the oldest pending job to processing and returns it.
// Returns nil, nil if no pending jobs exist.
func (s *Store) ClaimNextPendingJob(ctx context.Context) (*ImportJob, error) {
	var tx *sql.Tx
	if err := RetryOnBusy(ctx, func() error {
		var e error
		tx, e = s.writeDB.BeginTx(ctx, nil)
		return e
	}); err != nil {
		return nil, fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck

	// Find the oldest pending job.
	row := tx.QueryRowContext(ctx,
		`SELECT id FROM import_jobs WHERE status = 'pending' ORDER BY created_at LIMIT 1`)
	var id string
	if err := row.Scan(&id); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("find pending job: %w", err)
	}

	// Transition to processing.
	now := time.Now().UTC()
	if _, err := tx.ExecContext(ctx,
		`UPDATE import_jobs SET status = 'processing', updated_at = ? WHERE id = ?`, now, id); err != nil {
		return nil, fmt.Errorf("claim job: %w", err)
	}

	// Read back the full row.
	jobRow := tx.QueryRowContext(ctx,
		`SELECT id, status, filename, file_type, file_path, source, progress, total_steps, result, created_at, updated_at
		 FROM import_jobs WHERE id = ?`, id)
	job, err := scanImportJob(jobRow)
	if err != nil {
		return nil, err
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit claim: %w", err)
	}
	return job, nil
}

// ResetProcessingJobs transitions any processing jobs back to pending.
// Call on startup to recover from crashes.
func (s *Store) ResetProcessingJobs(ctx context.Context) error {
	_, err := s.exec(ctx,
		`UPDATE import_jobs SET status = 'pending', updated_at = ? WHERE status = 'processing'`,
		time.Now().UTC(),
	)
	if err != nil {
		return fmt.Errorf("reset processing jobs: %w", err)
	}
	return nil
}

// scanImportJob scans an ImportJob from a *sql.Row.
func scanImportJob(row *sql.Row) (*ImportJob, error) {
	var j ImportJob
	if err := row.Scan(&j.ID, &j.Status, &j.Filename, &j.FileType, &j.FilePath, &j.Source,
		&j.Progress, &j.TotalSteps, &j.Result, &j.CreatedAt, &j.UpdatedAt); err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("import job not found")
		}
		return nil, fmt.Errorf("scan import job: %w", err)
	}
	return &j, nil
}

// WatchedFile tracks a file that has been processed by the watcher.
type WatchedFile struct {
	Path        string
	Hash        string
	JobID       string
	ProcessedAt time.Time
}

// GetWatchedFile retrieves a watched file record by path.
func (s *Store) GetWatchedFile(ctx context.Context, path string) (*WatchedFile, error) {
	var wf WatchedFile
	err := s.queryRow(ctx,
		`SELECT path, hash, job_id, processed_at FROM watched_files WHERE path = ?`, path,
	).Scan(&wf.Path, &wf.Hash, &wf.JobID, &wf.ProcessedAt)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("get watched file: %w", err)
	}
	return &wf, nil
}

// UpsertWatchedFile inserts or updates a watched file record.
func (s *Store) UpsertWatchedFile(ctx context.Context, wf *WatchedFile) error {
	if wf.ProcessedAt.IsZero() {
		wf.ProcessedAt = time.Now().UTC()
	}
	_, err := s.exec(ctx,
		`INSERT INTO watched_files (path, hash, job_id, processed_at)
		 VALUES (?, ?, ?, ?)
		 ON CONFLICT(path) DO UPDATE SET hash = excluded.hash, job_id = excluded.job_id, processed_at = excluded.processed_at`,
		wf.Path, wf.Hash, wf.JobID, wf.ProcessedAt,
	)
	if err != nil {
		return fmt.Errorf("upsert watched file: %w", err)
	}
	return nil
}

// --- Telegram authorized chats ---

// IsTelegramChatAuthorized checks if a chat ID is in the authorized list.
func (s *Store) IsTelegramChatAuthorized(ctx context.Context, chatID int64) (bool, error) {
	var count int
	err := s.queryRow(ctx,
		`SELECT COUNT(*) FROM telegram_authorized_chats WHERE chat_id = ?`, chatID,
	).Scan(&count)
	if err != nil {
		return false, fmt.Errorf("check telegram auth: %w", err)
	}
	return count > 0, nil
}

// AuthorizeTelegramChat adds a chat ID to the authorized list.
func (s *Store) AuthorizeTelegramChat(ctx context.Context, chatID int64) error {
	_, err := s.exec(ctx,
		`INSERT INTO telegram_authorized_chats (chat_id) VALUES (?) ON CONFLICT(chat_id) DO NOTHING`,
		chatID,
	)
	if err != nil {
		return fmt.Errorf("authorize telegram chat: %w", err)
	}
	return nil
}

// RevokeTelegramChat removes a chat ID from the authorized list.
func (s *Store) RevokeTelegramChat(ctx context.Context, chatID int64) error {
	_, err := s.exec(ctx,
		`DELETE FROM telegram_authorized_chats WHERE chat_id = ?`, chatID,
	)
	if err != nil {
		return fmt.Errorf("revoke telegram chat: %w", err)
	}
	return nil
}

// ListTelegramAuthorizedChats returns all authorized chat IDs.
func (s *Store) ListTelegramAuthorizedChats(ctx context.Context) ([]int64, error) {
	rows, err := s.query(ctx, `SELECT chat_id FROM telegram_authorized_chats`)
	if err != nil {
		return nil, fmt.Errorf("list telegram authorized chats: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var ids []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("scan telegram chat id: %w", err)
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

// --- Granola sync state ---

// GranolaSyncState tracks which Granola notes have been imported and their version.
type GranolaSyncState struct {
	NoteID    string
	EntryID   string
	UpdatedAt string // Granola's updated_at timestamp
	SyncedAt  time.Time
}

// GetGranolaSyncState retrieves the sync state for a Granola note.
func (s *Store) GetGranolaSyncState(ctx context.Context, noteID string) (*GranolaSyncState, error) {
	var gs GranolaSyncState
	err := s.queryRow(ctx,
		`SELECT note_id, entry_id, updated_at, synced_at FROM granola_sync_state WHERE note_id = ?`, noteID,
	).Scan(&gs.NoteID, &gs.EntryID, &gs.UpdatedAt, &gs.SyncedAt)
	if err != nil {
		return nil, fmt.Errorf("get granola sync state: %w", err)
	}
	return &gs, nil
}

// UpsertGranolaSyncState inserts or updates the sync state for a Granola note.
func (s *Store) UpsertGranolaSyncState(ctx context.Context, gs *GranolaSyncState) error {
	if gs.SyncedAt.IsZero() {
		gs.SyncedAt = time.Now().UTC()
	}
	_, err := s.exec(ctx,
		`INSERT INTO granola_sync_state (note_id, entry_id, updated_at, synced_at)
		 VALUES (?, ?, ?, ?)
		 ON CONFLICT(note_id) DO UPDATE SET entry_id = excluded.entry_id, updated_at = excluded.updated_at, synced_at = excluded.synced_at`,
		gs.NoteID, gs.EntryID, gs.UpdatedAt, gs.SyncedAt,
	)
	if err != nil {
		return fmt.Errorf("upsert granola sync state: %w", err)
	}
	return nil
}

// --- User Preferences ---

// GetPreference returns the value for a preference key, or defaultVal if not set.
func (s *Store) GetPreference(ctx context.Context, key string, defaultVal string) (string, error) {
	var val string
	err := s.queryRow(ctx, `SELECT value FROM user_preferences WHERE key = ?`, key).Scan(&val)
	if err == sql.ErrNoRows {
		return defaultVal, nil
	}
	if err != nil {
		return "", fmt.Errorf("get preference %s: %w", key, err)
	}
	return val, nil
}

// SetPreference sets a preference key to the given value.
func (s *Store) SetPreference(ctx context.Context, key, value string) error {
	_, err := s.exec(ctx,
		`INSERT INTO user_preferences (key, value, updated_at) VALUES (?, ?, ?)
		 ON CONFLICT(key) DO UPDATE SET value = excluded.value, updated_at = excluded.updated_at`,
		key, value, time.Now().UTC(),
	)
	if err != nil {
		return fmt.Errorf("set preference %s: %w", key, err)
	}
	return nil
}

// GetAllPreferences returns all stored preferences as a map.
func (s *Store) GetAllPreferences(ctx context.Context) (map[string]string, error) {
	rows, err := s.query(ctx, `SELECT key, value FROM user_preferences`)
	if err != nil {
		return nil, fmt.Errorf("list preferences: %w", err)
	}
	defer func() { _ = rows.Close() }()

	prefs := make(map[string]string)
	for rows.Next() {
		var k, v string
		if err := rows.Scan(&k, &v); err != nil {
			return nil, fmt.Errorf("scan preference: %w", err)
		}
		prefs[k] = v
	}
	return prefs, rows.Err()
}


