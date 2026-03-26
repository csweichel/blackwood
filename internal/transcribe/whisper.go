package transcribe

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"strings"
	"time"
)

const (
	defaultWhisperEndpoint = "https://api.openai.com/v1/audio/transcriptions"
	whisperRequestTimeout  = 120 * time.Second
	whisperMaxRetries      = 3
)

// WhisperTranscriber implements Transcriber using the OpenAI Whisper API.
type WhisperTranscriber struct {
	apiKey   string
	endpoint string
}

// NewWhisper creates a new WhisperTranscriber.
func NewWhisper(apiKey string) *WhisperTranscriber {
	return &WhisperTranscriber{
		apiKey:   apiKey,
		endpoint: defaultWhisperEndpoint,
	}
}

// whisperResponse is the JSON response from the Whisper API.
type whisperResponse struct {
	Text string `json:"text"`
}

func (t *WhisperTranscriber) Transcribe(ctx context.Context, audio []byte, format string) (string, error) {
	var lastErr error
	for attempt := range whisperMaxRetries {
		result, statusCode, err := t.doRequest(ctx, audio, format)
		if err != nil {
			if ctx.Err() != nil {
				return "", ctx.Err()
			}
			lastErr = err
		} else if statusCode == http.StatusOK {
			return result, nil
		} else if statusCode == http.StatusTooManyRequests || statusCode >= 500 {
			lastErr = fmt.Errorf("API returned HTTP %d: %s", statusCode, result)
		} else {
			return "", fmt.Errorf("API returned HTTP %d: %s", statusCode, result)
		}

		// Exponential backoff: 1s, 2s, 4s
		if attempt < whisperMaxRetries-1 {
			backoff := time.Duration(1<<uint(attempt)) * time.Second
			select {
			case <-ctx.Done():
				return "", ctx.Err()
			case <-time.After(backoff):
			}
		}
	}

	return "", fmt.Errorf("request failed after %d attempts: %w", whisperMaxRetries, lastErr)
}

// doRequest performs a single multipart HTTP request to the Whisper API.
func (t *WhisperTranscriber) doRequest(ctx context.Context, audio []byte, format string) (string, int, error) {
	ctx, cancel := context.WithTimeout(ctx, whisperRequestTimeout)
	defer cancel()

	var body bytes.Buffer
	writer := multipart.NewWriter(&body)

	// Add the audio file part.
	filename := "audio." + format
	partHeader := make(textproto.MIMEHeader)
	partHeader.Set("Content-Disposition", fmt.Sprintf(`form-data; name="%s"; filename="%s"`, "file", filename))
	partHeader.Set("Content-Type", mimeTypeForFormat(format))
	part, err := writer.CreatePart(partHeader)
	if err != nil {
		return "", 0, fmt.Errorf("creating form file: %w", err)
	}
	if _, err := part.Write(audio); err != nil {
		return "", 0, fmt.Errorf("writing audio data: %w", err)
	}

	// Add the model field.
	if err := writer.WriteField("model", "whisper-1"); err != nil {
		return "", 0, fmt.Errorf("writing model field: %w", err)
	}

	if err := writer.Close(); err != nil {
		return "", 0, fmt.Errorf("closing multipart writer: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, t.endpoint, &body)
	if err != nil {
		return "", 0, fmt.Errorf("creating request: %w", err)
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())
	req.Header.Set("Authorization", "Bearer "+t.apiKey)

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

	var whisperResp whisperResponse
	if err := json.Unmarshal(respBody, &whisperResp); err != nil {
		return "", resp.StatusCode, fmt.Errorf("parsing response: %w", err)
	}

	return whisperResp.Text, resp.StatusCode, nil
}

func mimeTypeForFormat(format string) string {
	switch strings.ToLower(format) {
	case "m4a":
		return "audio/x-m4a"
	case "mp4":
		return "audio/mp4"
	case "mp3", "mpeg", "mpga":
		return "audio/mpeg"
	case "wav":
		return "audio/wav"
	case "ogg", "oga":
		return "audio/ogg"
	case "webm":
		return "audio/webm"
	case "flac":
		return "audio/flac"
	default:
		return "application/octet-stream"
	}
}
