package service

import (
	"context"

	"github.com/tiloneeee/ai-fraud-detection/backend/internal/domain/entity"
	"github.com/tiloneeee/ai-fraud-detection/backend/pkg/query"
)

// CreateUserCommand represents the command to create a user
type CreateUserCommand struct {
	Username string
	Email    string
	Password string
	Role     string
}

// UpdateUserCommand represents the command to update a user
type UpdateUserCommand struct {
	ID       uint
	Username string
	Email    string
	Role     string
	Status   string
}

// UserService defines the user service interface
type UserService interface {
	// CRUD
	Create(ctx context.Context, cmd CreateUserCommand) (*entity.User, error)
	GetByID(ctx context.Context, id uint) (*entity.User, error)
	Update(ctx context.Context, cmd UpdateUserCommand) (*entity.User, error)
	Delete(ctx context.Context, id uint) error

	// Query
	List(ctx context.Context, offset, limit int, opts query.QueryOptions) ([]entity.User, int64, error)
}
