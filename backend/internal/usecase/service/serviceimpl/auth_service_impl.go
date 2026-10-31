package serviceimpl

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"time"

	"github.com/thienel/tlog"
	"go.uber.org/zap"
	"golang.org/x/crypto/bcrypt"

	"github.com/tiloneeee/ai-fraud-detection/backend/internal/domain/entity"
	"github.com/tiloneeee/ai-fraud-detection/backend/internal/domain/repository"
	"github.com/tiloneeee/ai-fraud-detection/backend/internal/domain/valueobject"
	"github.com/tiloneeee/ai-fraud-detection/backend/internal/interface/api/dto"
	"github.com/tiloneeee/ai-fraud-detection/backend/internal/usecase/service"
	apperror "github.com/tiloneeee/ai-fraud-detection/backend/pkg/error"
)

type authServiceImpl struct {
	userRepo   repository.UserRepository
	jwtService service.JWTService
}

// NewAuthService creates a new auth service
func NewAuthService(userRepo repository.UserRepository, jwtService service.JWTService) service.AuthService {
	return &authServiceImpl{
		userRepo:   userRepo,
		jwtService: jwtService,
	}
}

func hashRefreshToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

func newRefreshToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", apperror.ErrInternalServerError.WithMessage("Không thể tạo refresh token").WithError(err)
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

// isNotFound tells a missing user apart from a database failure.
func isNotFound(err error) bool {
	var appErr *apperror.AppError
	return errors.As(err, &appErr) && appErr.Code == apperror.ErrNotFound.Code
}

func (s *authServiceImpl) Login(ctx context.Context, username, password string) (*dto.LoginResponse, error) {
	user, err := s.userRepo.FindByUsername(ctx, username)
	if err != nil {
		if !isNotFound(err) {
			return nil, err
		}
		tlog.Debug("Login failed: user not found", zap.String("username", username))
		return nil, apperror.ErrInvalidCredentials
	}

	if err := bcrypt.CompareHashAndPassword([]byte(user.Password), []byte(password)); err != nil {
		tlog.Debug("Login failed: invalid password", zap.String("username", username))
		return nil, apperror.ErrInvalidCredentials
	}

	if user.Status != entity.UserStatusActive {
		tlog.Debug("Login failed: user inactive", zap.String("username", username))
		return nil, apperror.ErrForbidden.WithMessage("Tài khoản đã bị vô hiệu hóa")
	}

	accessToken, err := s.jwtService.GenerateAccessToken(user.ID, user.Username, user.Role)
	if err != nil {
		return nil, apperror.ErrInternalServerError.WithMessage("Không thể tạo access token").WithError(err)
	}

	// A new login starts a new session and replaces any previous one.
	refreshToken, err := newRefreshToken()
	if err != nil {
		return nil, err
	}
	hash := hashRefreshToken(refreshToken)
	expiresAt := time.Now().UTC().Add(time.Duration(s.jwtService.GetRefreshExpirySeconds()) * time.Second)
	user.RefreshTokenHash = &hash
	user.RefreshTokenExpiresAt = &expiresAt
	if err := s.userRepo.Update(ctx, user, "refresh_token_hash", "refresh_token_expires_at"); err != nil {
		return nil, err
	}

	tlog.Info("User logged in", zap.Uint("user_id", user.ID), zap.String("username", user.Username))

	return &dto.LoginResponse{
		User:         toUserResponse(user),
		AccessToken:  accessToken,
		RefreshToken: refreshToken,
	}, nil
}

// Refresh rotates the refresh token. The expiry is not extended, so a session lives at most
// JWT_REFRESH_EXPIRY_HOURS from login.
func (s *authServiceImpl) Refresh(ctx context.Context, refreshToken string) (*dto.LoginResponse, error) {
	if refreshToken == "" {
		return nil, apperror.ErrRefreshTokenInvalid
	}
	oldHash := hashRefreshToken(refreshToken)
	user, err := s.userRepo.FindByRefreshTokenHash(ctx, oldHash)
	if err != nil {
		if !isNotFound(err) {
			return nil, err
		}
		return nil, apperror.ErrRefreshTokenInvalid
	}
	if user.RefreshTokenExpiresAt == nil || user.RefreshTokenExpiresAt.Before(time.Now()) {
		return nil, apperror.ErrRefreshTokenInvalid
	}
	if user.Status != entity.UserStatusActive {
		return nil, apperror.ErrForbidden.WithMessage("Tài khoản đã bị vô hiệu hóa")
	}

	newToken, err := newRefreshToken()
	if err != nil {
		return nil, err
	}
	// Conditional UPDATE: of two concurrent refreshes with the same token only one wins.
	ok, err := s.userRepo.RotateRefreshToken(ctx, user.ID, oldHash, hashRefreshToken(newToken))
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, apperror.ErrRefreshTokenInvalid
	}

	accessToken, err := s.jwtService.GenerateAccessToken(user.ID, user.Username, user.Role)
	if err != nil {
		return nil, apperror.ErrInternalServerError.WithMessage("Không thể tạo access token").WithError(err)
	}

	return &dto.LoginResponse{
		User:         toUserResponse(user),
		AccessToken:  accessToken,
		RefreshToken: newToken,
	}, nil
}

// Logout ends the session: the refresh token stops working and Authenticate rejects
// access tokens issued for it.
func (s *authServiceImpl) Logout(ctx context.Context, userID uint) error {
	user, err := s.userRepo.FindByID(ctx, userID)
	if err != nil {
		return err
	}
	user.RefreshTokenHash = nil
	user.RefreshTokenExpiresAt = nil
	if err := s.userRepo.Update(ctx, user, "refresh_token_hash", "refresh_token_expires_at"); err != nil {
		return err
	}
	tlog.Info("User logged out", zap.Uint("user_id", userID))
	return nil
}

// Authenticate re-reads the user on every request so deletion, deactivation, role changes
// and logout take effect immediately instead of when the access token expires.
func (s *authServiceImpl) Authenticate(ctx context.Context, accessToken string) (*valueobject.JWTClaims, error) {
	claims, err := s.jwtService.ValidateToken(accessToken)
	if err != nil {
		return nil, err
	}
	user, err := s.userRepo.FindByID(ctx, claims.UserID)
	if err != nil {
		if !isNotFound(err) {
			return nil, err
		}
		return nil, apperror.ErrUnauthorized
	}
	if user.Status != entity.UserStatusActive {
		return nil, apperror.ErrForbidden.WithMessage("Tài khoản đã bị vô hiệu hóa")
	}
	if user.RefreshTokenHash == nil || user.RefreshTokenExpiresAt == nil || user.RefreshTokenExpiresAt.Before(time.Now()) {
		return nil, apperror.ErrUnauthorized.WithMessage("Phiên đăng nhập đã kết thúc")
	}
	return &valueobject.JWTClaims{UserID: user.ID, Username: user.Username, Role: user.Role}, nil
}

func toUserResponse(user *entity.User) dto.UserResponse {
	return dto.UserResponse{
		ID:        user.ID,
		Username:  user.Username,
		Email:     user.Email,
		Role:      user.Role,
		Status:    user.Status,
		CreatedAt: user.CreatedAt,
		UpdatedAt: user.UpdatedAt,
	}
}
