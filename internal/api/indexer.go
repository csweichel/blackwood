package api

import "context"

// EntryIndexer is the subset of indexing operations used by API handlers.
type EntryIndexer interface {
	IndexEntry(ctx context.Context, entryID string, text string) error
	DeleteEntry(entryID string) error
}
