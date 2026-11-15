//go:generate go tool mockgen -source contract.go -destination mocks_test.go -package checkup_test
package checkup

import "context"

type databasePinger interface {
	Ping(ctx context.Context) error
}
