//go:generate go tool mockgen -source contract.go -destination mocks_test.go -package list_test
package list

import (
	"context"

	"github.com/google/uuid"

	"github.com/makkssik/template/internal/domain/position"
)

type usecase interface {
	List(context.Context, uuid.UUID) ([]position.Position, error)
}
