package describe

import "context"

// Describer generates text descriptions of images.
type Describer interface {
	Describe(ctx context.Context, image []byte) (string, error)
}
