package rag

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

const summarizeTimeout = 60 * time.Second

const summarizeSystemPrompt = `Summarize this daily note in one sentence, max 160 characters. Write in a neutral, impersonal style — do not start with "They" or any pronoun. Focus on topics and activities. No bullet points, no headings, no labels. Just the summary text.`

type nonStreamingResponse struct {
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
}

// Summarize generates a short summary of the given note content using the
// configured OpenAI model. Returns the summary text.
func (e *Engine) Summarize(ctx context.Context, content string) (string, error) {
	if content == "" {
		return "", fmt.Errorf("empty content")
	}

	// Strip existing # Summary section to avoid summarizing the previous summary.
	content = stripSection(content, "# Summary")

	if strings.TrimSpace(content) == "" {
		return "", fmt.Errorf("no content after stripping summary")
	}

	messages := []chatMsg{
		{Role: "system", Content: summarizeSystemPrompt},
		{Role: "user", Content: content},
	}

	reqBody := chatCompletionRequest{
		Model:    e.model,
		Messages: messages,
		Stream:   false,
	}

	bodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("marshal request: %w", err)
	}

	ctx, cancel := context.WithTimeout(ctx, summarizeTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, chatEndpoint, bytes.NewReader(bodyBytes))
	if err != nil {
		return "", fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+e.apiKey)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("send request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("OpenAI API returned HTTP %d: %s", resp.StatusCode, string(body))
	}

	var result nonStreamingResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", fmt.Errorf("decode response: %w", err)
	}

	if len(result.Choices) == 0 {
		return "", fmt.Errorf("no choices in response")
	}

	return result.Choices[0].Message.Content, nil
}

// stripSection removes a markdown section and its body from content.
func stripSection(content, heading string) string {
	idx := -1
	if strings.HasPrefix(content, heading+"\n") {
		idx = 0
	} else if i := strings.Index(content, "\n"+heading+"\n"); i >= 0 {
		idx = i + 1
	}
	if idx < 0 {
		return content
	}

	afterHeading := idx + len(heading) + 1
	nextSection := -1
	if i := strings.Index(content[afterHeading:], "\n# "); i >= 0 {
		nextSection = afterHeading + i + 1
	}

	if nextSection >= 0 {
		return content[:idx] + content[nextSection:]
	}
	return content[:idx]
}
