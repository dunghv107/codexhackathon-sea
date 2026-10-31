package serviceimpl

import (
	"context"
	"testing"
	"time"

	"golang.org/x/crypto/bcrypt"

	"github.com/tiloneeee/ai-fraud-detection/backend/internal/domain/entity"
	"github.com/tiloneeee/ai-fraud-detection/backend/internal/domain/repository"
	apperror "github.com/tiloneeee/ai-fraud-detection/backend/pkg/error"
)

// fakeUserRepo holds one user in memory; unused interface methods panic via the nil embed.
type fakeUserRepo struct {
	repository.UserRepository
	user     entity.User
	sessions map[string]entity.UserSession
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

func (f *fakeUserRepo) CreateSession(_ context.Context, session *entity.UserSession) error {
	if f.sessions == nil {
		f.sessions = make(map[string]entity.UserSession)
	}
	f.sessions[session.ID] = *session
	return nil
}

func (f *fakeUserRepo) FindSession(_ context.Context, userID uint, id string) (*entity.UserSession, error) {
	session, ok := f.sessions[id]
	if !ok || session.UserID != userID {
		return nil, apperror.ErrNotFound
	}
	return &session, nil
}

func (f *fakeUserRepo) FindSessionByRefreshTokenHash(_ context.Context, hash string) (*entity.UserSession, error) {
	for _, session := range f.sessions {
		if session.RefreshTokenHash == hash {
			return &session, nil
		}
	}
	return nil, apperror.ErrNotFound
}

func (f *fakeUserRepo) RotateRefreshToken(ctx context.Context, userID uint, id, oldHash, newHash string) (bool, error) {
	session, err := f.FindSession(ctx, userID, id)
	if err != nil || session.RefreshTokenHash != oldHash || !session.ExpiresAt.After(time.Now()) {
		return false, nil
	}
	session.RefreshTokenHash = newHash
	f.sessions[id] = *session
	return true, nil
}

func (f *fakeUserRepo) DeleteSession(ctx context.Context, userID uint, id string) error {
	if _, err := f.FindSession(ctx, userID, id); err == nil {
		delete(f.sessions, id)
	}
	return nil
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

	if err := auth.Logout(ctx, 7, claims.SessionID); err != nil {
		t.Fatalf("logout: %v", err)
	}
	if _, err := auth.Authenticate(ctx, refreshed.AccessToken); !isCode(err, apperror.ErrUnauthorized) {
		t.Fatalf("access token after logout: %v, want UNAUTHORIZED", err)
	}
	if _, err := auth.Refresh(ctx, refreshed.RefreshToken); !isCode(err, apperror.ErrRefreshTokenInvalid) {
		t.Fatalf("refresh after logout: %v, want REFRESH_TOKEN_INVALID", err)
	}
}

func TestAuth_DevicesHaveIndependentSessions(t *testing.T) {
	ctx := context.Background()
	pw, _ := bcrypt.GenerateFromPassword([]byte("secret"), bcrypt.MinCost)
	repo := &fakeUserRepo{user: entity.User{ID: 7, Username: "alice", Password: string(pw), Role: entity.UserRoleUser, Status: entity.UserStatusActive}}
	jwt := NewJWTService("test-secret-at-least-32-characters!!", 15, 12)
	auth := NewAuthService(repo, jwt)
	a, err := auth.Login(ctx, "alice", "secret")
	if err != nil {
		t.Fatal(err)
	}
	b, err := auth.Login(ctx, "alice", "secret")
	if err != nil {
		t.Fatal(err)
	}
	claimsA, err := auth.Authenticate(ctx, a.AccessToken)
	if err != nil {
		t.Fatal("second login invalidated first device", err)
	}
	claimsB, err := auth.Authenticate(ctx, b.AccessToken)
	if err != nil || claimsA.SessionID == claimsB.SessionID || a.AccessToken == b.AccessToken {
		t.Fatal("devices must have different sessions", err)
	}
	expiresA := repo.sessions[claimsA.SessionID].ExpiresAt
	a, err = auth.Refresh(ctx, a.RefreshToken)
	if err != nil || !repo.sessions[claimsA.SessionID].ExpiresAt.Equal(expiresA) {
		t.Fatal("refresh must preserve this device's session expiry", err)
	}
	b, err = auth.Refresh(ctx, b.RefreshToken)
	if err != nil {
		t.Fatal("refreshing first device invalidated second device", err)
	}
	if err := auth.Logout(ctx, 7, claimsA.SessionID); err != nil {
		t.Fatal(err)
	}
	if _, err := auth.Authenticate(ctx, a.AccessToken); !isCode(err, apperror.ErrUnauthorized) {
		t.Fatal("logged out access token is still valid", err)
	}
	if _, err := auth.Refresh(ctx, a.RefreshToken); !isCode(err, apperror.ErrRefreshTokenInvalid) {
		t.Fatal("logged out refresh token is still valid", err)
	}
	if _, err := auth.Authenticate(ctx, b.AccessToken); err != nil {
		t.Fatal("logging out first device invalidated second device", err)
	}
	if _, err := auth.Login(ctx, "alice", "secret"); err != nil {
		t.Fatal(err)
	}
	if _, err := auth.Authenticate(ctx, a.AccessToken); !isCode(err, apperror.ErrUnauthorized) {
		t.Fatal("new login revived an ended session", err)
	}

	legacy, err := jwt.GenerateAccessToken(7, "alice", entity.UserRoleUser, "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := auth.Authenticate(ctx, legacy); !isCode(err, apperror.ErrUnauthorized) {
		t.Fatal("legacy tokens must require a fresh login", err)
	}
	if err := auth.Logout(ctx, 7, ""); !isCode(err, apperror.ErrUnauthorized) {
		t.Fatal("logout without a session must fail", err)
	}

	repo.user.Status = entity.UserStatusInactive
	if _, err := auth.Authenticate(ctx, b.AccessToken); !isCode(err, apperror.ErrForbidden) {
		t.Fatal("inactive user must be blocked", err)
	}
	if _, err := auth.Refresh(ctx, b.RefreshToken); !isCode(err, apperror.ErrForbidden) {
		t.Fatal("inactive user must not refresh", err)
	}
	repo.user.Status = entity.UserStatusActive
	session := repo.sessions[claimsB.SessionID]
	session.ExpiresAt = time.Now().Add(-time.Second)
	repo.sessions[session.ID] = session
	if _, err := auth.Authenticate(ctx, b.AccessToken); !isCode(err, apperror.ErrUnauthorized) {
		t.Fatal("expired session must reject access", err)
	}
	if _, err := auth.Refresh(ctx, b.RefreshToken); !isCode(err, apperror.ErrRefreshTokenInvalid) {
		t.Fatal("expired session must reject refresh", err)
	}
}
