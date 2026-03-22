package ocr

import (
	"context"
)

// Recognizer converts page images to text.
type Recognizer interface {
	Recognize(ctx context.Context, image []byte) (string, error)
}
