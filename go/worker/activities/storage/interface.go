package storage

import (
	"context"
)

type Storage interface {
	Read(ctx context.Context, path string) (any, error)

	Protocol() string
}
