package positions

import (
	"bytes"
	"context"
	"slices"
	"sync"
	"time"

	"github.com/google/uuid"
	"go.opentelemetry.io/otel/trace"

	"github.com/makkssik/template/internal/domain/trip"
)

const droppedResult = "dropped"

type task struct {
	candidate trip.StaleTrip
	link      trace.SpanContext
	cancel    context.CancelFunc
}

type queue struct {
	mu       sync.Mutex
	pending  []*task
	entries  map[uuid.UUID]*task
	capacity int
	closed   bool
	changed  chan struct{}
}

func newQueue(capacity int) *queue {
	return &queue{capacity: capacity, entries: make(map[uuid.UUID]*task), changed: make(chan struct{})}
}

func (q *queue) signal() {
	close(q.changed)
	q.changed = make(chan struct{})
}

func compare(a, b *task) int {
	if c := a.candidate.LastSeen.Compare(b.candidate.LastSeen); c != 0 {
		return c
	}
	return bytes.Compare(a.candidate.TripID[:], b.candidate.TripID[:])
}

func (q *queue) offer(t *task) (string, *task) {
	q.mu.Lock()
	defer q.mu.Unlock()
	if q.closed {
		return droppedResult, t
	}
	if _, exists := q.entries[t.candidate.TripID]; exists {
		return "duplicate", nil
	}
	var evicted *task
	if len(q.pending) == q.capacity {
		newest := q.pending[len(q.pending)-1]
		if compare(t, newest) >= 0 {
			return droppedResult, t
		}
		evicted = newest
		delete(q.entries, newest.candidate.TripID)
		q.pending = q.pending[:len(q.pending)-1]
	}
	q.entries[t.candidate.TripID] = t
	q.pending = append(q.pending, t)
	slices.SortFunc(q.pending, compare)
	q.signal()
	return "queued", evicted
}

func (q *queue) take(ctx context.Context, timeout time.Duration) (*task, context.Context) {
	for {
		q.mu.Lock()
		if ctx.Err() != nil {
			q.mu.Unlock()
			return nil, nil
		}
		if len(q.pending) > 0 {
			t := q.pending[0]
			q.pending = slices.Delete(q.pending, 0, 1)
			taskCtx, cancel := context.WithTimeout(ctx, timeout)
			t.cancel = cancel
			q.mu.Unlock()
			return t, taskCtx
		}
		if q.closed {
			q.mu.Unlock()
			return nil, nil
		}
		changed := q.changed
		q.mu.Unlock()
		select {
		case <-ctx.Done():
			return nil, nil
		case <-changed:
		}
	}
}

func (q *queue) cancel(id uuid.UUID) bool {
	q.mu.Lock()
	defer q.mu.Unlock()
	t, ok := q.entries[id]
	if !ok {
		return false
	}
	if t.cancel != nil {
		t.cancel()
		return false
	}
	for i, v := range q.pending {
		if v == t {
			q.pending = slices.Delete(q.pending, i, i+1)
			break
		}
	}
	delete(q.entries, id)
	q.signal()
	return true
}

func (q *queue) finish(t *task) {
	q.mu.Lock()
	defer q.mu.Unlock()
	t.cancel()
	if q.entries[t.candidate.TripID] == t {
		delete(q.entries, t.candidate.TripID)
	}
}

func (q *queue) close() {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.closed = true
	q.signal()
}

func (q *queue) size() int64 {
	q.mu.Lock()
	defer q.mu.Unlock()
	return int64(len(q.pending))
}

func (q *queue) discard() int64 {
	q.mu.Lock()
	defer q.mu.Unlock()
	n := int64(len(q.pending))
	for _, t := range q.pending {
		delete(q.entries, t.candidate.TripID)
	}
	q.pending = nil
	return n
}
