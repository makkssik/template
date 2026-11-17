//go:generate go tool mockgen -source contract.go -destination mocks_test.go -package positions
package positions

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/jonboulle/clockwork"

	"github.com/makkssik/template/internal/domain/trip"
)

type repository interface {
	FindStale(context.Context, time.Time, *trip.StaleTrip, int) ([]trip.StaleTrip, error)
	StillStale(context.Context, trip.StaleTrip) (bool, error)
}

type gateway interface {
	RequestPosition(context.Context, uuid.UUID, uuid.UUID) error
}

type clock interface {
	Now() time.Time
	NewTicker(time.Duration) clockwork.Ticker
}
