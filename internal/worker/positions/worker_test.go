package positions

import (
	"bytes"
	"context"
	"errors"
	"io"
	"log/slog"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jonboulle/clockwork"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"go.uber.org/goleak"
	"go.uber.org/mock/gomock"

	"github.com/makkssik/template/internal/domain/trip"
)

func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m)
}

type fixture struct {
	w      *Worker
	repo   *Mockrepository
	push   *Mockgateway
	clock  *clockwork.FakeClock
	reader *sdkmetric.ManualReader
	spans  *tracetest.SpanRecorder
}

func setup(t *testing.T, size, workers int) fixture {
	t.Helper()
	ctrl := gomock.NewController(t)
	repo, push := NewMockrepository(ctrl), NewMockgateway(ctrl)
	clock := clockwork.NewFakeClockAt(time.Date(2026, 11, 15, 12, 0, 0, 0, time.UTC))
	reader := sdkmetric.NewManualReader()
	meter := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))
	spans := tracetest.NewSpanRecorder()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(spans))
	metrics, err := NewMetrics(meter.Meter("test"))
	if err != nil {
		t.Fatal(err)
	}
	w, err := New(Config{Interval: 30 * time.Second, StaleAfter: time.Minute, Timeout: time.Second, Workers: workers, QueueSize: size, BatchSize: 100}, repo, push, clock, slog.New(slog.NewTextHandler(io.Discard, nil)), tp.Tracer("test"), metrics)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := meter.Shutdown(context.Background()); err != nil {
			t.Error(err)
		}
		if err := tp.Shutdown(context.Background()); err != nil {
			t.Error(err)
		}
	})
	return fixture{w, repo, push, clock, reader, spans}
}

func candidate(at time.Time) trip.StaleTrip {
	return trip.StaleTrip{TripID: uuid.New(), DriverID: uuid.New(), LastSeen: at}
}

func wait[T any](t *testing.T, ch <-chan T) T {
	t.Helper()
	select {
	case v := <-ch:
		return v
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for worker")
		var zero T
		return zero
	}
}

func stop(t *testing.T, w *Worker) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := w.Shutdown(ctx); err != nil {
		t.Fatal(err)
	}
}

func TestPriorityEvictionCancellationAndDedup(t *testing.T) {
	q := newQueue(2)
	now := time.Now()
	old := &task{candidate: candidate(now.Add(-time.Hour))}
	middle := &task{candidate: candidate(now.Add(-time.Minute))}
	fresh := &task{candidate: candidate(now)}
	q.offer(middle)
	q.offer(fresh)
	result, evicted := q.offer(old)
	if result != "queued" || evicted != fresh {
		t.Fatalf("old task did not evict freshest: %s %v", result, evicted)
	}
	if result, _ := q.offer(old); result != "duplicate" {
		t.Fatal(result)
	}
	if result, rejected := q.offer(fresh); result != "dropped" || rejected != fresh {
		t.Fatal("overflow did not reject fresh task")
	}
	got, _ := q.take(t.Context(), time.Second)
	if got != old {
		t.Fatal("oldest must run first")
	}
	if result, _ := q.offer(old); result != "duplicate" {
		t.Fatal("in-flight task not deduplicated")
	}
	q.cancel(middle.candidate.TripID)
	q.finish(got)
	if q.size() != 0 {
		t.Fatal("canceled task remained queued")
	}
	q.offer(middle)
	q.close()
	got, _ = q.take(t.Context(), time.Second)
	if got != middle {
		t.Fatal("re-enqueued task lost")
	}
	q.finish(got)
	if got, _ := q.take(t.Context(), time.Second); got != nil {
		t.Fatal("closed queue did not drain")
	}
}

func TestBatchesRead250ExactlyOnce(t *testing.T) {
	f := setup(t, 300, 1)
	values := make([]trip.StaleTrip, 250)
	for i := range values {
		values[i] = candidate(f.clock.Now().Add(time.Duration(i-1000) * time.Second))
	}
	cutoff := f.clock.Now().Add(-time.Minute)
	gomock.InOrder(
		f.repo.EXPECT().FindStale(gomock.Any(), cutoff, nil, 100).Return(values[:100], nil),
		f.repo.EXPECT().FindStale(gomock.Any(), cutoff, &values[99], 100).Return(values[100:200], nil),
		f.repo.EXPECT().FindStale(gomock.Any(), cutoff, &values[199], 100).Return(values[200:], nil),
	)
	f.w.scan(t.Context())
	if f.w.queue.size() != 250 {
		t.Fatalf("queue=%d", f.w.queue.size())
	}
	for _, want := range values {
		got, _ := f.w.queue.take(t.Context(), time.Second)
		if got.candidate != want {
			t.Fatal("batch order or content changed")
		}
		f.w.queue.finish(got)
	}
	data := collect(t, f.reader)
	found := false
	for _, scope := range data.ScopeMetrics {
		for _, m := range scope.Metrics {
			if m.Name == "tripgo.stale_checks.rows_read" {
				h := m.Data.(metricdata.Histogram[int64])
				found = len(h.DataPoints) == 1 && h.DataPoints[0].Sum == 250 && h.DataPoints[0].Count == 1
			}
		}
	}
	if !found {
		t.Fatal("missing rows-per-tick measurement")
	}
}

func TestScannerStopsAfterFirstFullBatch(t *testing.T) {
	f := setup(t, 50, 1)
	trips := make([]trip.StaleTrip, 250)
	for i := range trips {
		trips[i] = candidate(f.clock.Now().Add(time.Duration(i-1000) * time.Second))
	}
	f.repo.EXPECT().FindStale(gomock.Any(), gomock.Any(), nil, 100).Return(trips[:100], nil)
	f.w.scan(t.Context())
	if got := f.w.queue.size(); got != 50 {
		t.Fatalf("queue size = %d, want 50", got)
	}
}

func collect(t *testing.T, r *sdkmetric.ManualReader) metricdata.ResourceMetrics {
	t.Helper()
	var data metricdata.ResourceMetrics
	if err := r.Collect(t.Context(), &data); err != nil {
		t.Fatal(err)
	}
	return data
}

func TestScannerOverflowAndDuplicateMetrics(t *testing.T) {
	f := setup(t, 1, 1)
	first := candidate(f.clock.Now().Add(-2 * time.Hour))
	second := candidate(f.clock.Now().Add(-time.Hour))
	f.repo.EXPECT().FindStale(gomock.Any(), gomock.Any(), nil, 100).Return([]trip.StaleTrip{first}, nil)
	f.w.scan(t.Context())
	f.repo.EXPECT().FindStale(gomock.Any(), gomock.Any(), nil, 100).Return([]trip.StaleTrip{first, second}, nil)
	f.w.scan(t.Context())
	if f.w.queue.size() != 1 {
		t.Fatal("queue exceeded capacity")
	}
	older := candidate(first.LastSeen.Add(-time.Minute))
	f.repo.EXPECT().FindStale(gomock.Any(), gomock.Any(), nil, 100).Return([]trip.StaleTrip{older}, nil)
	f.w.scan(t.Context())
	sums := resultCounters(t, f.reader)
	if sums["tripgo.stale_checks/queued"] != 2 || sums["tripgo.stale_checks/duplicate"] != 1 || sums["tripgo.stale_checks/dropped"] != 2 || len(sums) != 3 {
		t.Fatalf("unexpected counters: %v", sums)
	}
}

func resultCounters(t *testing.T, reader *sdkmetric.ManualReader) map[string]int64 {
	t.Helper()
	sums := make(map[string]int64)
	for _, scope := range collect(t, reader).ScopeMetrics {
		for _, m := range scope.Metrics {
			if sum, ok := m.Data.(metricdata.Sum[int64]); ok {
				for _, p := range sum.DataPoints {
					v, _ := p.Attributes.Value(attribute.Key("result"))
					sums[m.Name+"/"+v.AsString()] += p.Value
				}
			}
		}
	}
	return sums
}

func TestWorkersBoundedDrainAndRecoverFromError(t *testing.T) {
	f := setup(t, 10, 2)
	entered := make(chan struct{}, 10)
	release := make(chan struct{})
	var active, maxActive, calls atomic.Int32
	f.repo.EXPECT().StillStale(gomock.Any(), gomock.Any()).Return(true, nil).Times(6)
	f.push.EXPECT().RequestPosition(gomock.Any(), gomock.Any(), gomock.Any()).DoAndReturn(func(ctx context.Context, _, _ uuid.UUID) error {
		n := active.Add(1)
		defer active.Add(-1)
		for old := maxActive.Load(); n > old; old = maxActive.Load() {
			if maxActive.CompareAndSwap(old, n) {
				break
			}
		}
		call := calls.Add(1)
		entered <- struct{}{}
		select {
		case <-release:
		case <-ctx.Done():
			return ctx.Err()
		}
		if call == 1 {
			return errors.New("push failed")
		}
		return nil
	}).Times(6)
	for i := range 6 {
		f.w.queue.offer(&task{candidate: candidate(f.clock.Now().Add(time.Duration(i) * time.Second))})
	}
	f.w.Start(t.Context())
	wait(t, entered)
	wait(t, entered)
	if f.w.queue.size() != 4 || maxActive.Load() != 2 {
		t.Fatal("worker count is not bounded")
	}
	close(release)
	stop(t, f.w)
	if calls.Load() != 6 || f.w.queue.size() != 0 || f.w.active.Load() != 0 {
		t.Fatal("shutdown did not drain all tasks")
	}
}

func TestCoordinateCancelsQueuedAndInFlight(t *testing.T) {
	f := setup(t, 5, 1)
	a := candidate(f.clock.Now())
	b := candidate(f.clock.Now().Add(time.Second))
	entered := make(chan struct{})
	canceled := make(chan struct{})
	f.repo.EXPECT().StillStale(gomock.Any(), a).Return(true, nil)
	f.push.EXPECT().RequestPosition(gomock.Any(), a.DriverID, a.TripID).DoAndReturn(func(ctx context.Context, _, _ uuid.UUID) error {
		close(entered)
		<-ctx.Done()
		close(canceled)
		return ctx.Err()
	})
	f.w.queue.offer(&task{candidate: a})
	f.w.queue.offer(&task{candidate: b})
	f.w.Start(t.Context())
	wait(t, entered)
	f.w.Cancel(t.Context(), b.TripID)
	f.w.Cancel(t.Context(), a.TripID)
	wait(t, canceled)
	stop(t, f.w)
	if got := resultCounters(t, f.reader)["tripgo.position_requests/cancelled"]; got != 1 {
		t.Fatalf("queued cancellations = %d, want 1", got)
	}
	if f.w.queue.size() != 0 {
		t.Fatal("cancel left queued work")
	}
}

func TestRecheckSkipsChangedTripAndLinksTrace(t *testing.T) {
	f := setup(t, 5, 1)
	c := candidate(f.clock.Now().Add(-time.Hour))
	f.repo.EXPECT().FindStale(gomock.Any(), gomock.Any(), nil, 100).Return([]trip.StaleTrip{c}, nil)
	f.w.scan(t.Context())
	f.repo.EXPECT().StillStale(gomock.Any(), c).Return(false, nil)
	f.w.Start(t.Context())
	stop(t, f.w)
	spans := f.spans.Ended()
	if len(spans) != 2 {
		t.Fatalf("spans=%d", len(spans))
	}
	scan, request := spans[0], spans[1]
	if request.Parent().IsValid() || len(request.Links()) != 1 || !request.Links()[0].SpanContext.Equal(scan.SpanContext()) {
		t.Fatal("worker must be a linked root span")
	}
}

func TestTickerDoesNotOverlapAndCancellationStopsScanner(t *testing.T) {
	f := setup(t, 5, 1)
	entered := make(chan struct{})
	f.repo.EXPECT().FindStale(gomock.Any(), gomock.Any(), nil, 100).DoAndReturn(func(ctx context.Context, _ time.Time, _ *trip.StaleTrip, _ int) ([]trip.StaleTrip, error) {
		close(entered)
		<-ctx.Done()
		return nil, ctx.Err()
	}).Times(1)
	ctx, cancel := context.WithCancel(t.Context())
	f.w.Start(ctx)
	f.clock.Advance(30 * time.Second)
	wait(t, entered)
	f.clock.Advance(3 * time.Minute)
	cancel()
	stop(t, f.w)
}

func TestScanQueryError(t *testing.T) {
	tests := []struct {
		name       string
		cancelScan bool
		wantStatus codes.Code
	}{
		{name: "scanner stopped during query", cancelScan: true, wantStatus: codes.Unset},
		{name: "database query timed out", wantStatus: codes.Error},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			f := setup(t, 5, 1)
			var errorLogs bytes.Buffer
			f.w.logger = slog.New(slog.NewTextHandler(&errorLogs, &slog.HandlerOptions{Level: slog.LevelError}))
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			f.repo.EXPECT().FindStale(gomock.Any(), gomock.Any(), nil, 100).DoAndReturn(func(queryCtx context.Context, _ time.Time, _ *trip.StaleTrip, _ int) ([]trip.StaleTrip, error) {
				if tc.cancelScan {
					cancel()
					return nil, queryCtx.Err()
				}
				return nil, context.DeadlineExceeded
			})

			f.w.scan(ctx)

			spans := f.spans.Ended()
			if len(spans) != 1 || spans[0].Status().Code != tc.wantStatus {
				t.Fatalf("expected one scan span with status %v, got %v", tc.wantStatus, spans)
			}
			if got := errorLogs.Len() > 0; got != (tc.wantStatus == codes.Error) {
				t.Fatalf("unexpected error log: %q", errorLogs.String())
			}
		})
	}
}

func TestShutdownDeadlineCancelsWorkAndDropsPending(t *testing.T) {
	f := setup(t, 5, 1)
	entered := make(chan struct{})
	f.repo.EXPECT().StillStale(gomock.Any(), gomock.Any()).Return(true, nil).Times(1)
	f.push.EXPECT().RequestPosition(gomock.Any(), gomock.Any(), gomock.Any()).DoAndReturn(func(ctx context.Context, _, _ uuid.UUID) error {
		close(entered)
		<-ctx.Done()
		return ctx.Err()
	})
	for i := range 3 {
		f.w.queue.offer(&task{candidate: candidate(f.clock.Now().Add(time.Duration(i) * time.Second))})
	}
	f.w.Start(t.Context())
	wait(t, entered)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := f.w.Shutdown(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("shutdown: %v", err)
	}
	if f.w.active.Load() != 0 || f.w.queue.size() != 0 {
		t.Fatal("work remained after shutdown")
	}
}

func TestPriorityOrder(t *testing.T) {
	q := newQueue(3)
	now := time.Now()
	oldest := &task{candidate: candidate(now.Add(-3 * time.Hour))}
	middle := &task{candidate: candidate(now.Add(-2 * time.Hour))}
	newest := &task{candidate: candidate(now.Add(-time.Hour))}
	q.offer(newest)
	q.offer(oldest)
	q.offer(middle)
	for _, want := range []*task{oldest, middle, newest} {
		got, _ := q.take(t.Context(), time.Second)
		if got != want {
			t.Fatal("queue did not return oldest first")
		}
		q.finish(got)
	}
}

func TestPushMetricsMeasureOnlyHTTP(t *testing.T) {
	tests := []struct {
		result string
		err    error
	}{
		{result: "success"},
		{result: "error", err: errors.New("push unavailable")},
	}

	for _, tc := range tests {
		t.Run(tc.result, func(t *testing.T) {
			f := setup(t, 5, 1)
			staleTrip := candidate(f.clock.Now().Add(-time.Hour))
			f.repo.EXPECT().StillStale(gomock.Any(), staleTrip).DoAndReturn(func(context.Context, trip.StaleTrip) (bool, error) {
				f.clock.Advance(2 * time.Second)
				return true, nil
			})
			f.push.EXPECT().RequestPosition(gomock.Any(), staleTrip.DriverID, staleTrip.TripID).DoAndReturn(func(context.Context, uuid.UUID, uuid.UUID) error {
				f.clock.Advance(250 * time.Millisecond)
				return tc.err
			})

			f.w.process(t.Context(), &task{candidate: staleTrip})

			var requestsFound, durationFound bool
			for _, scope := range collect(t, f.reader).ScopeMetrics {
				for _, metric := range scope.Metrics {
					switch metric.Name {
					case "tripgo.position_requests":
						requestsFound = true
						data := metric.Data.(metricdata.Sum[int64])
						if len(data.DataPoints) != 1 || data.DataPoints[0].Value != 1 {
							t.Fatalf("requests: %+v", data.DataPoints)
						}
						result, _ := data.DataPoints[0].Attributes.Value(attribute.Key("result"))
						if result.AsString() != tc.result {
							t.Fatalf("request result = %q, want %q", result.AsString(), tc.result)
						}
					case "tripgo.position_requests.duration":
						durationFound = true
						data := metric.Data.(metricdata.Histogram[float64])
						if len(data.DataPoints) != 1 || data.DataPoints[0].Count != 1 || data.DataPoints[0].Sum != 0.25 {
							t.Fatalf("HTTP duration: %+v", data.DataPoints)
						}
						result, _ := data.DataPoints[0].Attributes.Value(attribute.Key("result"))
						if result.AsString() != tc.result {
							t.Fatalf("duration result = %q, want %q", result.AsString(), tc.result)
						}
					}
				}
			}
			if !requestsFound || !durationFound {
				t.Fatal("missing Push request metrics")
			}
		})
	}
}

func TestSkippedPushDoesNotRecordRequestMetrics(t *testing.T) {
	tests := []struct {
		name        string
		err         error
		cancelCheck bool
		errorType   string
	}{
		{
			name:      "database error",
			err:       errors.New("database unavailable"),
			errorType: "database",
		},
		{name: "trip changed"},
		{
			name:        "canceled check",
			err:         context.Canceled,
			cancelCheck: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			f := setup(t, 5, 1)
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			staleTrip := candidate(f.clock.Now().Add(-time.Hour))
			f.repo.EXPECT().StillStale(gomock.Any(), staleTrip).DoAndReturn(func(context.Context, trip.StaleTrip) (bool, error) {
				f.clock.Advance(2 * time.Second)
				if tc.cancelCheck {
					cancel()
				}
				return false, tc.err
			})

			f.w.process(ctx, &task{candidate: staleTrip})

			for _, scope := range collect(t, f.reader).ScopeMetrics {
				for _, metric := range scope.Metrics {
					switch metric.Name {
					case "tripgo.position_requests":
						data := metric.Data.(metricdata.Sum[int64])
						if len(data.DataPoints) != 0 {
							t.Fatalf("skipped Push recorded requests: %+v", data.DataPoints)
						}
					case "tripgo.position_requests.duration":
						data := metric.Data.(metricdata.Histogram[float64])
						if len(data.DataPoints) != 0 {
							t.Fatalf("skipped Push recorded duration: %+v", data.DataPoints)
						}
					}
				}
			}

			spans := f.spans.Ended()
			if len(spans) != 1 {
				t.Fatalf("spans = %d, want 1", len(spans))
			}
			var errorType string
			for _, attr := range spans[0].Attributes() {
				if attr.Key == "error.type" {
					errorType = attr.Value.AsString()
				}
			}
			if errorType != tc.errorType {
				t.Fatalf("span error.type = %q, want %q", errorType, tc.errorType)
			}
			wantStatus := codes.Unset
			if tc.errorType != "" {
				wantStatus = codes.Error
			}
			if spans[0].Status().Code != wantStatus {
				t.Fatalf("span status = %v, want %v", spans[0].Status().Code, wantStatus)
			}
		})
	}
}
