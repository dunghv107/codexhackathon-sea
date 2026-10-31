package serviceimpl

import (
	"context"
	"testing"

	"golang.org/x/crypto/bcrypt"

	"github.com/tiloneeee/ai-fraud-detection/backend/internal/domain/entity"
	"github.com/tiloneeee/ai-fraud-detection/backend/internal/domain/repository"
	apperror "github.com/tiloneeee/ai-fraud-detection/backend/pkg/error"
)

// fakeUserRepo holds one user in memory; unused interface methods panic via the nil embed.
type fakeUserRepo struct {
	repository.UserRepository
	user entity.User
}

func (f *fakeUserRepo) FindByUsername(_ context.Context, username string) (*entity.User, error) {
	if username != f.user.Username {
		return nil, apperror.ErrNotFound
	}
	u := f.user
	return &u, nil
}

func (f *fakeUserRepo) FindByID(_ context.Context, id uint) (*entity.User, error) {
	if id != f.user.ID {
		return nil, apperror.ErrNotFound
	}
	u := f.user
	return &u, nil
}

func (f *fakeUserRepo) FindByRefreshTokenHash(_ context.Context, hash string) (*entity.User, error) {
	if f.user.RefreshTokenHash == nil || *f.user.RefreshTokenHash != hash {
		return nil, apperror.ErrNotFound
	}
	u := f.user
	return &u, nil
}

func (f *fakeUserRepo) Update(_ context.Context, u *entity.User, _ ...string) error {
	f.user.RefreshTokenHash, f.user.RefreshTokenExpiresAt = u.RefreshTokenHash, u.RefreshTokenExpiresAt
	return nil
}

func (f *fakeUserRepo) RotateRefreshToken(_ context.Context, id uint, oldHash, newHash string) (bool, error) {
	if id != f.user.ID || f.user.RefreshTokenHash == nil || *f.user.RefreshTokenHash != oldHash {
		return false, nil
	}
	f.user.RefreshTokenHash = &newHash
	return true, nil
}

func isCode(err error, want *apperror.AppError) bool {
	appErr, ok := err.(*apperror.AppError)
	return ok && appErr.Code == want.Code
}

func TestAuth_RefreshRotatesAndLogoutEndsSession(t *testing.T) {
	ctx := context.Background()
	pw, _ := bcrypt.GenerateFromPassword([]byte("secret"), bcrypt.MinCost)
	repo := &fakeUserRepo{user: entity.User{ID: 7, Username: "alice", Password: string(pw), Role: entity.UserRoleUser, Status: entity.UserStatusActive}}
	auth := NewAuthService(repo, NewJWTService("test-secret-at-least-32-characters!!", 15, 12))

	login, err := auth.Login(ctx, "alice", "secret")
	if err != nil {
		t.Fatalf("login: %v", err)
	}
	if _, err := auth.Authenticate(ctx, login.AccessToken); err != nil {
		t.Fatalf("authenticate after login: %v", err)
	}

	refreshed, err := auth.Refresh(ctx, login.RefreshToken)
	if err != nil {
		t.Fatalf("refresh: %v", err)
	}
	if refreshed.RefreshToken == login.RefreshToken {
		t.Fatal("refresh token was not rotated")
	}
	if _, err := auth.Refresh(ctx, login.RefreshToken); !isCode(err, apperror.ErrRefreshTokenInvalid) {
		t.Fatalf("reusing old refresh token: %v, want REFRESH_TOKEN_INVALID", err)
	}

	// Role changes apply to existing access tokens on the next request.
	repo.user.Role = entity.UserRoleAdmin
	claims, err := auth.Authenticate(ctx, refreshed.AccessToken)
	if err != nil || claims.Role != entity.UserRoleAdmin {
		t.Fatalf("authenticate after role change: %+v, %v", claims, err)
	}

	if err := auth.Logout(ctx, 7); err != nil {
		t.Fatalf("logout: %v", err)
	}
	if _, err := auth.Authenticate(ctx, refreshed.AccessToken); !isCode(err, apperror.ErrUnauthorized) {
		t.Fatalf("access token after logout: %v, want UNAUTHORIZED", err)
	}
	if _, err := auth.Refresh(ctx, refreshed.RefreshToken); !isCode(err, apperror.ErrRefreshTokenInvalid) {
		t.Fatalf("refresh after logout: %v, want REFRESH_TOKEN_INVALID", err)
	}
}
