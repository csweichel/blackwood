// Package granola provides a periodic sync that imports meeting notes from
// Granola via its MCP (Model Context Protocol) server into Blackwood daily notes.
//
// The Granola MCP server at https://mcp.granola.ai/mcp exposes tools for
// listing and retrieving meeting notes. Authentication is via OAuth 2.0 —
// run `blackwood granola-login` to obtain a token.
package granola

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"sync/atomic"
	"time"

	"github.com/csweichel/blackwood/internal/index"
	"github.com/csweichel/blackwood/internal/storage"
)

const (
	mcpEndpoint     = "https://mcp.granola.ai/mcp"
	protocolVersion = "2025-03-26"
)

// --- JSON-RPC types ---

type jsonRPCRequest struct {
	JSONRPC string      `json:"jsonrpc"`
	ID      int64       `json:"id,omitempty"`
	Method  string      `json:"method"`
	Params  interface{} `json:"params,omitempty"`
}

type jsonRPCResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      *int64          `json:"id,omitempty"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *jsonRPCError   `json:"error,omitempty"`
}

type jsonRPCError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// --- MCP types ---

type initializeParams struct {
	ProtocolVersion string         `json:"protocolVersion"`
	Capabilities    map[string]any `json:"capabilities"`
	ClientInfo      clientInfo     `json:"clientInfo"`
}

type clientInfo struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

type callToolParams struct {
	Name      string         `json:"name"`
	Arguments map[string]any `json:"arguments,omitempty"`
}

type callToolResult struct {
	Content []toolContent `json:"content"`
	IsError bool          `json:"isError,omitempty"`
}

type toolContent struct {
	Type string `json:"type"`
	Text string `json:"text,omitempty"`
}

// --- MCP client ---

type mcpClient struct {
	tokenSource *TokenSource
	endpoint    string
	sessionID   string
	httpClient  *http.Client
	nextID      atomic.Int64
}

func newMCPClient(ts *TokenSource) *mcpClient {
	return &mcpClient{
		tokenSource: ts,
		endpoint:    mcpEndpoint,
		httpClient:  &http.Client{Timeout: 60 * time.Second},
	}
}

// initialize performs the MCP initialization handshake.
func (c *mcpClient) initialize(ctx context.Context) error {
	req := jsonRPCRequest{
		JSONRPC: "2.0",
		ID:      c.nextID.Add(1),
		Method:  "initialize",
		Params: initializeParams{
			ProtocolVersion: protocolVersion,
			Capabilities:    map[string]any{},
			ClientInfo: clientInfo{
				Name:    "blackwood",
				Version: "1.0",
			},
		},
	}

	resp, err := c.send(ctx, req)
	if err != nil {
		return fmt.Errorf("initialize: %w", err)
	}
	if resp.Error != nil {
		return fmt.Errorf("initialize error: %s", resp.Error.Message)
	}

	// Send initialized notification (no ID = notification).
	notif := jsonRPCRequest{
		JSONRPC: "2.0",
		Method:  "notifications/initialized",
	}
	return c.sendNotification(ctx, notif)
}

// callTool invokes an MCP tool and returns the text content.
func (c *mcpClient) callTool(ctx context.Context, name string, args map[string]any) (string, error) {
	req := jsonRPCRequest{
		JSONRPC: "2.0",
		ID:      c.nextID.Add(1),
		Method:  "tools/call",
		Params: callToolParams{
			Name:      name,
			Arguments: args,
		},
	}

	resp, err := c.send(ctx, req)
	if err != nil {
		return "", fmt.Errorf("call %s: %w", name, err)
	}
	if resp.Error != nil {
		return "", fmt.Errorf("call %s error: %s", name, resp.Error.Message)
	}

	var result callToolResult
	if err := json.Unmarshal(resp.Result, &result); err != nil {
		return "", fmt.Errorf("parse %s result: %w", name, err)
	}
	if result.IsError {
		if len(result.Content) > 0 {
			return "", fmt.Errorf("tool %s error: %s", name, result.Content[0].Text)
		}
		return "", fmt.Errorf("tool %s returned error", name)
	}

	var texts []string
	for _, c := range result.Content {
		if c.Type == "text" && c.Text != "" {
			texts = append(texts, c.Text)
		}
	}
	return strings.Join(texts, "\n"), nil
}

// send sends a JSON-RPC request and returns the response.
// Handles both direct JSON responses and SSE-streamed responses.
func (c *mcpClient) send(ctx context.Context, rpcReq jsonRPCRequest) (*jsonRPCResponse, error) {
	body, err := json.Marshal(rpcReq)
	if err != nil {
		return nil, err
	}

	token, err := c.tokenSource.AccessToken(ctx)
	if err != nil {
		return nil, fmt.Errorf("get access token: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	req.Header.Set("Authorization", "Bearer "+token)
	if c.sessionID != "" {
		req.Header.Set("Mcp-Session-Id", c.sessionID)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	// Capture session ID from response.
	if sid := resp.Header.Get("Mcp-Session-Id"); sid != "" {
		c.sessionID = sid
	}

	if resp.StatusCode == http.StatusNotFound && c.sessionID != "" {
		return nil, fmt.Errorf("session expired")
	}
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return nil, fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(b))
	}

	ct := resp.Header.Get("Content-Type")
	if strings.HasPrefix(ct, "text/event-stream") {
		return c.readSSEResponse(resp.Body, rpcReq.ID)
	}

	// Direct JSON response.
	var rpcResp jsonRPCResponse
	if err := json.NewDecoder(resp.Body).Decode(&rpcResp); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}
	return &rpcResp, nil
}

// readSSEResponse reads an SSE stream and returns the JSON-RPC response
// matching the given request ID.
func (c *mcpClient) readSSEResponse(r io.Reader, requestID int64) (*jsonRPCResponse, error) {
	scanner := bufio.NewScanner(r)
	var dataLines []string

	for scanner.Scan() {
		line := scanner.Text()

		if strings.HasPrefix(line, "data: ") {
			dataLines = append(dataLines, strings.TrimPrefix(line, "data: "))
			continue
		}

		// Empty line = end of SSE event.
		if line == "" && len(dataLines) > 0 {
			data := strings.Join(dataLines, "\n")
			dataLines = nil

			var resp jsonRPCResponse
			if err := json.Unmarshal([]byte(data), &resp); err != nil {
				continue
			}

			if resp.ID != nil && *resp.ID == requestID {
				return &resp, nil
			}
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("read SSE: %w", err)
	}
	return nil, fmt.Errorf("SSE stream ended without response for request %d", requestID)
}

// sendNotification sends a JSON-RPC notification (no response expected).
func (c *mcpClient) sendNotification(ctx context.Context, notif jsonRPCRequest) error {
	body, err := json.Marshal(notif)
	if err != nil {
		return err
	}

	token, err := c.tokenSource.AccessToken(ctx)
	if err != nil {
		return fmt.Errorf("get access token: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.endpoint, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	req.Header.Set("Authorization", "Bearer "+token)
	if c.sessionID != "" {
		req.Header.Set("Mcp-Session-Id", c.sessionID)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	_ = resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusAccepted {
		return fmt.Errorf("notification rejected: HTTP %d", resp.StatusCode)
	}
	return nil
}

// --- Granola meeting types (parsed from MCP tool responses) ---

// Meeting represents a meeting from list_meetings.
type Meeting struct {
	ID        string   `json:"id"`
	Title     string   `json:"title"`
	Date      string   `json:"date"`
	Attendees []string `json:"attendees"`
}

// MeetingDetail represents the full content from get_meetings.
type MeetingDetail struct {
	ID            string   `json:"id"`
	Title         string   `json:"title"`
	Date          string   `json:"date"`
	Attendees     []string `json:"attendees"`
	PrivateNotes  string   `json:"private_notes"`
	EnhancedNotes string   `json:"enhanced_notes"`
}

// --- Syncer ---

// Syncer periodically imports Granola meeting notes into Blackwood via MCP.
type Syncer struct {
	mcp     *mcpClient
	store   *storage.Store
	indexer *index.Index // may be nil
	poll    time.Duration
}

// New creates a new Granola syncer.
func New(ts *TokenSource, store *storage.Store, indexer *index.Index, pollInterval time.Duration) *Syncer {
	return &Syncer{
		mcp:     newMCPClient(ts),
		store:   store,
		indexer: indexer,
		poll:    pollInterval,
	}
}

// Start runs the sync loop until ctx is cancelled.
func (s *Syncer) Start(ctx context.Context) {
	slog.Info("granola MCP sync started", "interval", s.poll)

	if err := s.sync(ctx); err != nil {
		slog.Error("granola sync failed", "error", err)
	}

	ticker := time.NewTicker(s.poll)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			slog.Info("granola MCP sync stopped")
			return
		case <-ticker.C:
			if err := s.sync(ctx); err != nil {
				slog.Error("granola sync failed", "error", err)
			}
		}
	}
}

// sync initializes an MCP session, lists meetings, and imports new/updated ones.
func (s *Syncer) sync(ctx context.Context) error {
	if err := s.mcp.initialize(ctx); err != nil {
		return fmt.Errorf("MCP initialize: %w", err)
	}

	meetings, err := s.listMeetings(ctx)
	if err != nil {
		return fmt.Errorf("list meetings: %w", err)
	}

	slog.Info("granola sync: fetched meeting list", "count", len(meetings))

	var imported, skipped int
	for _, m := range meetings {
		existing, _ := s.store.GetGranolaSyncState(ctx, m.ID)
		if existing != nil {
			skipped++
			continue
		}

		if err := s.importMeeting(ctx, m); err != nil {
			slog.Error("granola import failed", "meeting_id", m.ID, "title", m.Title, "error", err)
			continue
		}
		imported++
	}

	slog.Info("granola sync complete", "imported", imported, "skipped", skipped)
	return nil
}

// listMeetings calls the list_meetings MCP tool and parses the result.
func (s *Syncer) listMeetings(ctx context.Context) ([]Meeting, error) {
	text, err := s.mcp.callTool(ctx, "list_meetings", map[string]any{})
	if err != nil {
		return nil, err
	}

	var meetings []Meeting
	if err := json.Unmarshal([]byte(text), &meetings); err != nil {
		// Try to extract a JSON array from the text.
		start := strings.Index(text, "[")
		end := strings.LastIndex(text, "]")
		if start >= 0 && end > start {
			if err2 := json.Unmarshal([]byte(text[start:end+1]), &meetings); err2 != nil {
				return nil, fmt.Errorf("parse meetings: %w (raw: %s)", err, truncate(text, 200))
			}
		} else {
			return nil, fmt.Errorf("parse meetings: %w (raw: %s)", err, truncate(text, 200))
		}
	}
	return meetings, nil
}

// getMeetingDetail calls get_meetings to fetch full content for a meeting.
func (s *Syncer) getMeetingDetail(ctx context.Context, meetingID string) (*MeetingDetail, error) {
	text, err := s.mcp.callTool(ctx, "get_meetings", map[string]any{
		"meeting_ids": []string{meetingID},
	})
	if err != nil {
		return nil, err
	}

	var details []MeetingDetail
	if err := json.Unmarshal([]byte(text), &details); err == nil && len(details) > 0 {
		return &details[0], nil
	}

	var detail MeetingDetail
	if err := json.Unmarshal([]byte(text), &detail); err == nil && detail.ID != "" {
		return &detail, nil
	}

	start := strings.Index(text, "{")
	end := strings.LastIndex(text, "}")
	if start >= 0 && end > start {
		if err := json.Unmarshal([]byte(text[start:end+1]), &detail); err == nil {
			return &detail, nil
		}
	}

	return nil, fmt.Errorf("parse meeting detail (raw: %s)", truncate(text, 200))
}

// getTranscript calls get_meeting_transcript for a meeting.
func (s *Syncer) getTranscript(ctx context.Context, meetingID string) (string, error) {
	text, err := s.mcp.callTool(ctx, "get_meeting_transcript", map[string]any{
		"meeting_id": meetingID,
	})
	if err != nil {
		slog.Debug("transcript not available", "meeting_id", meetingID, "error", err)
		return "", nil
	}
	return text, nil
}

// importMeeting fetches full detail and transcript, then writes to a daily note.
func (s *Syncer) importMeeting(ctx context.Context, m Meeting) error {
	detail, err := s.getMeetingDetail(ctx, m.ID)
	if err != nil {
		return fmt.Errorf("get detail for %s: %w", m.ID, err)
	}

	transcript, _ := s.getTranscript(ctx, m.ID)

	date := parseDateFromISO(m.Date)
	if date == "" && detail.Date != "" {
		date = parseDateFromISO(detail.Date)
	}
	if date == "" {
		date = time.Now().UTC().Format("2006-01-02")
	}

	dailyNote, err := s.store.GetOrCreateDailyNote(ctx, date)
	if err != nil {
		return fmt.Errorf("get or create daily note: %w", err)
	}

	md := buildNoteMarkdown(detail, transcript)

	meta, _ := json.Marshal(map[string]string{
		"granola_meeting_id": m.ID,
		"granola_title":      m.Title,
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
		return fmt.Errorf("create entry: %w", err)
	}

	snippet := "\n\n---\n*Imported from Granola*\n\n" + md + "\n"
	if err := s.store.AppendToSection(ctx, dailyNote.ID, "# Notes", snippet); err != nil {
		return fmt.Errorf("append to daily note: %w", err)
	}

	if s.indexer != nil && md != "" {
		if err := s.indexer.IndexEntry(ctx, entry.ID, md); err != nil {
			slog.Warn("failed to index granola entry", "entry_id", entry.ID, "error", err)
		}
	}

	state := &storage.GranolaSyncState{
		NoteID:    m.ID,
		EntryID:   entry.ID,
		UpdatedAt: m.Date,
	}
	if err := s.store.UpsertGranolaSyncState(ctx, state); err != nil {
		return fmt.Errorf("upsert sync state: %w", err)
	}

	slog.Info("granola meeting imported", "meeting_id", m.ID, "title", m.Title, "date", date)
	return nil
}

// buildNoteMarkdown formats a meeting as markdown.
func buildNoteMarkdown(d *MeetingDetail, transcript string) string {
	var md strings.Builder

	fmt.Fprintf(&md, "## %s\n\n", d.Title)

	if d.Date != "" {
		if t := formatTime(d.Date); t != "" {
			fmt.Fprintf(&md, "**Date:** %s\n\n", t)
		}
	}

	if len(d.Attendees) > 0 {
		fmt.Fprintf(&md, "**Attendees:** %s\n\n", strings.Join(d.Attendees, ", "))
	}

	if d.EnhancedNotes != "" {
		md.WriteString(d.EnhancedNotes)
		md.WriteString("\n")
	}

	if d.PrivateNotes != "" {
		md.WriteString("\n### Private Notes\n\n")
		md.WriteString(d.PrivateNotes)
		md.WriteString("\n")
	}

	if transcript != "" {
		md.WriteString("\n### Transcript\n\n")
		md.WriteString(transcript)
		md.WriteString("\n")
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

// formatTime formats an ISO 8601 timestamp as a readable date/time.
func formatTime(iso string) string {
	t, err := time.Parse(time.RFC3339, iso)
	if err != nil {
		t, err = time.Parse("2006-01-02", iso)
		if err != nil {
			return iso
		}
		return t.Format("2006-01-02")
	}
	return t.Format("2006-01-02 15:04")
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}
