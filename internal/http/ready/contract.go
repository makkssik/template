//go:generate go tool mockgen -source contract.go -destination mocks_test.go -package ready_test
package ready

import "context"

type checkupService interface{ Check(context.Context) error }
