package index

import (
	"context"
	"database/sql"
	"math"
	"path/filepath"
	"testing"

	_ "modernc.org/sqlite"
)

// mockEmbeddingClient returns deterministic embeddings based on text content.
type mockEmbeddingClient struct {
	embeddings map[string][]float32
}

func (m *mockEmbeddingClient) Embed(ctx context.Context, text string) ([]float32, error) {
	if emb, ok := m.embeddings[text]; ok {
		return emb, nil
	}
	// Default: return a zero vector.
	return make([]float32, 4), nil
}

func (m *mockEmbeddingClient) EmbedBatch(ctx context.Context, texts []string) ([][]float32, error) {
	results := make([][]float32, len(texts))
	for i, t := range texts {
		emb, err := m.Embed(ctx, t)
		if err != nil {
			return nil, err
		}
		results[i] = emb
	}
	return results, nil
}

func openTestDB(t *testing.T) *sql.DB {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "test.db")
	db, err := sql.Open("sqlite", dbPath+"?_pragma=foreign_keys(1)")
	if err != nil {
		t.Fatalf("opening test db: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

func TestCosineSimilarity_IdenticalVectors(t *testing.T) {
	a := []float32{1, 2, 3}
	score := cosineSimilarity(a, a)
	if math.Abs(score-1.0) > 1e-6 {
		t.Errorf("expected ~1.0 for identical vectors, got %f", score)
	}
}

func TestCosineSimilarity_OrthogonalVectors(t *testing.T) {
	a := []float32{1, 0, 0}
	b := []float32{0, 1, 0}
	score := cosineSimilarity(a, b)
	if math.Abs(score) > 1e-6 {
		t.Errorf("expected ~0.0 for orthogonal vectors, got %f", score)
	}
}

func TestCosineSimilarity_OppositeVectors(t *testing.T) {
	a := []float32{1, 2, 3}
	b := []float32{-1, -2, -3}
	score := cosineSimilarity(a, b)
	if math.Abs(score+1.0) > 1e-6 {
		t.Errorf("expected ~-1.0 for opposite vectors, got %f", score)
	}
}

func TestCosineSimilarity_ZeroVector(t *testing.T) {
	a := []float32{0, 0, 0}
	b := []float32{1, 2, 3}
	score := cosineSimilarity(a, b)
	if score != 0 {
		t.Errorf("expected 0 for zero vector, got %f", score)
	}
}

func TestIndexEntry_And_Search(t *testing.T) {
	db := openTestDB(t)
	client := &mockEmbeddingClient{
		embeddings: map[string][]float32{
			"cats are great":  {1, 0, 0, 0},
			"dogs are great":  {0.9, 0.1, 0, 0},
			"quantum physics": {0, 0, 1, 0},
			"cats":            {0.95, 0.05, 0, 0}, // query about cats
		},
	}

	idx, err := New(db, client)
	if err != nil {
		t.Fatalf("creating index: %v", err)
	}

	ctx := context.Background()

	if err := idx.IndexEntry(ctx, "e1", "cats are great"); err != nil {
		t.Fatalf("indexing e1: %v", err)
	}
	if err := idx.IndexEntry(ctx, "e2", "dogs are great"); err != nil {
		t.Fatalf("indexing e2: %v", err)
	}
	if err := idx.IndexEntry(ctx, "e3", "quantum physics"); err != nil {
		t.Fatalf("indexing e3: %v", err)
	}

	results, err := idx.Search(ctx, "cats", 10)
	if err != nil {
		t.Fatalf("searching: %v", err)
	}

	if len(results) != 3 {
		t.Fatalf("expected 3 results, got %d", len(results))
	}

	// "cats are great" should be the top result for "cats" query.
	if results[0].EntryID != "e1" {
		t.Errorf("expected top result to be e1, got %s", results[0].EntryID)
	}
	// "dogs are great" should be second (similar direction).
	if results[1].EntryID != "e2" {
		t.Errorf("expected second result to be e2, got %s", results[1].EntryID)
	}
	// Results should be sorted descending by score.
	for i := 1; i < len(results); i++ {
		if results[i].Score > results[i-1].Score {
			t.Errorf("results not sorted: score[%d]=%f > score[%d]=%f", i, results[i].Score, i-1, results[i-1].Score)
		}
	}
}

func TestIndexEntry_SkipsEmptyText(t *testing.T) {
	db := openTestDB(t)
	client := &mockEmbeddingClient{embeddings: map[string][]float32{}}

	idx, err := New(db, client)
	if err != nil {
		t.Fatalf("creating index: %v", err)
	}

	// Should not error on empty text.
	if err := idx.IndexEntry(context.Background(), "e1", ""); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should have no rows.
	var count int
	if err := db.QueryRow(`SELECT COUNT(*) FROM embeddings`).Scan(&count); err != nil {
		t.Fatalf("counting rows: %v", err)
	}
	if count != 0 {
		t.Errorf("expected 0 rows, got %d", count)
	}
}

func TestDeleteEntry(t *testing.T) {
	db := openTestDB(t)
	client := &mockEmbeddingClient{
		embeddings: map[string][]float32{
			"hello": {1, 0, 0, 0},
		},
	}

	idx, err := New(db, client)
	if err != nil {
		t.Fatalf("creating index: %v", err)
	}

	ctx := context.Background()
	if err := idx.IndexEntry(ctx, "e1", "hello"); err != nil {
		t.Fatalf("indexing: %v", err)
	}

	if err := idx.DeleteEntry("e1"); err != nil {
		t.Fatalf("deleting: %v", err)
	}

	var count int
	if err := db.QueryRow(`SELECT COUNT(*) FROM embeddings WHERE entry_id = ?`, "e1").Scan(&count); err != nil {
		t.Fatalf("counting: %v", err)
	}
	if count != 0 {
		t.Errorf("expected 0 rows after delete, got %d", count)
	}
}

func TestSearch_RespectsLimit(t *testing.T) {
	db := openTestDB(t)
	client := &mockEmbeddingClient{
		embeddings: map[string][]float32{
			"a":     {1, 0, 0, 0},
			"b":     {0, 1, 0, 0},
			"c":     {0, 0, 1, 0},
			"query": {0.5, 0.5, 0.5, 0},
		},
	}

	idx, err := New(db, client)
	if err != nil {
		t.Fatalf("creating index: %v", err)
	}

	ctx := context.Background()
	for _, e := range []struct{ id, text string }{{"e1", "a"}, {"e2", "b"}, {"e3", "c"}} {
		if err := idx.IndexEntry(ctx, e.id, e.text); err != nil {
			t.Fatalf("indexing %s: %v", e.id, err)
		}
	}

	results, err := idx.Search(ctx, "query", 2)
	if err != nil {
		t.Fatalf("searching: %v", err)
	}
	if len(results) != 2 {
		t.Errorf("expected 2 results with limit=2, got %d", len(results))
	}
}

func TestReindex(t *testing.T) {
	db := openTestDB(t)
	client := &mockEmbeddingClient{
		embeddings: map[string][]float32{
			"text one": {1, 0, 0, 0},
			"text two": {0, 1, 0, 0},
			"":         {0, 0, 0, 0},
		},
	}

	idx, err := New(db, client)
	if err != nil {
		t.Fatalf("creating index: %v", err)
	}

	entries := []EntryForIndex{
		{ID: "e1", Text: "text one"},
		{ID: "e2", Text: "text two"},
		{ID: "e3", Text: ""}, // should be skipped
	}

	if err := idx.Reindex(context.Background(), entries); err != nil {
		t.Fatalf("reindexing: %v", err)
	}

	var count int
	if err := db.QueryRow(`SELECT COUNT(*) FROM embeddings`).Scan(&count); err != nil {
		t.Fatalf("counting: %v", err)
	}
	if count != 2 {
		t.Errorf("expected 2 rows (empty text skipped), got %d", count)
	}
}

func TestIndexEntry_Upsert(t *testing.T) {
	db := openTestDB(t)
	client := &mockEmbeddingClient{
		embeddings: map[string][]float32{
			"version 1": {1, 0, 0, 0},
			"version 2": {0, 1, 0, 0},
			"query":     {0, 1, 0, 0},
		},
	}

	idx, err := New(db, client)
	if err != nil {
		t.Fatalf("creating index: %v", err)
	}

	ctx := context.Background()

	// Index, then re-index same entry with different text.
	if err := idx.IndexEntry(ctx, "e1", "version 1"); err != nil {
		t.Fatalf("indexing v1: %v", err)
	}
	if err := idx.IndexEntry(ctx, "e1", "version 2"); err != nil {
		t.Fatalf("indexing v2: %v", err)
	}

	// Should have exactly one row.
	var count int
	if err := db.QueryRow(`SELECT COUNT(*) FROM embeddings`).Scan(&count); err != nil {
		t.Fatalf("counting: %v", err)
	}
	if count != 1 {
		t.Errorf("expected 1 row after upsert, got %d", count)
	}

	// The embedding should match "version 2".
	results, err := idx.Search(ctx, "query", 1)
	if err != nil {
		t.Fatalf("searching: %v", err)
	}
	if len(results) != 1 || results[0].Snippet != "version 2" {
		t.Errorf("expected snippet 'version 2', got %v", results)
	}
}
