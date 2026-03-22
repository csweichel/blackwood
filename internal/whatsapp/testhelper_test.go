package whatsapp

import (
	"testing"

	"github.com/csweichel/blackwood/internal/storage"
)

func newTestStore(t *testing.T) *storage.Store {
	t.Helper()
	dir := t.TempDir()
	store, err := storage.New(dir+"/test.db", dir)
	if err != nil {
		t.Fatalf("create test store: %v", err)
	}
	return store
}
