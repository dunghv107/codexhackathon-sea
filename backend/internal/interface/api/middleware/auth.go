package middleware

import (
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/tiloneeee/ai-fraud-detection/backend/internal/domain/entity"
	"github.com/tiloneeee/ai-fraud-detection/backend/internal/domain/valueobject"
	apperror "github.com/tiloneeee/ai-fraud-detection/backend/pkg/error"
	"github.com/tiloneeee/ai-fraud-detection/backend/pkg/response"
)

type ContextKey string

const UserContextKey ContextKey = "user"

// Auth returns JWT authentication middleware
func (m *Middleware) Auth() gin.HandlerFunc {
	return func(c *gin.Context) {
		token := getTokenFromHeader(c.GetHeader("Authorization"))
		if token == "" {
			response.WriteErrorResponse(c, apperror.ErrUnauthorized)
			c.Abort()
			return
		}

		claims, err := m.authService.Authenticate(c.Request.Context(), token)
		if err != nil {
			response.WriteErrorResponse(c, err)
			c.Abort()
			return
		}

		c.Set(string(UserContextKey), claims)
		c.Next()
	}
}

// AllowRoles returns middleware that checks user role
func (m *Middleware) AllowRoles(requiredRoles ...string) gin.HandlerFunc {
	roleSet := make(map[string]struct{}, len(requiredRoles))
	for _, r := range requiredRoles {
		roleSet[r] = struct{}{}
	}

	return func(c *gin.Context) {
		claims := GetUserClaims(c)
		if claims == nil {
			response.WriteErrorResponse(c, apperror.ErrUnauthorized)
			c.Abort()
			return
		}

		if _, ok := roleSet[claims.Role]; !ok {
			response.WriteErrorResponse(c, apperror.ErrForbidden)
			c.Abort()
			return
		}

		c.Next()
	}
}

// RequireAdmin is a convenience method for admin-only routes
func (m *Middleware) RequireAdmin() gin.HandlerFunc {
	return m.AllowRoles(entity.UserRoleAdmin, entity.UserRoleSystemAdmin)
}

func getTokenFromHeader(authHeader string) string {
	if authHeader == "" {
		return ""
	}
	parts := strings.Split(authHeader, " ")
	if len(parts) == 2 && parts[0] == "Bearer" {
		return parts[1]
	}
	return ""
}

// GetUserClaims retrieves user claims from context
func GetUserClaims(c *gin.Context) *valueobject.JWTClaims {
	v, exists := c.Get(string(UserContextKey))
	if !exists {
		return nil
	}
	claims, ok := v.(*valueobject.JWTClaims)
	if !ok {
		return nil
	}
	return claims
}

// GetUserID retrieves user ID from context
func GetUserID(c *gin.Context) uint {
	claims := GetUserClaims(c)
	if claims == nil {
		return 0
	}
	return claims.UserID
}

// GetUsername retrieves username from context
func GetUsername(c *gin.Context) string {
	claims := GetUserClaims(c)
	if claims == nil {
		return ""
	}
	return claims.Username
}

// GetRole retrieves role from context
func GetRole(c *gin.Context) string {
	claims := GetUserClaims(c)
	if claims == nil {
		return ""
	}
	return claims.Role
}
