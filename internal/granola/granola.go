// Package granola provides a periodic sync that imports meeting notes from
// the Granola API into Blackwood daily notes.
package granola

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/csweichel/blackwood/internal/index"
	"github.com/csweichel/blackwood/internal/storage"
)

const (
	baseURL  = "https://public-api.granola.ai/v1"
	pageSize = 30
)

// --- Granola API types ---

type listNotesResponse struct {
	Notes   []Note `json:"notes"`
	HasMore bool   `json:"hasMore"`
	Cursor  string `json:"cursor"`
}

type Note struct {
	ID        string  `json:"id"`
	Title     string  `json:"title"`
	Owner     Person  `json:"owner"`
	CreatedAt string  `json:"created_at"`
	UpdatedAt string  `json:"updated_at"`
}

type noteDetail struct {
	ID              string           `json:"id"`
	Title           string           `json:"title"`
	Owner           Person           `json:"owner"`
	CreatedAt       string           `json:"created_at"`
	UpdatedAt       string           `json:"updated_at"`
	CalendarEvent   *calendarEvent   `json:"calendar_event"`
	Attendees       []Person         `json:"attendees"`
	SummaryMarkdown string           `json:"summary_markdown"`
	SummaryText     string           `json:"summary_text"`
	Transcript      []transcriptLine `json:"transcript"`
}

type Person struct {
	Name  string `json:"name"`
	Email string `json:"email"`
}

type calendarEvent struct {
	EventTitle         string `json:"event_title"`
	Organiser          string `json:"organiser"`
	ScheduledStartTime string `json:"scheduled_start_time"`
	ScheduledEndTime   string `json:"scheduled_end_time"`
}

type transcriptLine struct {
	Speaker   speaker `json:"speaker"`
	Text      string  `json:"text"`
	StartTime string  `json:"start_time"`
	EndTime   string  `json:"end_time"`
}

type speaker struct {
	Source string `json:"source"` // "microphone" or "speaker"
}

// --- API client ---

type client struct {
	apiKey     string
	baseURL    string
	httpClient *http.Client
}

func newClient(apiKey string) *client {
	return &client{
		apiKey:     apiKey,
		baseURL:    baseURL,
		httpClient: &http.Client{Timeout: 30 * time.Second},
	}
}

func (c *client) do(ctx context.Context, method, path string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+c.apiKey)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("granola API %s %s: HTTP %d: %s", method, path, resp.StatusCode, string(body))
	}
	return body, nil
}

// listNotes returns notes updated after the given time, paginating through all results.
func (c *client) listNotes(ctx context.Context, updatedAfter time.Time) ([]Note, error) {
	var all []Note
	cursor := ""

	for {
		path := fmt.Sprintf("/notes?page_size=%d&updated_after=%s",
			pageSize, updatedAfter.Format("2006-01-02"))
		if cursor != "" {
			path += "&cursor=" + cursor
		}

		data, err := c.do(ctx, http.MethodGet, path)
		if err != nil {
			return nil, err
		}

		var resp listNotesResponse
		if err := json.Unmarshal(data, &resp); err != nil {
			return nil, fmt.Errorf("parse notes list: %w", err)
		}

		all = append(all, resp.Notes...)

		if !resp.HasMore || resp.Cursor == "" {
			break
		}
		cursor = resp.Cursor
	}

	return all, nil
}

// getNote returns the full detail for a single note.
func (c *client) getNote(ctx context.Context, id string) (*noteDetail, error) {
	data, err := c.do(ctx, http.MethodGet, "/notes/"+id)
	if err != nil {
		return nil, err
	}

	var nd noteDetail
	if err := json.Unmarshal(data, &nd); err != nil {
		return nil, fmt.Errorf("parse note detail: %w", err)
	}
	return &nd, nil
}

// --- Syncer ---

// Syncer periodically imports Granola meeting notes into Blackwood.
type Syncer struct {
	client  *client
	store   *storage.Store
	indexer *index.Index // may be nil
	poll    time.Duration
}

// New creates a new Granola syncer.
func New(apiKey string, store *storage.Store, indexer *index.Index, pollInterval time.Duration) *Syncer {
	return &Syncer{
		client:  newClient(apiKey),
		store:   store,
		indexer: indexer,
		poll:    pollInterval,
	}
}

// Start runs the sync loop until ctx is cancelled. It syncs immediately on
// start, then every poll interval.
func (s *Syncer) Start(ctx context.Context) {
	slog.Info("granola sync started", "interval", s.poll)

	// Run immediately on start.
	if err := s.sync(ctx); err != nil {
		slog.Error("granola sync failed", "error", err)
	}

	ticker := time.NewTicker(s.poll)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			slog.Info("granola sync stopped")
			return
		case <-ticker.C:
			if err := s.sync(ctx); err != nil {
				slog.Error("granola sync failed", "error", err)
			}
		}
	}
}

// sync fetches recently updated notes from Granola and imports any that are
// new or updated since last sync.
func (s *Syncer) sync(ctx context.Context) error {
	// Look back 7 days to catch any notes we might have missed.
	lookback := time.Now().UTC().Add(-7 * 24 * time.Hour)

	notes, err := s.client.listNotes(ctx, lookback)
	if err != nil {
		return fmt.Errorf("list notes: %w", err)
	}

	slog.Info("granola sync: fetched note list", "count", len(notes))

	var imported, skipped int
	for _, n := range notes {
		// Check if we already imported this version.
		existing, _ := s.store.GetGranolaSyncState(ctx, n.ID)
		if existing != nil && existing.UpdatedAt == n.UpdatedAt {
			skipped++
			continue
		}

		if err := s.importNote(ctx, n); err != nil {
			slog.Error("granola import failed", "note_id", n.ID, "title", n.Title, "error", err)
			continue
		}
		imported++
	}

	slog.Info("granola sync complete", "imported", imported, "skipped", skipped)
	return nil
}

// importNote fetches the full note detail and writes it into a Blackwood daily note.
func (s *Syncer) importNote(ctx context.Context, n Note) error {
	detail, err := s.client.getNote(ctx, n.ID)
	if err != nil {
		return fmt.Errorf("get note %s: %w", n.ID, err)
	}

	// Determine the date from the meeting start time or note creation time.
	date := parseDateFromISO(detail.CreatedAt)
	if detail.CalendarEvent != nil && detail.CalendarEvent.ScheduledStartTime != "" {
		if d := parseDateFromISO(detail.CalendarEvent.ScheduledStartTime); d != "" {
			date = d
		}
	}
	if date == "" {
		date = time.Now().UTC().Format("2006-01-02")
	}

	dailyNote, err := s.store.GetOrCreateDailyNote(ctx, date)
	if err != nil {
		return fmt.Errorf("get or create daily note: %w", err)
	}

	// Build markdown content for the entry.
	md := buildNoteMarkdown(detail)

	// Check if we're updating an existing entry or creating a new one.
	existing, _ := s.store.GetGranolaSyncState(ctx, n.ID)

	var entryID string
	if existing != nil && existing.EntryID != "" {
		// Update existing entry.
		entry, err := s.store.GetEntry(ctx, existing.EntryID)
		if err != nil {
			// Entry was deleted — create a new one.
			entryID, err = s.createEntry(ctx, dailyNote, md, n, date)
			if err != nil {
				return err
			}
		} else {
			entry.Content = md
			entry.RawContent = md
			if err := s.store.UpdateEntry(ctx, entry); err != nil {
				return fmt.Errorf("update entry: %w", err)
			}
			entryID = entry.ID
		}
	} else {
		entryID, err = s.createEntry(ctx, dailyNote, md, n, date)
		if err != nil {
			return err
		}
	}

	// Record sync state.
	state := &storage.GranolaSyncState{
		NoteID:    n.ID,
		EntryID:   entryID,
		UpdatedAt: n.UpdatedAt,
	}
	if err := s.store.UpsertGranolaSyncState(ctx, state); err != nil {
		return fmt.Errorf("upsert sync state: %w", err)
	}

	slog.Info("granola note imported", "note_id", n.ID, "title", n.Title, "date", date)
	return nil
}

func (s *Syncer) createEntry(ctx context.Context, dailyNote *storage.DailyNote, md string, n Note, date string) (string, error) {
	meta, _ := json.Marshal(map[string]string{
		"granola_note_id": n.ID,
		"granola_title":   n.Title,
	})

	entry := &storage.Entry{
		DailyNoteID: dailyNote.ID,
		Type:        "text",
		Content:     md,
		RawContent:  md,
		Source:      "import",
		Metadata:    string(meta),
	}
	if err := s.store.CreateEntry(ctx, entry); err != nil {
		return "", fmt.Errorf("create entry: %w", err)
	}

	// Append to the daily note markdown.
	snippet := "\n\n---\n*Imported from Granola*\n\n" + md + "\n"
	if err := s.store.AppendToSection(ctx, dailyNote.ID, "# Notes", snippet); err != nil {
		return "", fmt.Errorf("append to daily note: %w", err)
	}

	// Index for semantic search.
	if s.indexer != nil && md != "" {
		if err := s.indexer.IndexEntry(ctx, entry.ID, md); err != nil {
			slog.Warn("failed to index granola entry", "entry_id", entry.ID, "error", err)
		}
	}

	return entry.ID, nil
}

// buildNoteMarkdown formats a Granola note detail as markdown.
func buildNoteMarkdown(d *noteDetail) string {
	var md strings.Builder

	// Title
	fmt.Fprintf(&md, "## %s\n\n", d.Title)

	// Meeting metadata
	if d.CalendarEvent != nil {
		start := formatTime(d.CalendarEvent.ScheduledStartTime)
		end := formatTime(d.CalendarEvent.ScheduledEndTime)
		if start != "" && end != "" {
			fmt.Fprintf(&md, "**Time:** %s – %s\n\n", start, end)
		}
	}

	if len(d.Attendees) > 0 {
		var names []string
		for _, a := range d.Attendees {
			if a.Name != "" {
				names = append(names, a.Name)
			} else {
				names = append(names, a.Email)
			}
		}
		fmt.Fprintf(&md, "**Attendees:** %s\n\n", strings.Join(names, ", "))
	}

	// Summary (prefer markdown, fall back to text)
	if d.SummaryMarkdown != "" {
		md.WriteString(d.SummaryMarkdown)
		md.WriteString("\n")
	} else if d.SummaryText != "" {
		md.WriteString(d.SummaryText)
		md.WriteString("\n")
	}

	// Transcript
	if len(d.Transcript) > 0 {
		md.WriteString("\n### Transcript\n\n")
		for _, t := range d.Transcript {
			label := t.Speaker.Source
			if label == "microphone" {
				label = "You"
			} else {
				label = "Speaker"
			}
			fmt.Fprintf(&md, "> **%s:** %s\n>\n", label, t.Text)
		}
	}

	return md.String()
}

// parseDateFromISO extracts YYYY-MM-DD from an ISO 8601 timestamp.
func parseDateFromISO(iso string) string {
	if len(iso) < 10 {
		return ""
	}
	return iso[:10]
}

// formatTime formats an ISO 8601 timestamp as HH:MM.
func formatTime(iso string) string {
	t, err := time.Parse(time.RFC3339, iso)
	if err != nil {
		return ""
	}
	return t.Format("15:04")
}
