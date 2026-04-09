package index

import "context"

// Indexer is the full interface for semantic indexing and search.
// Both the OpenAI-backed Index and the QMD-backed QMDIndex implement it.
type Indexer interface {
	IndexEntry(ctx context.Context, entryID string, text string) error
	Search(ctx context.Context, query string, limit int) ([]SearchResult, error)
	DeleteEntry(entryID string) error
	Reindex(ctx context.Context, entries []EntryForIndex) error
}

// EmbeddingClient computes text embeddings.
type EmbeddingClient interface {
	Embed(ctx context.Context, text string) ([]float32, error)
	EmbedBatch(ctx context.Context, texts []string) ([][]float32, error)
}
