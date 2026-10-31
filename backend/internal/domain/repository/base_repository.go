package repository

import (
	"context"

	"github.com/tiloneeee/ai-fraud-detection/backend/pkg/query"
)

// BaseRepository is a generic repository interface
type BaseRepository[T any] interface {
	Create(ctx context.Context, entity *T) error
	FindByID(ctx context.Context, id uint) (*T, error)
	// Update writes only columns when given (avoids clobbering concurrent writes), else the whole row.
	Update(ctx context.Context, entity *T, columns ...string) error
	Delete(ctx context.Context, id uint) error
	List(ctx context.Context, offset, limit int, opts query.QueryOptions) ([]T, int64, error)
	Exists(ctx context.Context, id uint) (bool, error)
}
