//go:build integration

package trip_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	domaintrip "github.com/makkssik/template/internal/domain/trip"
	"github.com/makkssik/template/internal/infra/postgres"
	positionrepo "github.com/makkssik/template/internal/repository/position"
	"github.com/makkssik/template/internal/repository/trip"
	positioncreate "github.com/makkssik/template/internal/usecase/position/create"
)

type positionObserver struct {
	mu              sync.Mutex
	saved, canceled int
}

func (o *positionObserver) Cancel(context.Context, uuid.UUID) {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.canceled++
}

func (o *positionObserver) Saved(context.Context, uuid.UUID) {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.saved++
}

func positionService(pool *pgxpool.Pool, o *positionObserver) *positioncreate.Usecase {
	return positioncreate.New(trip.NewPostgresqlRepository(pool, 5*time.Second), positionrepo.NewPostgresqlRepository(pool, 5*time.Second), postgres.NewTxManager(pool, 5*time.Second), o, o)
}

func TestPositionAtomicityOrderingAndLateArrival(t *testing.T) {
	pool := database(t, nil)
	trips := trip.NewPostgresqlRepository(pool, 5*time.Second)
	points := positionrepo.NewPostgresqlRepository(pool, 5*time.Second)
	o := &positionObserver{}
	service := positionService(pool, o)
	value := newTrip()
	value.StartedAt = time.Now().Add(-time.Hour).UTC().Truncate(time.Microsecond)
	if err := trips.CreateTrip(t.Context(), value); err != nil {
		t.Fatal(err)
	}
	latest := time.Now().UTC().Truncate(time.Microsecond)
	coordinates := domaintrip.Coordinates{Latitude: 59.93, Longitude: 30.33}
	for _, at := range []time.Time{latest, latest.Add(-time.Minute), latest} {
		point, err := service.CreatePosition(t.Context(), positioncreate.Input{
			TripID:      value.TripID,
			Coordinates: coordinates,
			RecordedAt:  at,
		})
		if err != nil {
			t.Fatal(err)
		}
		if point.TripID != value.TripID || point.Coordinates != coordinates || point.RecordedAt != at || point.RecordedAt.Location() != time.UTC {
			t.Fatalf("saved position mapping: %+v", point)
		}
	}
	loaded, err := trips.GetTripByID(t.Context(), value.TripID)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.LastPositionAt == nil || *loaded.LastPositionAt != latest {
		t.Fatalf("late point moved timestamp: %+v", loaded.LastPositionAt)
	}
	route, err := points.GetPositionsByTripID(t.Context(), value.TripID)
	if err != nil {
		t.Fatal(err)
	}
	if len(route) != 3 || route[0].RecordedAt != latest.Add(-time.Minute) || route[1].ID >= route[2].ID {
		t.Fatalf("route ordering: %+v", route)
	}
	for _, point := range route {
		if point.TripID != value.TripID || point.Coordinates != coordinates || point.RecordedAt.Location() != time.UTC {
			t.Fatalf("route position mapping: %+v", point)
		}
	}
	if o.saved != 3 || o.canceled != 3 {
		t.Fatal("missing post-commit notification")
	}
	if _, err := pool.Exec(t.Context(), "ALTER TABLE trips ADD CONSTRAINT reject_position_update CHECK (last_position_at IS NULL) NOT VALID"); err != nil {
		t.Fatal(err)
	}
	if got, err := service.CreatePosition(t.Context(), positioncreate.Input{TripID: value.TripID, RecordedAt: latest.Add(time.Second)}); err == nil || got != nil {
		t.Fatal("expected rollback")
	}
	route, err = points.GetPositionsByTripID(t.Context(), value.TripID)
	if err != nil {
		t.Fatal(err)
	}
	if len(route) != 3 || o.saved != 3 || o.canceled != 3 {
		t.Fatal("rollback leaked row or notifications")
	}
	loaded, err = trips.GetTripByID(t.Context(), value.TripID)
	if err != nil {
		t.Fatal(err)
	}
	if *loaded.LastPositionAt != latest {
		t.Fatal("rollback changed last_position_at")
	}
}

func TestStaleCursorAndRecheck(t *testing.T) {
	pool := database(t, nil)
	repo := trip.NewPostgresqlRepository(pool, 5*time.Second)
	now := time.Now().UTC().Truncate(time.Microsecond)
	cutoff := now.Add(-time.Minute)
	ids := make(map[uuid.UUID]bool)
	for range 250 {
		value := newTrip()
		value.StartedAt = now.Add(-time.Hour)
		if err := repo.CreateTrip(t.Context(), value); err != nil {
			t.Fatal(err)
		}
		ids[value.TripID] = true
	}
	excludedTrips := []struct {
		startedAt      time.Time
		finishedAt     *time.Time
		lastPositionAt *time.Time
	}{
		{startedAt: now},
		{startedAt: now.Add(-time.Hour), finishedAt: &now},
		{startedAt: now.Add(-time.Hour), lastPositionAt: &now},
	}
	for _, excluded := range excludedTrips {
		value := newTrip()
		value.StartedAt = excluded.startedAt
		if excluded.finishedAt != nil {
			value.Status = domaintrip.StatusCompleted
			value.FinishedAt = excluded.finishedAt
		}
		if err := repo.CreateTrip(t.Context(), value); err != nil {
			t.Fatal(err)
		}
		if excluded.lastPositionAt != nil {
			if err := repo.UpdateLastPosition(t.Context(), value.TripID, *excluded.lastPositionAt); err != nil {
				t.Fatal(err)
			}
		}
	}
	var cursor *domaintrip.StaleTrip
	all := make([]domaintrip.StaleTrip, 0, 250)
	for range 3 {
		batch, err := repo.FindStale(t.Context(), cutoff, cursor, 100)
		if err != nil {
			t.Fatal(err)
		}
		if len(batch) == 0 {
			t.Fatal("premature end")
		}
		all = append(all, batch...)
		last := batch[len(batch)-1]
		cursor = &last
	}
	if len(all) != 250 {
		t.Fatalf("got %d trips", len(all))
	}
	for _, v := range all {
		if !ids[v.TripID] {
			t.Fatal("duplicate or non-stale trip")
		}
		delete(ids, v.TripID)
	}
	if !sort.SliceIsSorted(all, func(i, j int) bool {
		return all[i].TripID.String() < all[j].TripID.String()
	}) {
		t.Fatal("unstable cursor tie order")
	}
	if batch, err := repo.FindStale(t.Context(), cutoff, cursor, 100); err != nil || len(batch) != 0 {
		t.Fatalf("cursor tail: %v %v", batch, err)
	}
	first := all[0]
	if ok, err := repo.StillStale(t.Context(), first); err != nil || !ok {
		t.Fatalf("stale recheck %v %v", ok, err)
	}
	if err := repo.UpdateLastPosition(t.Context(), first.TripID, now); err != nil {
		t.Fatal(err)
	}
	if ok, err := repo.StillStale(t.Context(), first); err != nil || ok {
		t.Fatal("new coordinate failed to invalidate snapshot")
	}
}

func TestPositionAndFinishConcurrency(t *testing.T) {
	pool := database(t, nil)
	repo := trip.NewPostgresqlRepository(pool, 5*time.Second)
	tx := postgres.NewTxManager(pool, 5*time.Second)
	service := positionService(pool, &positionObserver{})
	value := newTrip()
	if err := repo.CreateTrip(t.Context(), value); err != nil {
		t.Fatal(err)
	}
	start := make(chan struct{})
	results := make(chan error, 2)
	go func() {
		<-start
		_, err := service.CreatePosition(t.Context(), positioncreate.Input{TripID: value.TripID, RecordedAt: time.Now().UTC()})
		results <- err
	}()
	go func() {
		<-start
		results <- tx.Do(t.Context(), func(ctx context.Context) error {
			v, err := repo.GetTripForUpdate(ctx, value.TripID)
			if err != nil {
				return err
			}
			if err := v.Finish(time.Now().UTC()); err != nil {
				return err
			}
			_, err = repo.SaveTrip(ctx, *v)
			return err
		})
	}()
	close(start)
	for range 2 {
		if err := <-results; err != nil && !errors.Is(err, domaintrip.ErrTripCompleted) {
			t.Fatal(err)
		}
	}
	if _, err := service.CreatePosition(t.Context(), positioncreate.Input{TripID: value.TripID, RecordedAt: time.Now()}); !errors.Is(err, domaintrip.ErrTripCompleted) {
		t.Fatalf("completed trip accepted point: %v", err)
	}
	if _, err := service.CreatePosition(t.Context(), positioncreate.Input{TripID: uuid.New(), RecordedAt: time.Now()}); !errors.Is(err, domaintrip.ErrTripNotFound) {
		t.Fatalf("missing trip: %v", err)
	}
}

func TestConcurrentPositionsKeepNewestTimestamp(t *testing.T) {
	pool := database(t, nil)
	repo := trip.NewPostgresqlRepository(pool, 5*time.Second)
	service := positionService(pool, &positionObserver{})
	value := newTrip()
	if err := repo.CreateTrip(t.Context(), value); err != nil {
		t.Fatal(err)
	}
	latest := time.Now().UTC().Truncate(time.Microsecond)
	start := make(chan struct{})
	results := make(chan error, 20)
	for i := range 20 {
		go func() {
			<-start
			_, err := service.CreatePosition(t.Context(), positioncreate.Input{TripID: value.TripID, RecordedAt: latest.Add(-time.Duration(i) * time.Second)})
			results <- err
		}()
	}
	close(start)
	for range 20 {
		if err := <-results; err != nil {
			t.Fatal(err)
		}
	}
	loaded, err := repo.GetTripByID(t.Context(), value.TripID)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.LastPositionAt == nil || *loaded.LastPositionAt != latest {
		t.Fatal("concurrent older point moved last_position_at")
	}
	var count int
	if err := pool.QueryRow(t.Context(), "SELECT count(*) FROM trip_positions WHERE trip_id=$1", value.TripID).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 20 {
		t.Fatalf("lost positions: %d", count)
	}
}

func TestStaleIndexPlan(t *testing.T) {
	pool := database(t, nil)
	_, err := pool.Exec(t.Context(), `INSERT INTO trips (id,user_id,driver_id,start_latitude,start_longitude,end_latitude,end_longitude,price,status,started_at)
 SELECT gen_random_uuid(),gen_random_uuid(),gen_random_uuid(),0,0,0,0,0,'active',now() - CASE WHEN i<=300 THEN interval '1 hour' ELSE interval '1 second' END
 FROM generate_series(1,10000) i`)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(t.Context(), "ANALYZE trips"); err != nil {
		t.Fatal(err)
	}
	cutoff := time.Now().Add(-time.Minute)
	repo := trip.NewPostgresqlRepository(pool, 5*time.Second)
	first, err := repo.FindStale(t.Context(), cutoff, nil, 100)
	if err != nil || len(first) != 100 {
		t.Fatalf("first batch: rows=%d err=%v", len(first), err)
	}
	cursor := first[len(first)-1]
	rows, err := pool.Query(t.Context(), `EXPLAIN (ANALYZE, BUFFERS) SELECT id,driver_id,COALESCE(last_position_at,started_at) FROM trips WHERE status='active' AND COALESCE(last_position_at,started_at)<$1 AND (COALESCE(last_position_at,started_at),id)>($2,$3) ORDER BY COALESCE(last_position_at,started_at),id LIMIT 100`, cutoff, cursor.LastSeen, cursor.TripID)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var lines []string
	for rows.Next() {
		var line string
		if err := rows.Scan(&line); err != nil {
			t.Fatal(err)
		}
		lines = append(lines, line)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	plan := strings.Join(lines, "\n")
	t.Log(plan)
	if !strings.Contains(plan, "Index Scan using trips_active_position_idx") && !strings.Contains(plan, "Index Only Scan using trips_active_position_idx") {
		t.Fatal("planner did not use stale index")
	}
}

func TestPositionMigrationsDownAndUp(t *testing.T) {
	pool := database(t, nil)
	for _, name := range []string{"00004_stale_trips_index.sql", "00003_create_trip_positions.sql"} {
		contents, err := os.ReadFile(filepath.Join("../../../migrations", name))
		if err != nil {
			t.Fatal(err)
		}
		_, down, _ := strings.Cut(string(contents), "-- +goose Down")
		if _, err := pool.Exec(t.Context(), down); err != nil {
			t.Fatal(err)
		}
	}
	for _, name := range []string{"00003_create_trip_positions.sql", "00004_stale_trips_index.sql"} {
		contents, err := os.ReadFile(filepath.Join("../../../migrations", name))
		if err != nil {
			t.Fatal(err)
		}
		up, _, _ := strings.Cut(string(contents), "-- +goose Down")
		if _, err := pool.Exec(t.Context(), up); err != nil {
			t.Fatal(err)
		}
	}
}
