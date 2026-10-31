package serviceimpl

import (
	"context"

	"github.com/thienel/tlog"
	"go.uber.org/zap"
	"golang.org/x/crypto/bcrypt"

	"github.com/tiloneeee/ai-fraud-detection/backend/internal/domain/entity"
	"github.com/tiloneeee/ai-fraud-detection/backend/internal/domain/repository"
	"github.com/tiloneeee/ai-fraud-detection/backend/internal/usecase/service"
	apperror "github.com/tiloneeee/ai-fraud-detection/backend/pkg/error"
	"github.com/tiloneeee/ai-fraud-detection/backend/pkg/query"
)

type userServiceImpl struct {
	userRepo repository.UserRepository
	tx       repository.Transactor
}

// NewUserService creates a new user service
func NewUserService(userRepo repository.UserRepository, tx repository.Transactor) service.UserService {
	return &userServiceImpl{userRepo: userRepo, tx: tx}
}

func (s *userServiceImpl) Create(ctx context.Context, cmd service.CreateUserCommand) (*entity.User, error) {
	// Validate role
	role := entity.UserRoleUser
	if cmd.Role != "" {
		if !entity.IsValidUserRole(cmd.Role) {
			return nil, apperror.ErrValidation.WithMessage("Role không hợp lệ")
		}
		role = cmd.Role
	}

	// Check username exists
	if _, err := s.userRepo.FindByUsernameIncludingDeleted(ctx, cmd.Username); err == nil {
		tlog.Debug("Create user failed: username exists", zap.String("username", cmd.Username))
		return nil, apperror.ErrUsernameExists
	}

	// Check email exists
	if _, err := s.userRepo.FindByEmailIncludingDeleted(ctx, cmd.Email); err == nil {
		tlog.Debug("Create user failed: email exists", zap.String("email", cmd.Email))
		return nil, apperror.ErrEmailExists
	}

	// Hash password
	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(cmd.Password), bcrypt.DefaultCost)
	if err != nil {
		return nil, apperror.ErrInternalServerError.WithMessage("Không thể mã hóa mật khẩu").WithError(err)
	}

	user := &entity.User{
		Username: cmd.Username,
		Email:    cmd.Email,
		Password: string(hashedPassword),
		Role:     role,
		Status:   entity.UserStatusActive,
	}

	if err := s.userRepo.Create(ctx, user); err != nil {
		return nil, err
	}

	tlog.Info("User created", zap.Uint("user_id", user.ID), zap.String("username", user.Username))
	return user, nil
}

func (s *userServiceImpl) GetByID(ctx context.Context, id uint) (*entity.User, error) {
	user, err := s.userRepo.FindByID(ctx, id)
	if err != nil {
		tlog.Debug("Get user failed: not found", zap.Uint("user_id", id))
		return nil, err
	}
	return user, nil
}

func (s *userServiceImpl) Update(ctx context.Context, cmd service.UpdateUserCommand) (*entity.User, error) {
	var user *entity.User
	err := s.tx.WithinTransaction(ctx, func(ctx context.Context) error {
		var err error
		user, err = s.userRepo.FindByID(ctx, cmd.ID)
		if err != nil {
			tlog.Debug("Update user failed: not found", zap.Uint("user_id", cmd.ID))
			return err
		}

		// Update username if changed
		if cmd.Username != "" && cmd.Username != user.Username {
			if _, err := s.userRepo.FindByUsernameIncludingDeleted(ctx, cmd.Username); err == nil {
				return apperror.ErrUsernameExists
			}
			user.Username = cmd.Username
		}

		// Update email if changed
		if cmd.Email != "" && cmd.Email != user.Email {
			if _, err := s.userRepo.FindByEmailIncludingDeleted(ctx, cmd.Email); err == nil {
				return apperror.ErrEmailExists
			}
			user.Email = cmd.Email
		}

		// Update role
		if cmd.Role != "" {
			if !entity.IsValidUserRole(cmd.Role) {
				return apperror.ErrValidation.WithMessage("Role không hợp lệ")
			}
			user.Role = cmd.Role
		}

		// Update status
		if cmd.Status != "" {
			if !entity.IsValidUserStatus(cmd.Status) {
				return apperror.ErrValidation.WithMessage("Status không hợp lệ")
			}
			user.Status = cmd.Status
		}

		// Column allowlist: never rewrite the password during a profile update.
		return s.userRepo.Update(ctx, user, "username", "email", "role", "status", "updated_at")
	})
	if err != nil {
		return nil, err
	}

	tlog.Info("User updated", zap.Uint("user_id", user.ID))
	return user, nil
}

func (s *userServiceImpl) Delete(ctx context.Context, id uint) error {
	// Check exists
	if _, err := s.userRepo.FindByID(ctx, id); err != nil {
		tlog.Debug("Delete user failed: not found", zap.Uint("user_id", id))
		return err
	}

	if err := s.userRepo.Delete(ctx, id); err != nil {
		return err
	}

	tlog.Info("User deleted", zap.Uint("user_id", id))
	return nil
}

func (s *userServiceImpl) List(ctx context.Context, offset, limit int, opts query.QueryOptions) ([]entity.User, int64, error) {
	return s.userRepo.ListWithQuery(ctx, offset, limit, opts)
}
