package pg

import (
	"context"
	"errors"
	"fmt"

	sq "github.com/Masterminds/squirrel"
	"github.com/jackc/pgx/v5/pgconn"

	trips "github.com/makkssik/template/internal/domain/trip"
	"github.com/makkssik/template/internal/infra/postgres"
)

func (t *tripsRepository) CreateTrip(ctx context.Context, trip trips.Trip) error {
	query, args, err := sq.Insert("trips").
		Columns("id", "user_id", "driver_id", "start_latitude", "start_longitude",
			"end_latitude", "end_longitude", "price", "status", "started_at",
			"finished_at", "created_at", "updated_at").
		Values(trip.TripID, trip.UserID, trip.DriverID,
			trip.StartPoint.Latitude, trip.StartPoint.Longitude,
			trip.EndPoint.Latitude, trip.EndPoint.Longitude,
			trip.Price, string(trip.Status), trip.StartedAt, trip.FinishedAt,
			trip.CreatedAt, trip.UpdatedAt).
		PlaceholderFormat(sq.Dollar).
		ToSql()
	if err != nil {
		return fmt.Errorf("build create trip query: %w", err)
	}

	ctx, cancel := context.WithTimeout(ctx, t.queryTimeout)
	defer cancel()
	_, err = postgres.ExecutorFromContext(ctx, t.pool).Exec(ctx, query, args...)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" && pgErr.ConstraintName == "trips_active_driver_uidx" {
			return fmt.Errorf("create trip: %w: %w", trips.ErrDriverBusy, err)
		}
		return fmt.Errorf("create trip: %w", err)
	}
	return nil
}
