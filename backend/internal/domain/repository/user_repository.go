package repository

import (
	"context"

	"github.com/tiloneeee/ai-fraud-detection/backend/internal/domain/entity"
	"github.com/tiloneeee/ai-fraud-detection/backend/pkg/query"
)

// UserRepository extends BaseRepository for User entity
type UserRepository interface {
	BaseRepository[entity.User]

	// Additional user-specific methods
	FindByUsername(ctx context.Context, username string) (*entity.User, error)
	FindByEmail(ctx context.Context, email string) (*entity.User, error)
	FindByUsernameIncludingDeleted(ctx context.Context, username string) (*entity.User, error)
	FindByEmailIncludingDeleted(ctx context.Context, email string) (*entity.User, error)
	Restore(ctx context.Context, id uint) error

	CreateSession(ctx context.Context, session *entity.UserSession) error
	FindSession(ctx context.Context, userID uint, sessionID string) (*entity.UserSession, error)
	FindSessionByRefreshTokenHash(ctx context.Context, hash string) (*entity.UserSession, error)
	// RotateRefreshToken atomically replaces oldHash with newHash; false when oldHash no longer matches.
	RotateRefreshToken(ctx context.Context, userID uint, sessionID, oldHash, newHash string) (bool, error)
	DeleteSession(ctx context.Context, userID uint, sessionID string) error

	// ListWithQuery supports search filter across multiple fields
	ListWithQuery(ctx context.Context, offset, limit int, opts query.QueryOptions) ([]entity.User, int64, error)
}
