package trip

import (
	"context"
	"fmt"
	"time"

	sq "github.com/Masterminds/squirrel"
	"github.com/google/uuid"

	"github.com/makkssik/template/internal/infra/postgres"
)

func (t *tripsRepository) UpdateLastPosition(ctx context.Context, id uuid.UUID, at time.Time) error {
	ctx, cancel := context.WithTimeout(ctx, t.queryTimeout)
	defer cancel()
	query, args, err := sq.Update("trips").
		Set("last_position_at", sq.Expr("GREATEST(last_position_at, ?)", at)).
		Set("updated_at", sq.Expr("clock_timestamp()")).
		Where(sq.Eq{"id": id}).
		PlaceholderFormat(sq.Dollar).
		ToSql()
	if err != nil {
		return fmt.Errorf("build last position update: %w", err)
	}
	_, err = postgres.ExecutorFromContext(ctx, t.pool).Exec(ctx, query, args...)
	if err != nil {
		return fmt.Errorf("update last position: %w", err)
	}
	return nil
}
