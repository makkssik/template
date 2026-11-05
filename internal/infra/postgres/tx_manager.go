package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type TxManager struct {
	pool         *pgxpool.Pool
	queryTimeout time.Duration
}

func NewTxManager(pool *pgxpool.Pool, queryTimeout time.Duration) *TxManager {
	return &TxManager{pool: pool, queryTimeout: queryTimeout}
}

func (m *TxManager) Do(ctx context.Context, fn func(context.Context) error) (err error) {
	if _, ok := ctx.Value(txKey{}).(pgx.Tx); ok {
		return fn(ctx)
	}

	beginCtx, cancel := context.WithTimeout(ctx, m.queryTimeout)
	tx, err := m.pool.BeginTx(beginCtx, pgx.TxOptions{IsoLevel: pgx.ReadCommitted})
	cancel()
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}

	defer func() {
		rollbackCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), m.queryTimeout)
		defer cancel()
		if rollbackErr := tx.Rollback(rollbackCtx); rollbackErr != nil && !errors.Is(rollbackErr, pgx.ErrTxClosed) {
			err = errors.Join(err, fmt.Errorf("rollback transaction: %w", rollbackErr))
		}
	}()

	if err := fn(context.WithValue(ctx, txKey{}, tx)); err != nil {
		return fmt.Errorf("transaction callback: %w", err)
	}

	commitCtx, cancel := context.WithTimeout(ctx, m.queryTimeout)
	defer cancel()
	if err := tx.Commit(commitCtx); err != nil {
		return fmt.Errorf("commit transaction: %w", err)
	}
	return nil
}
