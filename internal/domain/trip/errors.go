package trip

import (
	"errors"
	"fmt"
)

var (
	ErrTripNotFound  = errors.New("trip not found")
	ErrTripCompleted = errors.New("trip already completed")
	ErrDriverBusy    = errors.New("driver already has an active trip")
)

type ValidationError struct {
	Field  string
	Detail string
}

func (e *ValidationError) Error() string { return fmt.Sprintf("%s: %s", e.Field, e.Detail) }
