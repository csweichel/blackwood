package api

import (
	"context"
	"strings"
	"sync"
	"testing"

	"connectrpc.com/connect"

	blackwoodv1 "github.com/csweichel/blackwood/gen/blackwood/v1"
)

func TestCreateEntryIsIdempotentForClientRequestID(t *testing.T) {
	store := newUploadTestStore(t)
	handler := NewDailyNotesHandler(store, nil, nil, nil)

	const attempts = 8
	entryIDs := make(chan string, attempts)
	errs := make(chan error, attempts)
	var wg sync.WaitGroup

	for i := 0; i < attempts; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			response, err := handler.CreateEntry(
				context.Background(),
				connect.NewRequest(&blackwoodv1.CreateEntryRequest{
					Date:                   "2026-07-09",
					Type:                   blackwoodv1.EntryType_ENTRY_TYPE_AUDIO,
					Source:                 blackwoodv1.EntrySource_ENTRY_SOURCE_API,
					ClientRequestId:        "recording-request-123",
					AttachmentData:         [][]byte{[]byte("audio-data")},
					AttachmentFilenames:    []string{"recording.m4a"},
					AttachmentContentTypes: []string{"audio/x-m4a"},
				}),
			)
			if err != nil {
				errs <- err
				return
			}
			entryIDs <- response.Msg.Id
		}()
	}

	wg.Wait()
	close(errs)
	close(entryIDs)

	for err := range errs {
		t.Fatalf("create entry: %v", err)
	}

	var firstID string
	for id := range entryIDs {
		if firstID == "" {
			firstID = id
		}
		if id != firstID {
			t.Fatalf("entry ID = %q, want every retry to return %q", id, firstID)
		}
	}
	if firstID == "" {
		t.Fatal("expected an entry ID")
	}

	note, err := store.GetDailyNoteByDate(context.Background(), "2026-07-09")
	if err != nil {
		t.Fatalf("get daily note: %v", err)
	}
	entries, err := store.ListEntries(context.Background(), note.ID)
	if err != nil {
		t.Fatalf("list entries: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("entries = %d, want 1", len(entries))
	}
	attachments, err := store.ListAttachments(context.Background(), entries[0].ID)
	if err != nil {
		t.Fatalf("list attachments: %v", err)
	}
	if len(attachments) != 1 {
		t.Fatalf("attachments = %d, want 1", len(attachments))
	}
	if count := strings.Count(note.Content, "Voice memo"); count != 1 {
		t.Fatalf("voice memo snippets = %d, want 1\nnote content:\n%s", count, note.Content)
	}

	mapped, err := store.GetEntryByClientRequestID(context.Background(), "recording-request-123")
	if err != nil {
		t.Fatalf("get idempotency mapping: %v", err)
	}
	if mapped == nil || mapped.ID != firstID {
		t.Fatalf("mapped entry = %v, want ID %s", mapped, firstID)
	}
}
