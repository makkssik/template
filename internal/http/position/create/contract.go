//go:generate go tool mockgen -source contract.go -destination mocks_test.go -package create_test
package create

import (
	"context"

	"github.com/makkssik/template/internal/domain/position"
	"github.com/makkssik/template/internal/usecase/position/create"
)

type usecase interface {
	CreatePosition(context.Context, create.Input) (*position.Position, error)
}
