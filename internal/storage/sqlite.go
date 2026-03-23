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
type Store struct {
	db      *sql.DB
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

// New opens (or creates) a SQLite database at dbPath, runs migrations,
// and returns a ready-to-use Store. dataDir is used for attachment file storage.
func New(dbPath string, dataDir string) (*Store, error) {
	db, err := sql.Open("sqlite", dbPath+"?_pragma=foreign_keys(1)")
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}
	if _, err := db.Exec(schema); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("run migrations: %w", err)
	}
	return &Store{db: db, dataDir: dataDir}, nil
}

// Close closes the underlying database connection.
func (s *Store) Close() error {
	return s.db.Close()
}

// DB returns the underlying database connection for use by other packages
// that need to share the same database (e.g., the embeddings index).
func (s *Store) DB() *sql.DB {
	return s.db
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
	err := s.db.QueryRowContext(ctx, `SELECT date FROM daily_notes WHERE id = ?`, id).Scan(&date)
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
	_, err := s.db.ExecContext(ctx,
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
	err := s.db.QueryRowContext(ctx,
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
	err := s.db.QueryRowContext(ctx,
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
	rows, err := s.db.QueryContext(ctx,
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
	_, err = s.db.ExecContext(ctx,
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
	_, err = s.db.ExecContext(ctx,
		`UPDATE daily_notes SET updated_at = ? WHERE id = ?`,
		time.Now().UTC(), id,
	)
	return err
}

// ListDatesWithContent returns dates that have non-empty content within the given range.
func (s *Store) ListDatesWithContent(ctx context.Context, startDate, endDate string) ([]string, error) {
	rows, err := s.db.QueryContext(ctx,
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

// UpdateEntryContent updates only the content field of an entry.
func (s *Store) UpdateEntryContent(ctx context.Context, id, content string) error {
	_, err := s.db.ExecContext(ctx, `UPDATE entries SET content = ?, raw_content = ?, updated_at = ? WHERE id = ?`, content, content, time.Now().UTC(), id)
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

	_, err := s.db.ExecContext(ctx,
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
	_, err := s.db.ExecContext(ctx,
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
	err := s.db.QueryRowContext(ctx,
		`SELECT id, title, created_at, updated_at FROM conversations WHERE id = ?`, id,
	).Scan(&c.ID, &c.Title, &c.CreatedAt, &c.UpdatedAt)
	if err != nil {
		return nil, fmt.Errorf("get conversation: %w", err)
	}

	rows, err := s.db.QueryContext(ctx,
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
	rows, err := s.db.QueryContext(ctx,
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

	_, err := s.db.ExecContext(ctx,
		`INSERT INTO messages (id, conversation_id, role, content, sources, created_at) VALUES (?, ?, ?, ?, ?, ?)`,
		id, conversationID, role, content, sourcesJSON, now,
	)
	if err != nil {
		return nil, fmt.Errorf("add message: %w", err)
	}

	_, err = s.db.ExecContext(ctx,
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
	res, err := s.db.ExecContext(ctx,
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
	_, err := s.db.ExecContext(ctx,
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
	row := s.db.QueryRowContext(ctx,
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

	rows, err := s.db.QueryContext(ctx, query.String(), args...)
	if err != nil {
		return nil, fmt.Errorf("list import jobs: %w", err)
	}
	defer rows.Close()

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
	res, err := s.db.ExecContext(ctx,
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
	res, err := s.db.ExecContext(ctx,
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
	res, err := s.db.ExecContext(ctx,
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
	err := s.db.QueryRowContext(ctx, `SELECT file_path FROM import_jobs WHERE id = ?`, id).Scan(&filePath)
	if err != nil {
		if err == sql.ErrNoRows {
			return fmt.Errorf("delete import job: not found")
		}
		return fmt.Errorf("delete import job: %w", err)
	}

	res, err := s.db.ExecContext(ctx, `DELETE FROM import_jobs WHERE id = ?`, id)
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
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
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
	_, err = tx.ExecContext(ctx,
		`UPDATE import_jobs SET status = 'processing', updated_at = ? WHERE id = ?`, now, id)
	if err != nil {
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
	_, err := s.db.ExecContext(ctx,
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
	err := s.db.QueryRowContext(ctx,
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
	_, err := s.db.ExecContext(ctx,
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
	err := s.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM telegram_authorized_chats WHERE chat_id = ?`, chatID,
	).Scan(&count)
	if err != nil {
		return false, fmt.Errorf("check telegram auth: %w", err)
	}
	return count > 0, nil
}

// AuthorizeTelegramChat adds a chat ID to the authorized list.
func (s *Store) AuthorizeTelegramChat(ctx context.Context, chatID int64) error {
	_, err := s.db.ExecContext(ctx,
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
	_, err := s.db.ExecContext(ctx,
		`DELETE FROM telegram_authorized_chats WHERE chat_id = ?`, chatID,
	)
	if err != nil {
		return fmt.Errorf("revoke telegram chat: %w", err)
	}
	return nil
}

// ListTelegramAuthorizedChats returns all authorized chat IDs.
func (s *Store) ListTelegramAuthorizedChats(ctx context.Context) ([]int64, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT chat_id FROM telegram_authorized_chats`)
	if err != nil {
		return nil, fmt.Errorf("list telegram authorized chats: %w", err)
	}
	defer rows.Close()

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

