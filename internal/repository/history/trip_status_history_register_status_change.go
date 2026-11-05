package pg

import (
	"context"
	"fmt"

	sq "github.com/Masterminds/squirrel"

	"github.com/makkssik/template/internal/domain/trip"
	"github.com/makkssik/template/internal/infra/postgres"
)

func (r *statusHistoryRepository) RegisterStatusChange(ctx context.Context, change trip.StatusChange) error {
	model := domainToDB(change)

	query, args, err := sq.Insert("trip_status_history").
		Columns("trip_id", "from_status", "to_status", "reason", "changed_at").
		Values(model.TripID, model.FromStatus, model.ToStatus, model.Reason, model.ChangedAt).
		PlaceholderFormat(sq.Dollar).
		ToSql()
	if err != nil {
		return fmt.Errorf("build register status change query: %w", err)
	}

	ctx, cancel := context.WithTimeout(ctx, r.queryTimeout)
	defer cancel()
	if _, err := postgres.ExecutorFromContext(ctx, r.pool).Exec(ctx, query, args...); err != nil {
		return fmt.Errorf("register status change: %w", err)
	}
	return nil
}
