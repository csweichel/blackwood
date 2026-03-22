package ocr

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

const (
	defaultEndpoint   = "https://api.openai.com/v1/chat/completions"
	requestTimeout    = 60 * time.Second
	maxRetries        = 3
	maxTokens         = 4096
)

// OpenAIRecognizer implements Recognizer using the OpenAI chat completions API.
type OpenAIRecognizer struct {
	apiKey   string
	model    string
	prompt   string
	endpoint string // overridable for testing
}

// NewOpenAI creates a new OpenAIRecognizer.
func NewOpenAI(apiKey, model, prompt string) *OpenAIRecognizer {
	return &OpenAIRecognizer{
		apiKey:   apiKey,
		model:    model,
		prompt:   prompt,
		endpoint: defaultEndpoint,
	}
}

// chatRequest is the request body for the chat completions API.
type chatRequest struct {
	Model     string        `json:"model"`
	Messages  []chatMessage `json:"messages"`
	MaxTokens int           `json:"max_tokens"`
}

// chatMessage represents a single message in the chat.
type chatMessage struct {
	Role    string      `json:"role"`
	Content interface{} `json:"content"`
}

// contentPart represents a part of a multi-part user message.
type contentPart struct {
	Type     string    `json:"type"`
	Text     string    `json:"text,omitempty"`
	ImageURL *imageURL `json:"image_url,omitempty"`
}

type imageURL struct {
	URL string `json:"url"`
}

// chatResponse is the response body from the chat completions API.
type chatResponse struct {
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
	Error *apiError `json:"error,omitempty"`
}

type apiError struct {
	Message string `json:"message"`
	Type    string `json:"type"`
}

func (r *OpenAIRecognizer) Recognize(ctx context.Context, image []byte) (string, error) {
	dataURL := "data:image/png;base64," + base64.StdEncoding.EncodeToString(image)

	reqBody := chatRequest{
		Model: r.model,
		Messages: []chatMessage{
			{Role: "system", Content: r.prompt},
			{Role: "user", Content: []contentPart{
				{Type: "image_url", ImageURL: &imageURL{URL: dataURL}},
			}},
		},
		MaxTokens: maxTokens,
	}

	bodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("marshalling request: %w", err)
	}

	var lastErr error
	for attempt := range maxRetries {
		result, statusCode, err := r.doRequest(ctx, bodyBytes)
		if err != nil {
			// Context cancellation is not retryable.
			if ctx.Err() != nil {
				return "", ctx.Err()
			}
			lastErr = err
		} else if statusCode == http.StatusOK {
			return result, nil
		} else if statusCode == http.StatusTooManyRequests || statusCode >= 500 {
			lastErr = fmt.Errorf("API returned HTTP %d: %s", statusCode, result)
		} else {
			// 4xx errors (other than 429) are not retryable.
			return "", fmt.Errorf("API returned HTTP %d: %s", statusCode, result)
		}

		// Exponential backoff: 1s, 2s, 4s
		if attempt < maxRetries-1 {
			backoff := time.Duration(1<<uint(attempt)) * time.Second
			select {
			case <-ctx.Done():
				return "", ctx.Err()
			case <-time.After(backoff):
			}
		}
	}

	return "", fmt.Errorf("request failed after %d attempts: %w", maxRetries, lastErr)
}

// doRequest performs a single HTTP request and returns the response text, status code, and any error.
func (r *OpenAIRecognizer) doRequest(ctx context.Context, body []byte) (string, int, error) {
	ctx, cancel := context.WithTimeout(ctx, requestTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, r.endpoint, bytes.NewReader(body))
	if err != nil {
		return "", 0, fmt.Errorf("creating request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+r.apiKey)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", 0, fmt.Errorf("sending request: %w", err)
	}
	defer resp.Body.Close() //nolint:errcheck

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", resp.StatusCode, fmt.Errorf("reading response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return string(respBody), resp.StatusCode, nil
	}

	var chatResp chatResponse
	if err := json.Unmarshal(respBody, &chatResp); err != nil {
		return "", resp.StatusCode, fmt.Errorf("parsing response: %w", err)
	}

	if chatResp.Error != nil {
		return "", resp.StatusCode, fmt.Errorf("API error: %s", chatResp.Error.Message)
	}

	if len(chatResp.Choices) == 0 {
		return "", resp.StatusCode, fmt.Errorf("no choices in response")
	}

	return chatResp.Choices[0].Message.Content, resp.StatusCode, nil
}
