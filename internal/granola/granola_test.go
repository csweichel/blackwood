package granola

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/csweichel/blackwood/internal/storage"
)

func TestBuildNoteMarkdown(t *testing.T) {
	t.Run("with transcript", func(t *testing.T) {
		d := &MeetingDetail{Text: "meeting content here"}
		md := buildNoteMarkdown(d, "speaker said things")
		if !strings.Contains(md, "meeting content here") {
			t.Errorf("expected detail text, got: %s", md)
		}
		if !strings.Contains(md, "### Transcript") {
			t.Errorf("expected transcript section, got: %s", md)
		}
		if !strings.Contains(md, "speaker said things") {
			t.Errorf("expected transcript text, got: %s", md)
		}
	})

	t.Run("without transcript", func(t *testing.T) {
		d := &MeetingDetail{Text: "just the meeting"}
		md := buildNoteMarkdown(d, "")
		if md != "just the meeting" {
			t.Errorf("expected raw text, got: %s", md)
		}
		if strings.Contains(md, "Transcript") {
			t.Errorf("should not contain transcript section, got: %s", md)
		}
	})
}

func TestParseMeetingsList(t *testing.T) {
	// Real-world format from Granola MCP.
	input := `<meetings_data from="Feb 23, 2026" to="Mar 25, 2026" count="2">
<meeting id="abc-123" title="Sprint Planning" date="Mar 25, 2026 10:30 AM">
    <known_participants>
    Alice, Bob
    </known_participants>
  </meeting>

<meeting id="def-456" title="1:1 Sync" date="Mar 24, 2026 2:00 PM">
    <known_participants>
    Charlie
    </known_participants>
  </meeting>
</meetings_data>`

	meetings, err := parseMeetingsList(input)
	if err != nil {
		t.Fatalf("parseMeetingsList: %v", err)
	}
	if len(meetings) != 2 {
		t.Fatalf("got %d meetings, want 2", len(meetings))
	}
	if meetings[0].ID != "abc-123" {
		t.Errorf("meetings[0].ID = %q, want %q", meetings[0].ID, "abc-123")
	}
	if meetings[0].Title != "Sprint Planning" {
		t.Errorf("meetings[0].Title = %q, want %q", meetings[0].Title, "Sprint Planning")
	}
	if meetings[1].ID != "def-456" {
		t.Errorf("meetings[1].ID = %q, want %q", meetings[1].ID, "def-456")
	}
}

func TestParseMeetingsListEmpty(t *testing.T) {
	_, err := parseMeetingsList("no meetings here")
	if err == nil {
		t.Fatal("expected error for empty response")
	}
}

func TestParseDate(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"2026-01-27T15:30:00Z", "2026-01-27"},
		{"2026-03-01", "2026-03-01"},
		{"Mar 25, 2026 10:30 AM", "2026-03-25"},
		{"Jan 2, 2026", "2026-01-02"},
		{"short", ""},
		{"", ""},
	}
	for _, tt := range tests {
		got := parseDate(tt.input)
		if got != tt.want {
			t.Errorf("parseDate(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

// fakeMCPServer creates an httptest server that speaks the MCP Streamable HTTP
// protocol (JSON-RPC over HTTP POST) and responds to initialize, tools/call.
// Responses use the same XML-like markup format as the real Granola MCP.
func fakeMCPServer(t *testing.T, meetings []Meeting, details map[string]*MeetingDetail) *httptest.Server {
	t.Helper()
	var sessionCounter atomic.Int64

	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		var req jsonRPCRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}

		w.Header().Set("Content-Type", "application/json")

		switch req.Method {
		case "initialize":
			sid := fmt.Sprintf("session-%d", sessionCounter.Add(1))
			w.Header().Set("Mcp-Session-Id", sid)
			resp := jsonRPCResponse{
				JSONRPC: "2.0",
				ID:      &req.ID,
				Result:  json.RawMessage(`{"protocolVersion":"2025-03-26","capabilities":{},"serverInfo":{"name":"granola-test","version":"1.0"}}`),
			}
			_ = json.NewEncoder(w).Encode(resp)

		case "notifications/initialized":
			w.WriteHeader(http.StatusAccepted)

		case "tools/call":
			paramsBytes, _ := json.Marshal(req.Params)
			var params callToolParams
			_ = json.Unmarshal(paramsBytes, &params)

			var resultText string
			switch params.Name {
			case "list_meetings":
				// Return XML-like markup matching real Granola format.
				var sb strings.Builder
				fmt.Fprintf(&sb, `<meetings_data from="Jan 1, 2026" to="Jan 31, 2026" count="%d">`, len(meetings))
				sb.WriteString("\n")
				for _, m := range meetings {
					fmt.Fprintf(&sb, `<meeting id="%s" title="%s" date="%s">`, m.ID, m.Title, m.Date)
					sb.WriteString("\n")
					if m.Participants != "" {
						fmt.Fprintf(&sb, "    <known_participants>\n    %s\n    </known_participants>\n", m.Participants)
					}
					sb.WriteString("  </meeting>\n\n")
				}
				sb.WriteString("</meetings_data>")
				resultText = sb.String()
			case "get_meetings":
				args := params.Arguments
				ids, _ := args["meeting_ids"].([]interface{})
				if len(ids) > 0 {
					id := fmt.Sprint(ids[0])
					if d, ok := details[id]; ok {
						resultText = d.Text
					}
				}
			case "get_meeting_transcript":
				resultText = `{"id":"mtg_test1","title":"Test Meeting","transcript":"> Speaker: This is a test transcript."}`
			default:
				resultText = "unknown tool"
			}

			result := callToolResult{
				Content: []toolContent{{Type: "text", Text: resultText}},
			}
			resultBytes, _ := json.Marshal(result)
			resp := jsonRPCResponse{
				JSONRPC: "2.0",
				ID:      &req.ID,
				Result:  resultBytes,
			}
			_ = json.NewEncoder(w).Encode(resp)

		default:
			resp := jsonRPCResponse{
				JSONRPC: "2.0",
				ID:      &req.ID,
				Error:   &jsonRPCError{Code: -32601, Message: "method not found"},
			}
			_ = json.NewEncoder(w).Encode(resp)
		}
	}))
}

func TestSyncWithFakeMCPServer(t *testing.T) {
	meetingText := `<meetings_data from="Jan 27, 2026" to="Jan 27, 2026" count="1">
<meeting id="mtg_test1" title="Test Meeting" date="Jan 27, 2026 3:30 PM">
    <known_participants>
    Alice, Bob
    </known_participants>
  </meeting>
</meetings_data>`

	meetings := []Meeting{
		{
			ID:           "mtg_test1",
			Title:        "Test Meeting",
			Date:         "Jan 27, 2026 3:30 PM",
			Participants: "Alice, Bob",
		},
	}

	details := map[string]*MeetingDetail{
		"mtg_test1": {
			Title: "Test Meeting",
			Date:  "Jan 27, 2026 3:30 PM",
			Text:  meetingText,
		},
	}

	srv := fakeMCPServer(t, meetings, details)
	defer srv.Close()

	// Set up storage.
	dir := t.TempDir()
	store, err := storage.New(filepath.Join(dir, "test.db"), dir)
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	defer func() { _ = store.Close() }()

	// Create syncer pointing at the fake MCP server.
	pt := &PersistedToken{AccessToken: "test-oauth-token"}
	ts := NewTokenSource(pt, "")
	syncer := New(ts, store, nil, 1*time.Hour)
	syncer.mcp.endpoint = srv.URL

	ctx := context.Background()

	// First sync should import the meeting.
	if err := syncer.sync(ctx); err != nil {
		t.Fatalf("first sync: %v", err)
	}

	// Verify sync state was recorded.
	state, err := store.GetGranolaSyncState(ctx, "mtg_test1")
	if err != nil {
		t.Fatalf("get sync state: %v", err)
	}
	if state.EntryID == "" {
		t.Fatal("expected non-empty entry ID")
	}

	// Verify the entry was created with correct content.
	entry, err := store.GetEntry(ctx, state.EntryID)
	if err != nil {
		t.Fatalf("get entry: %v", err)
	}
	if !strings.Contains(entry.Content, "Test Meeting") {
		t.Errorf("entry content should contain title, got: %s", entry.Content)
	}
	if !strings.Contains(entry.Content, "Transcript") {
		t.Errorf("entry content should contain transcript section, got: %s", entry.Content)
	}
	if entry.Source != "import" {
		t.Errorf("entry source = %q, want %q", entry.Source, "import")
	}

	// Verify the daily note contains a wikilink (not the full content).
	dailyNote, err := store.GetDailyNoteByDate(ctx, "2026-01-27")
	if err != nil {
		t.Fatalf("get daily note: %v", err)
	}
	if !strings.Contains(dailyNote.Content, "[[Test Meeting]]") {
		t.Errorf("daily note should contain wikilink, got: %s", dailyNote.Content)
	}
	if strings.Contains(dailyNote.Content, "Transcript") {
		t.Errorf("daily note should NOT contain full content, got: %s", dailyNote.Content)
	}

	// Verify the subpage file was created with the full content.
	subpagePath, err := store.SubpagePath("2026-01-27", "Test Meeting")
	if err != nil {
		t.Fatalf("subpage path: %v", err)
	}
	subpageContent, err := os.ReadFile(subpagePath)
	if err != nil {
		t.Fatalf("read subpage: %v", err)
	}
	if !strings.Contains(string(subpageContent), "Test Meeting") {
		t.Errorf("subpage should contain meeting content, got: %s", string(subpageContent))
	}
	if !strings.Contains(string(subpageContent), "Transcript") {
		t.Errorf("subpage should contain transcript, got: %s", string(subpageContent))
	}

	// Second sync should skip (already imported).
	if err := syncer.sync(ctx); err != nil {
		t.Fatalf("second sync: %v", err)
	}

	// Verify no duplicate — same entry ID.
	state2, err := store.GetGranolaSyncState(ctx, "mtg_test1")
	if err != nil {
		t.Fatalf("get sync state after second sync: %v", err)
	}
	if state2.EntryID != state.EntryID {
		t.Errorf("expected same entry ID after skip, got %q vs %q", state2.EntryID, state.EntryID)
	}
}

func TestLoadTokenJSON(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "token.json")

	original := &PersistedToken{
		AccessToken:  "access-123",
		RefreshToken: "refresh-456",
		ExpiresAt:    time.Now().Add(1 * time.Hour).Truncate(time.Second),
		ClientID:     "client-abc",
		ClientSecret: "secret-xyz",
	}
	if err := SaveToken(path, original); err != nil {
		t.Fatalf("save: %v", err)
	}

	loaded, err := LoadToken(path)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if loaded.AccessToken != "access-123" {
		t.Errorf("access_token = %q, want %q", loaded.AccessToken, "access-123")
	}
	if loaded.RefreshToken != "refresh-456" {
		t.Errorf("refresh_token = %q, want %q", loaded.RefreshToken, "refresh-456")
	}
	if loaded.ClientID != "client-abc" {
		t.Errorf("client_id = %q, want %q", loaded.ClientID, "client-abc")
	}
}

func TestLoadTokenLegacyPlainText(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "token")
	if err := os.WriteFile(path, []byte("  plain-token-value  \n"), 0o600); err != nil {
		t.Fatal(err)
	}

	loaded, err := LoadToken(path)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if loaded.AccessToken != "plain-token-value" {
		t.Errorf("access_token = %q, want %q", loaded.AccessToken, "plain-token-value")
	}
	if loaded.RefreshToken != "" {
		t.Errorf("refresh_token should be empty for legacy format, got %q", loaded.RefreshToken)
	}
}

func TestTokenSourceAutoRefresh(t *testing.T) {
	// Set up a fake token endpoint that issues refreshed tokens.
	refreshCalled := false
	tokenSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			http.Error(w, "bad form", http.StatusBadRequest)
			return
		}
		if r.FormValue("grant_type") != "refresh_token" {
			http.Error(w, "expected refresh_token grant", http.StatusBadRequest)
			return
		}
		if r.FormValue("refresh_token") != "old-refresh" {
			http.Error(w, "wrong refresh token", http.StatusBadRequest)
			return
		}
		refreshCalled = true
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(OAuthToken{
			AccessToken:  "new-access-token",
			RefreshToken: "new-refresh-token",
			ExpiresIn:    3600,
		})
	}))
	defer tokenSrv.Close()

	dir := t.TempDir()
	tokenPath := filepath.Join(dir, "token.json")

	pt := &PersistedToken{
		AccessToken:  "expired-access",
		RefreshToken: "old-refresh",
		ExpiresAt:    time.Now().Add(-1 * time.Hour), // already expired
		ClientID:     "test-client",
		TokenURL:     tokenSrv.URL,
	}

	ts := NewTokenSource(pt, tokenPath)

	tok, err := ts.AccessToken(context.Background())
	if err != nil {
		t.Fatalf("AccessToken: %v", err)
	}
	if !refreshCalled {
		t.Error("expected refresh to be called")
	}
	if tok != "new-access-token" {
		t.Errorf("access token = %q, want %q", tok, "new-access-token")
	}

	// Verify the token was persisted to disk.
	persisted, err := LoadToken(tokenPath)
	if err != nil {
		t.Fatalf("load persisted: %v", err)
	}
	if persisted.AccessToken != "new-access-token" {
		t.Errorf("persisted access_token = %q, want %q", persisted.AccessToken, "new-access-token")
	}
	if persisted.RefreshToken != "new-refresh-token" {
		t.Errorf("persisted refresh_token = %q, want %q", persisted.RefreshToken, "new-refresh-token")
	}
}

func TestTokenSourceNoRefreshWhenValid(t *testing.T) {
	pt := &PersistedToken{
		AccessToken:  "still-valid",
		RefreshToken: "some-refresh",
		ExpiresAt:    time.Now().Add(1 * time.Hour), // plenty of time
	}

	ts := NewTokenSource(pt, "")

	tok, err := ts.AccessToken(context.Background())
	if err != nil {
		t.Fatalf("AccessToken: %v", err)
	}
	if tok != "still-valid" {
		t.Errorf("access token = %q, want %q", tok, "still-valid")
	}
}

func TestMCPClientSSEResponse(t *testing.T) {
	// Test that the MCP client correctly parses SSE-streamed responses.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req jsonRPCRequest
		_ = json.NewDecoder(r.Body).Decode(&req)

		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)

		// Send response as SSE event.
		result := callToolResult{
			Content: []toolContent{{Type: "text", Text: "hello from SSE"}},
		}
		resultBytes, _ := json.Marshal(result)
		resp := jsonRPCResponse{
			JSONRPC: "2.0",
			ID:      &req.ID,
			Result:  resultBytes,
		}
		respBytes, _ := json.Marshal(resp)
		_, _ = fmt.Fprintf(w, "data: %s\n\n", respBytes)
	}))
	defer srv.Close()

	pt := &PersistedToken{AccessToken: "test-token"}
	ts := NewTokenSource(pt, "")
	client := newMCPClient(ts)
	client.endpoint = srv.URL

	text, err := client.callTool(context.Background(), "test_tool", nil)
	if err != nil {
		t.Fatalf("callTool via SSE: %v", err)
	}
	if text != "hello from SSE" {
		t.Errorf("got %q, want %q", text, "hello from SSE")
	}
}
