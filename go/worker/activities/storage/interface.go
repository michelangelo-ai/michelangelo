package storage

import (
	"context"
	"go.uber.org/cadence"
)

type Storage interface {
	Read(ctx context.Context, req interface{}) (any, *cadence.CustomError)

	Protocol() string
}
