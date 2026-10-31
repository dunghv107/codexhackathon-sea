package serviceimpl

import (
	"errors"
	"time"

	"github.com/golang-jwt/jwt/v5"

	"github.com/tiloneeee/ai-fraud-detection/backend/internal/domain/valueobject"
	"github.com/tiloneeee/ai-fraud-detection/backend/internal/usecase/service"
	apperror "github.com/tiloneeee/ai-fraud-detection/backend/pkg/error"
)

type jwtServiceImpl struct {
	secret              string
	accessExpiryMinutes int
	refreshExpiryHours  int
}

// NewJWTService creates a new JWT service
func NewJWTService(secret string, accessExpiryMinutes, refreshExpiryHours int) service.JWTService {
	return &jwtServiceImpl{
		secret:              secret,
		accessExpiryMinutes: accessExpiryMinutes,
		refreshExpiryHours:  refreshExpiryHours,
	}
}

type jwtClaims struct {
	UserID    uint   `json:"user_id"`
	Username  string `json:"username"`
	Role      string `json:"role"`
	SessionID string `json:"session_id"`
	jwt.RegisteredClaims
}

func (s *jwtServiceImpl) GenerateAccessToken(userID uint, username, role, sessionID string) (string, error) {
	expiry := time.Now().Add(time.Duration(s.accessExpiryMinutes) * time.Minute)

	claims := jwtClaims{
		UserID:    userID,
		Username:  username,
		Role:      role,
		SessionID: sessionID,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(expiry),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			Subject:   username,
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString([]byte(s.secret))
}

func (s *jwtServiceImpl) ValidateToken(tokenString string) (*valueobject.JWTClaims, error) {
	token, err := jwt.ParseWithClaims(tokenString, &jwtClaims{}, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, errors.New("invalid signing method")
		}
		return []byte(s.secret), nil
	})

	if err != nil {
		if errors.Is(err, jwt.ErrTokenExpired) {
			return nil, apperror.ErrTokenExpired
		}
		return nil, apperror.ErrUnauthorized.WithMessage("Token không hợp lệ")
	}

	claims, ok := token.Claims.(*jwtClaims)
	if !ok || !token.Valid {
		return nil, apperror.ErrUnauthorized.WithMessage("Token không hợp lệ")
	}

	return &valueobject.JWTClaims{
		UserID:    claims.UserID,
		Username:  claims.Username,
		Role:      claims.Role,
		SessionID: claims.SessionID,
	}, nil
}

func (s *jwtServiceImpl) GetAccessExpirySeconds() int {
	return s.accessExpiryMinutes * 60
}

func (s *jwtServiceImpl) GetRefreshExpirySeconds() int {
	return s.refreshExpiryHours * 3600
}
