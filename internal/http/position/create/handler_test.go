package create_test

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"go.uber.org/mock/gomock"

	"github.com/makkssik/template/internal/domain/position"
	"github.com/makkssik/template/internal/domain/trip"
	api "github.com/makkssik/template/internal/generated"
	"github.com/makkssik/template/internal/http/position/create"
	positioncreate "github.com/makkssik/template/internal/usecase/position/create"
)

const validPositionBody = `{"coordinates":{"latitude":0,"longitude":30},"recorded_at":"2026-01-01T00:00:00Z"}`

func TestCreatePositionHandler(t *testing.T) {
	tests := []struct {
		name       string
		body       string
		err        error
		wantStatus int
		wantCode   string
	}{
		{
			name:       "success",
			body:       validPositionBody,
			wantStatus: http.StatusCreated,
		},
		{
			name:       "trailing whitespace",
			body:       validPositionBody + " \n\t",
			wantStatus: http.StatusCreated,
		},
		{
			name:       "missing trip",
			body:       validPositionBody,
			err:        trip.ErrTripNotFound,
			wantStatus: http.StatusNotFound,
			wantCode:   "trip_not_found",
		},
		{
			name:       "completed",
			body:       validPositionBody,
			err:        trip.ErrTripCompleted,
			wantStatus: http.StatusConflict,
			wantCode:   "trip_completed",
		},
		{
			name:       "failure",
			body:       validPositionBody,
			err:        errors.New("secret SQL"),
			wantStatus: http.StatusInternalServerError,
			wantCode:   "internal_error",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			uc := NewMockusecase(ctrl)
			id := uuid.New()
			uc.EXPECT().CreatePosition(gomock.Any(), gomock.Any()).DoAndReturn(func(_ context.Context, p positioncreate.Input) (*position.Position, error) {
				if p.TripID != id || p.Coordinates.Latitude != 0 || p.Coordinates.Longitude != 30 {
					t.Fatal("wrong input")
				}
				return &position.Position{ID: 42, TripID: p.TripID, Coordinates: p.Coordinates, RecordedAt: p.RecordedAt}, tc.err
			})
			request := httptest.NewRequest(http.MethodPost, "/api/v1/trips/"+id.String()+"/positions", strings.NewReader(tc.body))
			request.Header.Set("Content-Type", "application/json")
			response := httptest.NewRecorder()
			create.New(uc, slog.New(slog.NewTextHandler(io.Discard, nil))).CreateTripPosition(response, request, id)
			if response.Code != tc.wantStatus {
				t.Fatalf("status %d body %s", response.Code, response.Body)
			}
			if tc.wantStatus == http.StatusCreated {
				var p api.TripPosition
				if err := json.Unmarshal(response.Body.Bytes(), &p); err != nil {
					t.Fatal(err)
				}
				if p.Id != 42 || p.TripId != id {
					t.Fatal("wrong response")
				}
			} else {
				var p api.Problem
				if err := json.Unmarshal(response.Body.Bytes(), &p); err != nil {
					t.Fatal(err)
				}
				if p.Code != tc.wantCode || strings.Contains(response.Body.String(), "secret SQL") {
					t.Fatalf("wrong problem: %s", response.Body)
				}
			}
		})
	}
}

func TestCreatePositionRejectsInvalidBody(t *testing.T) {
	tests := []struct {
		name string
		body string
	}{
		{name: "empty object", body: "{}"},
		{name: "null body", body: "null"},
		{name: "empty body", body: ""},
		{name: "array", body: "[]"},
		{name: "missing coordinates", body: `{"recorded_at":"2026-01-01T00:00:00Z"}`},
		{
			name: "null coordinates",
			body: `{"coordinates":null,"recorded_at":"2026-01-01T00:00:00Z"}`,
		},
		{name: "missing time", body: `{"coordinates":{"latitude":0,"longitude":30}}`},
		{
			name: "null time",
			body: `{"coordinates":{"latitude":0,"longitude":30},"recorded_at":null}`,
		},
		{
			name: "unknown top-level field",
			body: `{"extra":1,"coordinates":{"latitude":0,"longitude":30},"recorded_at":"2026-01-01T00:00:00Z"}`,
		},
		{
			name: "unknown coordinate",
			body: `{"coordinates":{"extra":0,"latitude":0,"longitude":30},"recorded_at":"2026-01-01T00:00:00Z"}`,
		},
		{
			name: "extra Coordinates",
			body: `{"coordinates":{"latitude":0,"longitude":30},"Coordinates":{},"recorded_at":"2026-01-01T00:00:00Z"}`,
		},
		{
			name: "extra Latitude",
			body: `{"coordinates":{"latitude":0,"Latitude":30,"longitude":30},"recorded_at":"2026-01-01T00:00:00Z"}`,
		},
		{
			name: "missing latitude",
			body: `{"coordinates":{"longitude":30},"recorded_at":"2026-01-01T00:00:00Z"}`,
		},
		{
			name: "missing longitude",
			body: `{"coordinates":{"latitude":0},"recorded_at":"2026-01-01T00:00:00Z"}`,
		},
		{
			name: "null coordinate",
			body: `{"coordinates":{"latitude":null,"longitude":30},"recorded_at":"2026-01-01T00:00:00Z"}`,
		},
		{
			name: "null longitude",
			body: `{"coordinates":{"latitude":0,"longitude":null},"recorded_at":"2026-01-01T00:00:00Z"}`,
		},
		{
			name: "latitude above 90",
			body: `{"coordinates":{"latitude":91,"longitude":30},"recorded_at":"2026-01-01T00:00:00Z"}`,
		},
		{
			name: "timestamp one hour in the future",
			body: `{"coordinates":{"latitude":0,"longitude":30},"recorded_at":"` + time.Now().Add(time.Hour).Format(time.RFC3339) + `"}`,
		},
		{
			name: "malformed timestamp",
			body: `{"coordinates":{"latitude":0,"longitude":30},"recorded_at":"wrong"}`,
		},
		{name: "second JSON object", body: validPositionBody + "{}"},
		{name: "trailing garbage", body: validPositionBody + "x"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			assertInvalidPositionRequest(t, "application/json", tc.body)
		})
	}
}

func TestCreatePositionRejectsInvalidRequest(t *testing.T) {
	tests := []struct {
		name        string
		contentType string
		body        string
	}{
		{name: "missing content type", body: validPositionBody},
		{
			name:        "wrong content type",
			contentType: "text/plain",
			body:        validPositionBody,
		},
		{
			name:        "malformed content type",
			contentType: "application/json; charset",
			body:        validPositionBody,
		},
		{
			name:        "body too large",
			contentType: "application/json",
			body:        validPositionBody + strings.Repeat(" ", 1<<20),
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			assertInvalidPositionRequest(t, tc.contentType, tc.body)
		})
	}
}

func assertInvalidPositionRequest(t *testing.T, contentType, body string) {
	t.Helper()
	uc := NewMockusecase(gomock.NewController(t))
	id := uuid.New()
	request := httptest.NewRequest(http.MethodPost, "/api/v1/trips/"+id.String()+"/positions", strings.NewReader(body))
	request.Header.Set("Content-Type", contentType)
	response := httptest.NewRecorder()
	create.New(uc, slog.New(slog.NewTextHandler(io.Discard, nil))).CreateTripPosition(response, request, id)
	var problem api.Problem
	if err := json.Unmarshal(response.Body.Bytes(), &problem); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(response.Body.String(), "secret SQL") {
		t.Fatal("internal details leaked")
	}
	if response.Code != http.StatusBadRequest || problem.Code != "invalid_request" || response.Header().Get("Content-Type") != "application/problem+json" {
		t.Fatalf("unexpected response: status=%d body=%s", response.Code, response.Body)
	}
}

func TestCreatePositionRejectsZeroTripID(t *testing.T) {
	uc := NewMockusecase(gomock.NewController(t))
	request := httptest.NewRequest(http.MethodPost, "/api/v1/trips/"+uuid.Nil.String()+"/positions", strings.NewReader(`{"coordinates":{"latitude":0,"longitude":0},"recorded_at":"2026-01-01T00:00:00Z"}`))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	create.New(uc, slog.New(slog.NewTextHandler(io.Discard, nil))).CreateTripPosition(response, request, uuid.Nil)
	var problem api.Problem
	if err := json.Unmarshal(response.Body.Bytes(), &problem); err != nil {
		t.Fatal(err)
	}
	if response.Code != http.StatusBadRequest || problem.Code != "invalid_request" || response.Header().Get("Content-Type") != "application/problem+json" {
		t.Fatalf("unexpected response: status=%d body=%s", response.Code, response.Body)
	}
}
