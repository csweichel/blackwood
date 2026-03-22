package rag

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
	"time"

	"github.com/csweichel/blackwood/internal/index"
	"github.com/csweichel/blackwood/internal/storage"
)

const (
	chatEndpoint   = "https://api.openai.com/v1/chat/completions"
	requestTimeout = 120 * time.Second
)

// Message represents a conversation message for building the prompt.
type Message struct {
	Role    string
	Content string
}

// SourceReference holds information about a source used to answer a query.
type SourceReference struct {
	EntryID       string
	DailyNoteDate string
	Snippet       string
	Score         float64
}

// Engine provides RAG-based chat over the user's daily notes.
type Engine struct {
	index  *index.Index
	store  *storage.Store
	apiKey string
	model  string
}

// New creates a new RAG engine.
func New(idx *index.Index, store *storage.Store, apiKey, model string) *Engine {
	if model == "" {
		model = "gpt-5.2"
	}
	return &Engine{
		index:  idx,
		store:  store,
		apiKey: apiKey,
		model:  model,
	}
}

// Query searches the index, builds context from matching entries, and streams
// a response from the OpenAI chat completions API. Returns a channel of text
// chunks and the source references used.
func (e *Engine) Query(ctx context.Context, query string, conversationHistory []Message) (<-chan string, []SourceReference, error) {
	// Search the semantic index for relevant entries.
	results, err := e.index.Search(ctx, query, 5)
	if err != nil {
		return nil, nil, fmt.Errorf("search index: %w", err)
	}

	// Build context from search results.
	var contextParts []string
	var sources []SourceReference

	for _, r := range results {
		entry, err := e.store.GetEntry(ctx, r.EntryID)
		if err != nil {
			slog.Warn("failed to fetch entry for RAG context", "entry_id", r.EntryID, "error", err)
			continue
		}

		// Look up the daily note to get the date.
		var date string
		dn, err := e.store.GetDailyNote(ctx, entry.DailyNoteID)
		if err == nil {
			date = dn.Date
		}

		content := entry.Content
		if len(content) > 2000 {
			content = content[:2000]
		}

		contextParts = append(contextParts, fmt.Sprintf("[Date: %s, Type: %s] %s", date, entry.Type, content))
		sources = append(sources, SourceReference{
			EntryID:       r.EntryID,
			DailyNoteDate: date,
			Snippet:       r.Snippet,
			Score:         r.Score,
		})
	}

	systemPrompt := "You are a helpful assistant with access to the user's daily notes. " +
		"Use the following context from their notes to answer questions. " +
		"Always cite your sources by referencing the date.\n\nContext:\n"
	if len(contextParts) > 0 {
		systemPrompt += strings.Join(contextParts, "\n")
	} else {
		systemPrompt += "(No relevant notes found)"
	}

	// Build the messages array for the API call.
	var apiMessages []chatMsg
	apiMessages = append(apiMessages, chatMsg{Role: "system", Content: systemPrompt})
	for _, m := range conversationHistory {
		apiMessages = append(apiMessages, chatMsg{Role: m.Role, Content: m.Content})
	}
	apiMessages = append(apiMessages, chatMsg{Role: "user", Content: query})

	ch, err := e.streamChat(ctx, apiMessages)
	if err != nil {
		return nil, nil, err
	}

	return ch, sources, nil
}

// --- OpenAI streaming chat ---

type chatMsg struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type chatCompletionRequest struct {
	Model    string    `json:"model"`
	Messages []chatMsg `json:"messages"`
	Stream   bool      `json:"stream"`
}

type streamDelta struct {
	Content string `json:"content"`
}

type streamChoice struct {
	Delta        streamDelta `json:"delta"`
	FinishReason *string     `json:"finish_reason"`
}

type streamChunk struct {
	Choices []streamChoice `json:"choices"`
}

func (e *Engine) streamChat(ctx context.Context, messages []chatMsg) (<-chan string, error) {
	reqBody := chatCompletionRequest{
		Model:    e.model,
		Messages: messages,
		Stream:   true,
	}

	bodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	ctx, cancel := context.WithTimeout(ctx, requestTimeout)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, chatEndpoint, bytes.NewReader(bodyBytes))
	if err != nil {
		cancel()
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+e.apiKey)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		cancel()
		return nil, fmt.Errorf("send request: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		cancel()
		return nil, fmt.Errorf("OpenAI API returned HTTP %d: %s", resp.StatusCode, string(body))
	}

	ch := make(chan string, 64)
	go func() {
		defer cancel()
		defer close(ch)
		defer resp.Body.Close()

		scanner := bufio.NewScanner(resp.Body)
		for scanner.Scan() {
			line := scanner.Text()
			if !strings.HasPrefix(line, "data: ") {
				continue
			}
			data := strings.TrimPrefix(line, "data: ")
			if data == "[DONE]" {
				return
			}

			var chunk streamChunk
			if err := json.Unmarshal([]byte(data), &chunk); err != nil {
				continue
			}
			for _, choice := range chunk.Choices {
				if choice.Delta.Content != "" {
					select {
					case ch <- choice.Delta.Content:
					case <-ctx.Done():
						return
					}
				}
			}
		}
	}()

	return ch, nil
}
