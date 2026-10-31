package service

import (
	"context"

	"github.com/tiloneeee/ai-fraud-detection/backend/internal/domain/valueobject"
	"github.com/tiloneeee/ai-fraud-detection/backend/internal/interface/api/dto"
)

// AuthService defines authentication service interface
type AuthService interface {
	Login(ctx context.Context, username, password string) (*dto.LoginResponse, error)
	Refresh(ctx context.Context, refreshToken string) (*dto.LoginResponse, error)
	Logout(ctx context.Context, userID uint, sessionID string) error
	// Authenticate validates an access token and returns claims re-read from the database.
	Authenticate(ctx context.Context, accessToken string) (*valueobject.JWTClaims, error)
}
