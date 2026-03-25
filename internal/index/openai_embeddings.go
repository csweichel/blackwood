package index

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

const (
	defaultEmbeddingEndpoint = "https://api.openai.com/v1/embeddings"
	defaultEmbeddingModel    = "text-embedding-3-small"
	embeddingRequestTimeout  = 60 * time.Second
	embeddingMaxRetries      = 3
)

// OpenAIEmbeddingClient implements EmbeddingClient using the OpenAI embeddings API.
type OpenAIEmbeddingClient struct {
	apiKey   string
	model    string
	endpoint string // overridable for testing
}

// NewOpenAIEmbeddingClient creates a new OpenAI embedding client.
func NewOpenAIEmbeddingClient(apiKey string) *OpenAIEmbeddingClient {
	return &OpenAIEmbeddingClient{
		apiKey:   apiKey,
		model:    defaultEmbeddingModel,
		endpoint: defaultEmbeddingEndpoint,
	}
}

// WithModel overrides the embedding model used for requests.
func (c *OpenAIEmbeddingClient) WithModel(model string) *OpenAIEmbeddingClient {
	if model != "" {
		c.model = model
	}
	return c
}

type embeddingRequest struct {
	Model string   `json:"model"`
	Input []string `json:"input"`
}

type embeddingResponse struct {
	Data  []embeddingData `json:"data"`
	Error *embeddingError `json:"error,omitempty"`
}

type embeddingData struct {
	Embedding []float32 `json:"embedding"`
	Index     int       `json:"index"`
}

type embeddingError struct {
	Message string `json:"message"`
	Type    string `json:"type"`
}

func (c *OpenAIEmbeddingClient) Embed(ctx context.Context, text string) ([]float32, error) {
	results, err := c.EmbedBatch(ctx, []string{text})
	if err != nil {
		return nil, err
	}
	if len(results) == 0 {
		return nil, fmt.Errorf("no embeddings returned")
	}
	return results[0], nil
}

func (c *OpenAIEmbeddingClient) EmbedBatch(ctx context.Context, texts []string) ([][]float32, error) {
	if len(texts) == 0 {
		return nil, nil
	}

	reqBody := embeddingRequest{
		Model: c.model,
		Input: texts,
	}
	bodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("marshalling request: %w", err)
	}

	var lastErr error
	for attempt := range embeddingMaxRetries {
		respData, statusCode, err := c.doRequest(ctx, bodyBytes)
		if err != nil {
			if ctx.Err() != nil {
				return nil, ctx.Err()
			}
			lastErr = err
		} else if statusCode == http.StatusOK {
			// Order results by index to match input order.
			results := make([][]float32, len(texts))
			for _, d := range respData.Data {
				if d.Index < len(results) {
					results[d.Index] = d.Embedding
				}
			}
			return results, nil
		} else if statusCode == http.StatusTooManyRequests || statusCode >= 500 {
			lastErr = fmt.Errorf("API returned HTTP %d", statusCode)
		} else {
			return nil, fmt.Errorf("API returned HTTP %d", statusCode)
		}

		// Exponential backoff: 1s, 2s, 4s
		if attempt < embeddingMaxRetries-1 {
			backoff := time.Duration(1<<uint(attempt)) * time.Second
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(backoff):
			}
		}
	}

	return nil, fmt.Errorf("request failed after %d attempts: %w", embeddingMaxRetries, lastErr)
}

func (c *OpenAIEmbeddingClient) doRequest(ctx context.Context, body []byte) (*embeddingResponse, int, error) {
	ctx, cancel := context.WithTimeout(ctx, embeddingRequestTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, 0, fmt.Errorf("creating request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.apiKey)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, 0, fmt.Errorf("sending request: %w", err)
	}
	defer resp.Body.Close() //nolint:errcheck

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, resp.StatusCode, fmt.Errorf("reading response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, resp.StatusCode, nil
	}

	var embResp embeddingResponse
	if err := json.Unmarshal(respBody, &embResp); err != nil {
		return nil, resp.StatusCode, fmt.Errorf("parsing response: %w", err)
	}

	if embResp.Error != nil {
		return nil, resp.StatusCode, fmt.Errorf("API error: %s", embResp.Error.Message)
	}

	return &embResp, resp.StatusCode, nil
}
