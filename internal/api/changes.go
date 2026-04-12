package api

import (
	"sync"
	"time"

	blackwoodv1 "github.com/csweichel/blackwood/gen/blackwood/v1"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func revisionString(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.UTC().Format(time.RFC3339Nano)
}

type ChangeBroadcaster struct {
	mu          sync.Mutex
	subscribers map[chan *blackwoodv1.ChangeEvent]struct{}
}

func NewChangeBroadcaster() *ChangeBroadcaster {
	return &ChangeBroadcaster{
		subscribers: make(map[chan *blackwoodv1.ChangeEvent]struct{}),
	}
}

func (b *ChangeBroadcaster) Subscribe() (<-chan *blackwoodv1.ChangeEvent, func()) {
	if b == nil {
		ch := make(chan *blackwoodv1.ChangeEvent)
		close(ch)
		return ch, func() {}
	}

	ch := make(chan *blackwoodv1.ChangeEvent, 32)

	b.mu.Lock()
	b.subscribers[ch] = struct{}{}
	b.mu.Unlock()

	return ch, func() {
		b.mu.Lock()
		if _, ok := b.subscribers[ch]; ok {
			delete(b.subscribers, ch)
			close(ch)
		}
		b.mu.Unlock()
	}
}

func (b *ChangeBroadcaster) Publish(event *blackwoodv1.ChangeEvent) {
	if b == nil || event == nil {
		return
	}

	b.mu.Lock()
	defer b.mu.Unlock()

	for ch := range b.subscribers {
		select {
		case ch <- event:
		default:
			// Drop stale events for slow consumers. The stream is used for
			// invalidation, so newer events supersede older ones.
		}
	}
}

func (b *ChangeBroadcaster) PublishDailyNote(date string, updatedAt time.Time) {
	b.Publish(&blackwoodv1.ChangeEvent{
		Kind:      blackwoodv1.ChangeEventKind_CHANGE_EVENT_KIND_DAILY_NOTE_UPDATED,
		Date:      date,
		Revision:  revisionString(updatedAt),
		ChangedAt: timestamppb.New(updatedAt.UTC()),
	})
}

func (b *ChangeBroadcaster) PublishSubpage(date, name string, updatedAt time.Time) {
	b.Publish(&blackwoodv1.ChangeEvent{
		Kind:        blackwoodv1.ChangeEventKind_CHANGE_EVENT_KIND_SUBPAGE_UPDATED,
		Date:        date,
		SubpageName: name,
		Revision:    revisionString(updatedAt),
		ChangedAt:   timestamppb.New(updatedAt.UTC()),
	})
}
