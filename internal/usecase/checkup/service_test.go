package checkup_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"go.uber.org/mock/gomock"

	"github.com/makkssik/template/internal/usecase/checkup"
)

func TestCheck(t *testing.T) {
	for _, tc := range []struct {
		name    string
		failure error
	}{
		{"ready", nil},
		{"unavailable", errors.New("database unavailable")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			failure := tc.failure
			database := NewMockdatabasePinger(gomock.NewController(t))
			database.EXPECT().Ping(gomock.Any()).DoAndReturn(func(ctx context.Context) error {
				deadline, ok := ctx.Deadline()
				if !ok || time.Until(deadline) > time.Second {
					t.Fatal("ping is not bounded by query timeout")
				}
				return failure
			})
			service := checkup.NewService(checkup.ServiceCfg{Database: database, QueryTimeout: time.Second})
			if err := service.Check(t.Context()); !errors.Is(err, failure) {
				t.Fatalf("got %v, want %v", err, failure)
			}
		})
	}
}
