package index

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"math"
	"sort"
)

const embeddingsSchema = `
CREATE TABLE IF NOT EXISTS embeddings (
    entry_id TEXT PRIMARY KEY,
    embedding TEXT NOT NULL,
    snippet TEXT NOT NULL DEFAULT '',
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);
`

// Index provides semantic search over entries using embeddings stored in SQLite.
type Index struct {
	db     *sql.DB
	client EmbeddingClient
}

// SearchResult represents a single search hit.
type SearchResult struct {
	EntryID string
	Score   float64
	Snippet string
}

// EntryForIndex is the minimal data needed to index an entry.
type EntryForIndex struct {
	ID   string
	Text string
}

// New creates an Index, ensuring the embeddings table exists.
func New(db *sql.DB, client EmbeddingClient) (*Index, error) {
	if _, err := db.Exec(embeddingsSchema); err != nil {
		return nil, fmt.Errorf("creating embeddings table: %w", err)
	}
	return &Index{db: db, client: client}, nil
}

// IndexEntry computes an embedding for text and stores it. Empty text is skipped.
func (idx *Index) IndexEntry(ctx context.Context, entryID string, text string) error {
	if text == "" {
		return nil
	}

	emb, err := idx.client.Embed(ctx, text)
	if err != nil {
		return fmt.Errorf("computing embedding: %w", err)
	}

	embJSON, err := json.Marshal(emb)
	if err != nil {
		return fmt.Errorf("marshalling embedding: %w", err)
	}

	snippet := truncate(text, 200)

	_, err = idx.db.ExecContext(ctx,
		`INSERT INTO embeddings (entry_id, embedding, snippet) VALUES (?, ?, ?)
		 ON CONFLICT(entry_id) DO UPDATE SET embedding = excluded.embedding, snippet = excluded.snippet`,
		entryID, string(embJSON), snippet,
	)
	if err != nil {
		return fmt.Errorf("storing embedding: %w", err)
	}
	return nil
}

// Search computes a query embedding, then ranks all stored embeddings by cosine similarity.
func (idx *Index) Search(ctx context.Context, query string, limit int) ([]SearchResult, error) {
	if limit <= 0 {
		limit = 10
	}

	queryEmb, err := idx.client.Embed(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("computing query embedding: %w", err)
	}

	rows, err := idx.db.QueryContext(ctx, `SELECT entry_id, embedding, snippet FROM embeddings`)
	if err != nil {
		return nil, fmt.Errorf("querying embeddings: %w", err)
	}
	defer rows.Close()

	var results []SearchResult
	for rows.Next() {
		var entryID, embJSON, snippet string
		if err := rows.Scan(&entryID, &embJSON, &snippet); err != nil {
			return nil, fmt.Errorf("scanning row: %w", err)
		}

		var emb []float32
		if err := json.Unmarshal([]byte(embJSON), &emb); err != nil {
			continue // skip malformed rows
		}

		score := cosineSimilarity(queryEmb, emb)
		results = append(results, SearchResult{
			EntryID: entryID,
			Score:   score,
			Snippet: snippet,
		})
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating rows: %w", err)
	}

	sort.Slice(results, func(i, j int) bool {
		return results[i].Score > results[j].Score
	})

	if len(results) > limit {
		results = results[:limit]
	}
	return results, nil
}

// DeleteEntry removes the embedding for an entry.
func (idx *Index) DeleteEntry(entryID string) error {
	_, err := idx.db.Exec(`DELETE FROM embeddings WHERE entry_id = ?`, entryID)
	if err != nil {
		return fmt.Errorf("deleting embedding: %w", err)
	}
	return nil
}

// Reindex computes embeddings in batches and upserts them.
func (idx *Index) Reindex(ctx context.Context, entries []EntryForIndex) error {
	const batchSize = 50

	for i := 0; i < len(entries); i += batchSize {
		end := i + batchSize
		if end > len(entries) {
			end = len(entries)
		}
		batch := entries[i:end]

		// Filter out empty texts.
		var texts []string
		var indices []int
		for j, e := range batch {
			if e.Text != "" {
				texts = append(texts, e.Text)
				indices = append(indices, j)
			}
		}
		if len(texts) == 0 {
			continue
		}

		embeddings, err := idx.client.EmbedBatch(ctx, texts)
		if err != nil {
			return fmt.Errorf("batch embedding: %w", err)
		}

		for k, emb := range embeddings {
			entry := batch[indices[k]]
			embJSON, err := json.Marshal(emb)
			if err != nil {
				return fmt.Errorf("marshalling embedding: %w", err)
			}
			snippet := truncate(entry.Text, 200)

			_, err = idx.db.ExecContext(ctx,
				`INSERT INTO embeddings (entry_id, embedding, snippet) VALUES (?, ?, ?)
				 ON CONFLICT(entry_id) DO UPDATE SET embedding = excluded.embedding, snippet = excluded.snippet`,
				entry.ID, string(embJSON), snippet,
			)
			if err != nil {
				return fmt.Errorf("storing embedding for %s: %w", entry.ID, err)
			}
		}
	}
	return nil
}

func cosineSimilarity(a, b []float32) float64 {
	var dot, normA, normB float64
	for i := range a {
		if i >= len(b) {
			break
		}
		dot += float64(a[i]) * float64(b[i])
		normA += float64(a[i]) * float64(a[i])
		normB += float64(b[i]) * float64(b[i])
	}
	if normA == 0 || normB == 0 {
		return 0
	}
	return dot / (math.Sqrt(normA) * math.Sqrt(normB))
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen]
}
