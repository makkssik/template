package pg

import (
	"time"

	"github.com/google/uuid"

	"github.com/makkssik/template/internal/domain/trip"
)

type StatusChange struct {
	ID         int64     `db:"id"`
	TripID     uuid.UUID `db:"trip_id"`
	FromStatus *string   `db:"from_status"`
	ToStatus   string    `db:"to_status"`
	Reason     *string   `db:"reason"`
	ChangedAt  time.Time `db:"changed_at"`
}

func domainToDB(change trip.StatusChange) StatusChange {
	model := StatusChange{
		TripID:    change.TripID,
		ToStatus:  string(change.ToStatus),
		Reason:    change.Reason,
		ChangedAt: change.ChangedAt,
	}
	if change.FromStatus != nil {
		status := string(*change.FromStatus)
		model.FromStatus = &status
	}
	return model
}
