package pg

import (
	"context"
	"fmt"

	sq "github.com/Masterminds/squirrel"

	"github.com/makkssik/template/internal/domain/trip"
	"github.com/makkssik/template/internal/infra/postgres"
)

func (t *tripsRepository) CountActive(ctx context.Context) (int64, error) {
	query, args, err := sq.Select("count(*)").From("trips").
		Where(sq.Eq{"status": trip.StatusActive}).PlaceholderFormat(sq.Dollar).ToSql()
	if err != nil {
		return 0, fmt.Errorf("build count active trips: %w", err)
	}
	ctx, cancel := context.WithTimeout(ctx, t.queryTimeout)
	defer cancel()
	var count int64
	if err := postgres.ExecutorFromContext(ctx, t.pool).QueryRow(ctx, query, args...).Scan(&count); err != nil {
		return 0, fmt.Errorf("count active trips: %w", err)
	}
	return count, nil
}
