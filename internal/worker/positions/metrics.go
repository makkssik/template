package positions

import (
	"context"
	"errors"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
)

type Metrics struct {
	meter            metric.Meter
	checks, requests metric.Int64Counter
	duration         metric.Float64Histogram
	rows             metric.Int64Histogram
}

func NewMetrics(meter metric.Meter) (*Metrics, error) {
	checks, e1 := meter.Int64Counter("tripgo.stale_checks")
	requests, e2 := meter.Int64Counter("tripgo.position_requests")
	duration, e3 := meter.Float64Histogram("tripgo.position_requests.duration", metric.WithUnit("s"))
	rows, e4 := meter.Int64Histogram("tripgo.stale_checks.rows_read", metric.WithDescription("Rows read per scanner tick"))
	return &Metrics{meter: meter, checks: checks, requests: requests, duration: duration, rows: rows}, errors.Join(e1, e2, e3, e4)
}

func (m *Metrics) observe(size, active func() int64) error {
	_, e1 := m.meter.Int64ObservableGauge("tripgo.position_requests.queue.size", metric.WithInt64Callback(func(_ context.Context, o metric.Int64Observer) error {
		o.Observe(size())
		return nil
	}))
	_, e2 := m.meter.Int64ObservableGauge("tripgo.position_requests.workers.active", metric.WithInt64Callback(func(_ context.Context, o metric.Int64Observer) error {
		o.Observe(active())
		return nil
	}))
	return errors.Join(e1, e2)
}

func resultOption(result string) metric.MeasurementOption {
	return metric.WithAttributes(attribute.String("result", result))
}

func (m *Metrics) check(ctx context.Context, result string) {
	m.checks.Add(ctx, 1, resultOption(result))
}

func (m *Metrics) request(ctx context.Context, result string) {
	m.requests.Add(ctx, 1, resultOption(result))
}
