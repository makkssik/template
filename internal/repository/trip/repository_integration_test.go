//go:build integration

package pg_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	trip "github.com/makkssik/template/internal/domain/trip"
	"github.com/makkssik/template/internal/infra/postgres"
	historypg "github.com/makkssik/template/internal/repository/history"
	trippg "github.com/makkssik/template/internal/repository/trip"
	"github.com/makkssik/template/internal/usecase/trip/finish"
)

func database(t *testing.T, tracer pgx.QueryTracer) *pgxpool.Pool {
	t.Helper()
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Fatal("TEST_DATABASE_URL must point to a PostgreSQL database for integration tests")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	admin, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	schema := "test_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	name := pgx.Identifier{schema}.Sanitize()
	if _, err = admin.Exec(ctx, "CREATE SCHEMA "+name); err != nil {
		admin.Close()
		t.Fatal(err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if _, err := admin.Exec(ctx, "DROP SCHEMA "+name+" CASCADE"); err != nil {
			t.Error(err)
		}
		admin.Close()
	})
	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		t.Fatal(err)
	}
	cfg.ConnConfig.RuntimeParams["search_path"] = schema
	cfg.ConnConfig.Tracer = tracer
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	files, err := filepath.Glob("../../../migrations/*.sql")
	if err != nil {
		t.Fatal(err)
	}
	if len(files) == 0 {
		t.Fatal("no migrations")
	}
	for _, file := range files {
		contents, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		up, _, _ := strings.Cut(string(contents), "-- +goose Down")
		if _, err := pool.Exec(ctx, up); err != nil {
			t.Fatal(err)
		}
	}
	return pool
}

func newTrip() trip.Trip {
	now := time.Now().UTC()
	return trip.Trip{TripID: uuid.New(), UserID: uuid.New(), DriverID: uuid.New(), Status: trip.StatusActive, StartedAt: now, CreatedAt: now, UpdatedAt: now}
}

func TestAtomicityAndNestedTransaction(t *testing.T) {
	pool := database(t, nil)
	repo := trippg.NewPostgresqlRepository(pool, 5*time.Second)
	history := historypg.NewPostgresqlRepository(pool, 5*time.Second)
	tx := postgres.NewTxManager(pool, 5*time.Second)
	ctx := context.Background()
	for _, panics := range []bool{false, true} {
		value := newTrip()
		func() {
			defer func() {
				v := recover()
				if panics && v == nil {
					t.Error("panic swallowed")
				}
				if !panics && v != nil {
					t.Errorf("unexpected panic %v", v)
				}
			}()
			err := tx.Do(ctx, func(ctx context.Context) error {
				if err := repo.CreateTrip(ctx, value); err != nil {
					return err
				}
				return tx.Do(ctx, func(ctx context.Context) error {
					if panics {
						panic("rollback")
					}
					return history.RegisterStatusChange(ctx, trip.StatusChange{TripID: value.TripID, ToStatus: trip.Status("invalid"), ChangedAt: time.Now()})
				})
			})
			if err == nil {
				t.Error("expected failure")
			}
		}()
		if _, err := repo.GetTripByID(ctx, value.TripID); !errors.Is(err, trip.ErrTripNotFound) {
			t.Fatalf("trip survived rollback: %v", err)
		}
	}
	value := newTrip()
	if err := tx.Do(ctx, func(ctx context.Context) error {
		if err := repo.CreateTrip(ctx, value); err != nil {
			return err
		}
		return tx.Do(ctx, func(ctx context.Context) error {
			return history.RegisterStatusChange(ctx, trip.StatusChange{TripID: value.TripID, ToStatus: trip.StatusActive, ChangedAt: value.StartedAt})
		})
	}); err != nil {
		t.Fatal(err)
	}
	got, err := repo.GetTripByID(ctx, value.TripID)
	if err != nil || got.TripID != value.TripID {
		t.Fatalf("committed trip missing: %v", err)
	}
}

func TestConcurrentCreateAndFinish(t *testing.T) {
	pool := database(t, nil)
	repo := trippg.NewPostgresqlRepository(pool, 5*time.Second)
	history := historypg.NewPostgresqlRepository(pool, 5*time.Second)
	tx := postgres.NewTxManager(pool, 5*time.Second)
	driver := uuid.New()
	var success, conflict atomic.Int32
	var winner uuid.UUID
	var mu sync.Mutex
	parallel := func(fn func()) {
		var wg sync.WaitGroup
		start := make(chan struct{})
		for range 20 {
			wg.Go(func() { <-start; fn() })
		}
		close(start)
		wg.Wait()
	}
	parallel(func() {
		value := newTrip()
		value.DriverID = driver
		err := tx.Do(context.Background(), func(ctx context.Context) error {
			if err := repo.CreateTrip(ctx, value); err != nil {
				return err
			}
			return history.RegisterStatusChange(ctx, trip.StatusChange{TripID: value.TripID, ToStatus: trip.StatusActive, ChangedAt: value.StartedAt})
		})
		switch {
		case err == nil:
			success.Add(1)
			mu.Lock()
			winner = value.TripID
			mu.Unlock()
		case errors.Is(err, trip.ErrDriverBusy):
			conflict.Add(1)
		default:
			t.Errorf("create: %v", err)
		}
	})
	if success.Load() != 1 || conflict.Load() != 19 {
		t.Fatalf("create successes=%d conflicts=%d", success.Load(), conflict.Load())
	}
	if count, err := repo.CountActive(t.Context()); err != nil || count != 1 {
		t.Fatalf("active trips after create: count=%d err=%v", count, err)
	}
	success.Store(0)
	conflict.Store(0)
	var winnerFinishedAt time.Time
	observer := &completionCounter{}
	finishTrip := finish.New(repo, history, tx, observer)
	parallel(func() {
		value, err := finishTrip.FinishTrip(t.Context(), winner)
		switch {
		case err == nil:
			success.Add(1)
			mu.Lock()
			winnerFinishedAt = *value.FinishedAt
			mu.Unlock()
		case errors.Is(err, trip.ErrTripCompleted):
			conflict.Add(1)
		default:
			t.Errorf("finish: %v", err)
		}
	})
	if success.Load() != 1 || conflict.Load() != 19 {
		t.Fatalf("finish successes=%d conflicts=%d", success.Load(), conflict.Load())
	}
	if observer.count.Load() != 1 {
		t.Fatalf("completion events=%d, want 1", observer.count.Load())
	}
	completed, err := repo.GetTripByID(t.Context(), winner)
	if err != nil {
		t.Fatal(err)
	}
	if completed.Status != trip.StatusCompleted || completed.FinishedAt == nil || winnerFinishedAt.IsZero() || !completed.FinishedAt.Equal(winnerFinishedAt) {
		t.Fatalf("finish result changed after concurrent calls: got %+v, winner finished at %v", completed, winnerFinishedAt)
	}
	if count, err := repo.CountActive(t.Context()); err != nil || count != 0 {
		t.Fatalf("active trips after finish: count=%d err=%v", count, err)
	}
	var count int
	if err := pool.QueryRow(context.Background(), "SELECT count(*) FROM trip_status_history WHERE trip_id=$1", winner).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 2 {
		t.Fatalf("history count %d", count)
	}
}

type queryRecorder struct {
	mu      sync.Mutex
	queries []string
}

func (r *queryRecorder) TraceQueryStart(ctx context.Context, _ *pgx.Conn, data pgx.TraceQueryStartData) context.Context {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.queries = append(r.queries, data.SQL)
	return ctx
}

func (*queryRecorder) TraceQueryEnd(context.Context, *pgx.Conn, pgx.TraceQueryEndData) {}

func (r *queryRecorder) reset() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.queries = nil
}

func (r *queryRecorder) statements() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	queries := make([]string, 0, len(r.queries))
	for _, query := range r.queries {
		statement := strings.ToUpper(strings.Join(strings.Fields(query), " "))
		operation, _, _ := strings.Cut(statement, " ")
		switch operation {
		case "BEGIN", "COMMIT", "ROLLBACK":
			continue
		}
		queries = append(queries, statement)
	}
	return queries
}

type completionRecorder struct{ tripID uuid.UUID }

func (r *completionRecorder) Completed(_ context.Context, id uuid.UUID) { r.tripID = id }

type completionCounter struct{ count atomic.Int32 }

func (r *completionCounter) Completed(context.Context, uuid.UUID) { r.count.Add(1) }

func TestFinishLocksAndSavesTrip(t *testing.T) {
	queries := &queryRecorder{}
	pool := database(t, queries)
	repo := trippg.NewPostgresqlRepository(pool, 5*time.Second)
	history := historypg.NewPostgresqlRepository(pool, 5*time.Second)
	tx := postgres.NewTxManager(pool, 5*time.Second)
	ctx := t.Context()
	value := newTrip()
	if got, err := repo.GetTripForUpdate(ctx, value.TripID); err == nil || got != nil {
		t.Fatalf("locking read outside a transaction must fail: trip=%+v err=%v", got, err)
	}
	if err := repo.CreateTrip(ctx, value); err != nil {
		t.Fatal(err)
	}
	if err := history.RegisterStatusChange(ctx, trip.StatusChange{TripID: value.TripID, ToStatus: trip.StatusActive, ChangedAt: value.StartedAt}); err != nil {
		t.Fatal(err)
	}
	observer := &completionRecorder{}
	usecase := finish.New(repo, history, tx, observer)
	queries.reset()
	completed, err := usecase.FinishTrip(ctx, value.TripID)
	if err != nil {
		t.Fatal(err)
	}
	statements := queries.statements()
	if len(statements) != 3 || !strings.HasPrefix(statements[0], "SELECT ") || !strings.HasSuffix(statements[0], " FOR UPDATE") || !strings.HasPrefix(statements[1], "UPDATE TRIPS ") || !strings.HasPrefix(statements[2], "INSERT INTO TRIP_STATUS_HISTORY ") {
		t.Fatalf("finish must lock the trip, save its state and append history: %v", statements)
	}
	if completed.TripID != value.TripID || completed.Status != trip.StatusCompleted || completed.FinishedAt == nil || observer.tripID != value.TripID {
		t.Fatalf("wrong completion: trip=%+v event=%v", completed, observer.tripID)
	}
	persisted, err := repo.GetTripByID(ctx, value.TripID)
	if err != nil {
		t.Fatal(err)
	}
	var changedAt time.Time
	if err := pool.QueryRow(ctx, "SELECT changed_at FROM trip_status_history WHERE trip_id=$1 AND to_status='completed'", value.TripID).Scan(&changedAt); err != nil {
		t.Fatal(err)
	}
	if !completed.FinishedAt.Equal(*persisted.FinishedAt) || !completed.UpdatedAt.Equal(persisted.UpdatedAt) || !changedAt.Equal(*completed.FinishedAt) {
		t.Fatalf("finish timestamp differs between result, trip and history: result=%+v persisted=%+v history=%v", completed, persisted, changedAt)
	}
}

func TestFinishRollbackAndConflicts(t *testing.T) {
	pool := database(t, nil)
	repo := trippg.NewPostgresqlRepository(pool, 5*time.Second)
	history := historypg.NewPostgresqlRepository(pool, 5*time.Second)
	tx := postgres.NewTxManager(pool, 5*time.Second)
	ctx := t.Context()
	value := newTrip()
	if err := repo.CreateTrip(ctx, value); err != nil {
		t.Fatal(err)
	}
	if err := history.RegisterStatusChange(ctx, trip.StatusChange{TripID: value.TripID, ToStatus: trip.StatusActive, ChangedAt: value.StartedAt}); err != nil {
		t.Fatal(err)
	}
	before, err := repo.GetTripByID(ctx, value.TripID)
	if err != nil {
		t.Fatal(err)
	}
	// This constraint exists only in the temporary schema and makes the history INSERT fail after the UPDATE.
	if _, err := pool.Exec(ctx, "ALTER TABLE trip_status_history ADD CONSTRAINT reject_completed CHECK (to_status <> 'completed')"); err != nil {
		t.Fatal(err)
	}
	observer := &completionRecorder{}
	usecase := finish.New(repo, history, tx, observer)
	if got, err := usecase.FinishTrip(ctx, value.TripID); err == nil || got != nil {
		t.Fatalf("history failure must fail completion: trip=%+v err=%v", got, err)
	}
	after, err := repo.GetTripByID(ctx, value.TripID)
	if err != nil {
		t.Fatal(err)
	}
	if after.Status != trip.StatusActive || after.FinishedAt != nil || !after.UpdatedAt.Equal(before.UpdatedAt) || observer.tripID != uuid.Nil {
		t.Fatalf("completion survived rollback: trip=%+v event=%v", after, observer.tripID)
	}
	var count int
	if err := pool.QueryRow(ctx, "SELECT count(*) FROM trip_status_history WHERE trip_id=$1", value.TripID).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("failed completion changed history: count=%d", count)
	}
	if _, err := pool.Exec(ctx, "ALTER TABLE trip_status_history DROP CONSTRAINT reject_completed"); err != nil {
		t.Fatal(err)
	}
	completed, err := usecase.FinishTrip(ctx, value.TripID)
	if err != nil {
		t.Fatal(err)
	}
	if got, err := usecase.FinishTrip(ctx, value.TripID); !errors.Is(err, trip.ErrTripCompleted) || got != nil {
		t.Fatalf("repeated completion: trip=%+v err=%v", got, err)
	}
	after, err = repo.GetTripByID(ctx, value.TripID)
	if err != nil {
		t.Fatal(err)
	}
	if after.FinishedAt == nil || !after.FinishedAt.Equal(*completed.FinishedAt) {
		t.Fatalf("repeated completion changed finish time: before=%+v after=%+v", completed, after)
	}
	if got, err := usecase.FinishTrip(ctx, uuid.New()); !errors.Is(err, trip.ErrTripNotFound) || got != nil {
		t.Fatalf("missing trip completion: trip=%+v err=%v", got, err)
	}
}
