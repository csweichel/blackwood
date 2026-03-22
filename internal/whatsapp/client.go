package whatsapp

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
	graphAPIBase   = "https://graph.facebook.com/v21.0"
	requestTimeout = 30 * time.Second
)

// Client interacts with the WhatsApp Business Cloud API.
type Client struct {
	accessToken   string
	phoneNumberID string
	httpClient    *http.Client
	baseURL       string // overridable for testing
}

func NewClient(accessToken, phoneNumberID string) *Client {
	return &Client{
		accessToken:   accessToken,
		phoneNumberID: phoneNumberID,
		httpClient:    &http.Client{Timeout: requestTimeout},
		baseURL:       graphAPIBase,
	}
}

// mediaResponse is the JSON response when fetching media metadata.
type mediaResponse struct {
	URL      string `json:"url"`
	MimeType string `json:"mime_type"`
}

// DownloadMedia downloads a media file from WhatsApp.
// It first fetches the media URL from the media ID, then downloads the actual file.
func (c *Client) DownloadMedia(ctx context.Context, mediaID string) ([]byte, string, error) {
	// Step 1: Get media URL
	url := fmt.Sprintf("%s/%s", c.baseURL, mediaID)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, "", fmt.Errorf("create media metadata request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.accessToken)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, "", fmt.Errorf("fetch media metadata: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, "", fmt.Errorf("media metadata HTTP %d: %s", resp.StatusCode, body)
	}

	var media mediaResponse
	if err := json.NewDecoder(resp.Body).Decode(&media); err != nil {
		return nil, "", fmt.Errorf("decode media metadata: %w", err)
	}

	// Step 2: Download the actual media file
	dlReq, err := http.NewRequestWithContext(ctx, http.MethodGet, media.URL, nil)
	if err != nil {
		return nil, "", fmt.Errorf("create media download request: %w", err)
	}
	dlReq.Header.Set("Authorization", "Bearer "+c.accessToken)

	dlResp, err := c.httpClient.Do(dlReq)
	if err != nil {
		return nil, "", fmt.Errorf("download media: %w", err)
	}
	defer func() { _ = dlResp.Body.Close() }()

	if dlResp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(dlResp.Body)
		return nil, "", fmt.Errorf("media download HTTP %d: %s", dlResp.StatusCode, body)
	}

	data, err := io.ReadAll(dlResp.Body)
	if err != nil {
		return nil, "", fmt.Errorf("read media data: %w", err)
	}

	return data, media.MimeType, nil
}

// sendMessageRequest is the JSON body for sending a WhatsApp message.
type sendMessageRequest struct {
	MessagingProduct string      `json:"messaging_product"`
	To               string      `json:"to"`
	Type             string      `json:"type"`
	Text             *textBody   `json:"text,omitempty"`
}

type textBody struct {
	Body string `json:"body"`
}

// SendTextMessage sends a text reply to a WhatsApp user.
func (c *Client) SendTextMessage(ctx context.Context, to, text string) error {
	url := fmt.Sprintf("%s/%s/messages", c.baseURL, c.phoneNumberID)

	payload := sendMessageRequest{
		MessagingProduct: "whatsapp",
		To:               to,
		Type:             "text",
		Text:             &textBody{Body: text},
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal send message: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("create send message request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.accessToken)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("send message: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("send message HTTP %d: %s", resp.StatusCode, respBody)
	}

	return nil
}
