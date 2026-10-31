package service

import "github.com/tiloneeee/ai-fraud-detection/backend/internal/domain/valueobject"

// JWTService defines JWT operations
type JWTService interface {
	GenerateAccessToken(userID uint, username, role, sessionID string) (string, error)
	ValidateToken(tokenString string) (*valueobject.JWTClaims, error)
	GetAccessExpirySeconds() int
	GetRefreshExpirySeconds() int
}
