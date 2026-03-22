package transcribe

import "context"

// Transcriber converts audio to text.
type Transcriber interface {
	Transcribe(ctx context.Context, audio []byte, format string) (string, error)
}
