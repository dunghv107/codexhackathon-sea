package persistence

import (
	"context"

	"gorm.io/gorm"

	"github.com/tiloneeee/ai-fraud-detection/backend/internal/domain/entity"
	"github.com/tiloneeee/ai-fraud-detection/backend/internal/domain/repository"
	apperror "github.com/tiloneeee/ai-fraud-detection/backend/pkg/error"
	"github.com/tiloneeee/ai-fraud-detection/backend/pkg/query"
)

// userAllowedFields defines filterable/sortable fields for security
var userAllowedFields = map[string]bool{
	"id":         true,
	"username":   true,
	"email":      true,
	"role":       true,
	"status":     true,
	"created_at": true,
	"updated_at": true,
}

type userRepositoryImpl struct {
	*BaseRepositoryImpl[entity.User]
}

// NewUserRepository creates a new user repository
func NewUserRepository(db *gorm.DB) repository.UserRepository {
	base := NewBaseRepository[entity.User](db, userAllowedFields, "người dùng")
	return &userRepositoryImpl{BaseRepositoryImpl: base}
}

func (r *userRepositoryImpl) FindByUsername(ctx context.Context, username string) (*entity.User, error) {
	var user entity.User
	if err := dbFrom(ctx, r.DB).Where("username = ?", username).First(&user).Error; err != nil {
		return nil, wrapFindError(err, "người dùng")
	}
	return &user, nil
}

func (r *userRepositoryImpl) FindByEmail(ctx context.Context, email string) (*entity.User, error) {
	var user entity.User
	if err := dbFrom(ctx, r.DB).Where("email = ?", email).First(&user).Error; err != nil {
		return nil, wrapFindError(err, "người dùng")
	}
	return &user, nil
}

func (r *userRepositoryImpl) FindByUsernameIncludingDeleted(ctx context.Context, username string) (*entity.User, error) {
	var user entity.User
	if err := dbFrom(ctx, r.DB).Unscoped().Where("username = ?", username).First(&user).Error; err != nil {
		return nil, wrapFindError(err, "người dùng")
	}
	return &user, nil
}

func (r *userRepositoryImpl) FindByEmailIncludingDeleted(ctx context.Context, email string) (*entity.User, error) {
	var user entity.User
	if err := dbFrom(ctx, r.DB).Unscoped().Where("email = ?", email).First(&user).Error; err != nil {
		return nil, wrapFindError(err, "người dùng")
	}
	return &user, nil
}

func (r *userRepositoryImpl) Restore(ctx context.Context, id uint) error {
	if err := dbFrom(ctx, r.DB).Unscoped().Model(&entity.User{}).Where("id = ?", id).Update("deleted_at", nil).Error; err != nil {
		return apperror.ErrInternalServerError.WithMessage("Không thể khôi phục người dùng").WithError(err)
	}
	return nil
}

func (r *userRepositoryImpl) CreateSession(ctx context.Context, session *entity.UserSession) error {
	err := dbFrom(ctx, r.DB).Transaction(func(tx *gorm.DB) error {
		// Clean up this user's expired logins without affecting other devices.
		if err := tx.Where("user_id = ? AND expires_at <= now()", session.UserID).Delete(&entity.UserSession{}).Error; err != nil {
			return err
		}
		return tx.Create(session).Error
	})
	if err != nil {
		return wrapCreateError(err, "phiên đăng nhập")
	}
	return nil
}

func (r *userRepositoryImpl) FindSession(ctx context.Context, userID uint, sessionID string) (*entity.UserSession, error) {
	var session entity.UserSession
	if err := dbFrom(ctx, r.DB).Where("user_id = ? AND id = ?", userID, sessionID).First(&session).Error; err != nil {
		return nil, wrapFindError(err, "phiên đăng nhập")
	}
	return &session, nil
}

func (r *userRepositoryImpl) FindSessionByRefreshTokenHash(ctx context.Context, hash string) (*entity.UserSession, error) {
	var session entity.UserSession
	if err := dbFrom(ctx, r.DB).Where("refresh_token_hash = ?", hash).First(&session).Error; err != nil {
		return nil, wrapFindError(err, "phiên đăng nhập")
	}
	return &session, nil
}

func (r *userRepositoryImpl) RotateRefreshToken(ctx context.Context, userID uint, sessionID, oldHash, newHash string) (bool, error) {
	res := dbFrom(ctx, r.DB).Model(&entity.UserSession{}).
		Where("id = ? AND user_id = ? AND refresh_token_hash = ? AND expires_at > now()", sessionID, userID, oldHash).
		Update("refresh_token_hash", newHash)
	if res.Error != nil {
		return false, wrapUpdateError(res.Error, "phiên đăng nhập")
	}
	return res.RowsAffected == 1, nil
}

func (r *userRepositoryImpl) DeleteSession(ctx context.Context, userID uint, sessionID string) error {
	if err := dbFrom(ctx, r.DB).Where("user_id = ? AND id = ?", userID, sessionID).Delete(&entity.UserSession{}).Error; err != nil {
		return wrapDeleteError(err, "phiên đăng nhập")
	}
	return nil
}

func (r *userRepositoryImpl) ListWithQuery(ctx context.Context, offset, limit int, opts query.QueryOptions) ([]entity.User, int64, error) {
	var users []entity.User
	var total int64

	q := dbFrom(ctx, r.DB).Model(&entity.User{})

	// Handle special 'search' filter
	if searchFilter, ok := opts.Filters["search"]; ok {
		searchValue := searchFilter.Value.(string)
		q = q.Where("username ILIKE ? OR email ILIKE ?", "%"+searchValue+"%", "%"+searchValue+"%")
		delete(opts.Filters, "search")
	}

	// Apply filters
	q = q.Scopes(query.ApplyFilters(opts, r.AllowedFields))

	// Count total
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, wrapListError(err, "người dùng")
	}

	// Apply sort and pagination
	if err := q.Scopes(
		query.ApplySort(opts, r.AllowedFields),
		query.ApplyDefaultSort(opts, "created_at", true),
	).Offset(offset).Limit(limit).Find(&users).Error; err != nil {
		return nil, 0, wrapListError(err, "người dùng")
	}

	return users, total, nil
}
