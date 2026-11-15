package trip

import (
	"encoding/json"
	"testing"
	"time"
)

func TestDBToDomainTimestamps(t *testing.T) {
	zone := time.FixedZone("UTC+3", 3*60*60)
	started := time.Date(2026, 11, 15, 17, 37, 13, 494127000, zone)
	finished := started.Add(10 * time.Minute)
	for _, tc := range []struct {
		name       string
		finishedAt *time.Time
		wantJSON   string
	}{
		{"active", nil, "null"},
		{"completed", &finished, `"2026-11-15T14:47:13.494127Z"`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := dbToDomain(Trip{
				StartedAt: started, CreatedAt: started, UpdatedAt: started,
				FinishedAt: tc.finishedAt,
			})
			for name, at := range map[string]time.Time{
				"started_at": got.StartedAt, "created_at": got.CreatedAt, "updated_at": got.UpdatedAt,
			} {
				if !at.Equal(started) || at.Location() != time.UTC {
					t.Errorf("%s must preserve the instant in UTC: got %v, source %v", name, at, started)
				}
				data, err := json.Marshal(at)
				if err != nil {
					t.Fatal(err)
				}
				if string(data) != `"2026-11-15T14:37:13.494127Z"` {
					t.Errorf("unexpected %s JSON: %s", name, data)
				}
			}
			data, err := json.Marshal(got.FinishedAt)
			if err != nil {
				t.Fatal(err)
			}
			if string(data) != tc.wantJSON {
				t.Errorf("finished_at JSON: got %s, want %s", data, tc.wantJSON)
			}
			if finished.Location() != zone {
				t.Fatal("conversion modified the source timestamp")
			}
		})
	}
}
