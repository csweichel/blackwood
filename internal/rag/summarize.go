package rag

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

const summarizeTimeout = 60 * time.Second

const summarizeSystemPrompt = `You are a concise summarizer for daily notes. Given the full markdown content of a day's notes, produce a brief summary (2-4 sentences) highlighting key activities, decisions, and topics. Do not use bullet points. Write in third person past tense. Output only the summary text, no headings or labels.`

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
