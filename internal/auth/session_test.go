package auth

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/csweichel/blackwood/internal/storage"
)

func newTestStore(t *testing.T) *storage.Store {
	t.Helper()
	dir := t.TempDir()
	s, err := storage.New(filepath.Join(dir, "test.db"), dir)
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s
}

func TestCreateAndValidateSession(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	token, err := CreateSession(ctx, store)
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}
	if token == "" {
		t.Fatal("expected non-empty token")
	}
	if len(token) != 64 { // 32 bytes hex-encoded
		t.Errorf("token length = %d, want 64", len(token))
	}

	if !ValidateSession(ctx, store, token) {
		t.Error("ValidateSession returned false for a valid session")
	}
}

func TestValidateSession_Empty(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	if ValidateSession(ctx, store, "") {
		t.Error("ValidateSession returned true for empty token")
	}
}

func TestValidateSession_NonExistent(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	if ValidateSession(ctx, store, "nonexistent") {
		t.Error("ValidateSession returned true for non-existent token")
	}
}

func TestValidateSession_Expired(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	// Insert a session that's already expired.
	token := "expired-token-abc123"
	err := store.CreateSession(ctx, token, time.Now().Add(-1*time.Hour))
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}

	if ValidateSession(ctx, store, token) {
		t.Error("ValidateSession returned true for an expired session")
	}
}

func TestDeleteSession(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	token, err := CreateSession(ctx, store)
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}

	if !ValidateSession(ctx, store, token) {
		t.Fatal("session should be valid before deletion")
	}

	if err := store.DeleteSession(ctx, token); err != nil {
		t.Fatalf("DeleteSession: %v", err)
	}

	if ValidateSession(ctx, store, token) {
		t.Error("ValidateSession returned true after deletion")
	}
}

func TestCleanExpiredSessions(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	// Create one valid and one expired session.
	validToken, err := CreateSession(ctx, store)
	if err != nil {
		t.Fatalf("CreateSession (valid): %v", err)
	}

	expiredToken := "expired-token-xyz"
	if err := store.CreateSession(ctx, expiredToken, time.Now().Add(-1*time.Hour)); err != nil {
		t.Fatalf("CreateSession (expired): %v", err)
	}

	if err := store.CleanExpiredSessions(ctx); err != nil {
		t.Fatalf("CleanExpiredSessions: %v", err)
	}

	if !ValidateSession(ctx, store, validToken) {
		t.Error("valid session was incorrectly cleaned up")
	}
	if ValidateSession(ctx, store, expiredToken) {
		t.Error("expired session was not cleaned up")
	}
}
