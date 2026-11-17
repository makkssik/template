package position_test

import (
	"math"
	"testing"
	"time"

	"github.com/makkssik/template/internal/domain/position"
	"github.com/makkssik/template/internal/domain/trip"
)

func TestValidation(t *testing.T) {
	now := time.Now()
	tests := []struct {
		name  string
		lat   float64
		lon   float64
		at    time.Time
		valid bool
	}{
		{
			name:  "zero coordinates",
			lat:   0,
			lon:   0,
			at:    now,
			valid: true,
		},
		{
			name:  "boundaries",
			lat:   90,
			lon:   -180,
			at:    now.Add(time.Minute),
			valid: true,
		},
		{
			name:  "old point",
			lat:   -90,
			lon:   180,
			at:    now.Add(-time.Hour),
			valid: true,
		},
		{
			name:  "latitude",
			lat:   91,
			lon:   0,
			at:    now,
			valid: false,
		},
		{
			name:  "longitude",
			lat:   0,
			lon:   -181,
			at:    now,
			valid: false,
		},
		{
			name:  "nan",
			lat:   math.NaN(),
			lon:   0,
			at:    now,
			valid: false,
		},
		{
			name:  "nan longitude",
			lat:   0,
			lon:   math.NaN(),
			at:    now,
			valid: false,
		},
		{
			name:  "future",
			lat:   0,
			lon:   0,
			at:    now.Add(time.Minute + time.Nanosecond),
			valid: false,
		},
		{
			name:  "missing time",
			lat:   0,
			lon:   0,
			at:    time.Time{},
			valid: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := position.Validate(trip.Coordinates{Latitude: tc.lat, Longitude: tc.lon}, tc.at, now)
			if (err == nil) != tc.valid {
				t.Fatalf("valid=%v err=%v", tc.valid, err)
			}
		})
	}
}
