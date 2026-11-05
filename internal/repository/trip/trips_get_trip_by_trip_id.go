package pg

import (
	"context"
	"errors"
	"fmt"

	sq "github.com/Masterminds/squirrel"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/samber/lo"

	trips "github.com/makkssik/template/internal/domain/trip"
	"github.com/makkssik/template/internal/infra/postgres"
)

func (t *tripsRepository) GetTripByID(ctx context.Context, tripID uuid.UUID) (*trips.Trip, error) {
	return t.getTripByID(ctx, tripID, false)
}

func (t *tripsRepository) GetTripForUpdate(ctx context.Context, tripID uuid.UUID) (*trips.Trip, error) {
	if _, ok := postgres.ExecutorFromContext(ctx, t.pool).(pgx.Tx); !ok {
		return nil, errors.New("get trip for update requires a transaction")
	}
	return t.getTripByID(ctx, tripID, true)
}

func (t *tripsRepository) getTripByID(ctx context.Context, tripID uuid.UUID, forUpdate bool) (*trips.Trip, error) {
	queryBuilder := sq.Select(tripColumns).
		From("trips").
		Where(sq.Eq{"id": tripID}).
		PlaceholderFormat(sq.Dollar)
	if forUpdate {
		queryBuilder = queryBuilder.Suffix("FOR UPDATE")
	}
	query, args, err := queryBuilder.ToSql()
	if err != nil {
		return nil, fmt.Errorf("build get trip query: %w", err)
	}

	ctx, cancel := context.WithTimeout(ctx, t.queryTimeout)
	defer cancel()
	rows, err := postgres.ExecutorFromContext(ctx, t.pool).Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("get trip: %w", err)
	}

	model, err := pgx.CollectExactlyOneRow(rows, pgx.RowToStructByName[Trip])
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, fmt.Errorf("get trip: %w", trips.ErrTripNotFound)
	}
	if err != nil {
		return nil, fmt.Errorf("scan trip: %w", err)
	}
	return lo.ToPtr(dbToDomain(model)), nil
}
