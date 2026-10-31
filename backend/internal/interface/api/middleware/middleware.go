package middleware

import (
	"strings"

	"github.com/tiloneeee/ai-fraud-detection/backend/internal/usecase/service"
)

// Middleware holds all middleware dependencies
type Middleware struct {
	authService    service.AuthService
	profileService service.ProfileService
	sellerService  service.SellerService
	limiter        *RateLimiter
	origins        string
	allowedOrigins []string
	allowAll       bool
}

// New creates a new Middleware instance
// limiter may be nil (rate limiting disabled).
func New(
	authService service.AuthService,
	profileService service.ProfileService,
	sellerService service.SellerService,
	limiter *RateLimiter,
	origins string,
) *Middleware {
	allowed := strings.Split(origins, ",")
	allowAll := len(allowed) == 1 && strings.TrimSpace(allowed[0]) == "*"

	for i := range allowed {
		allowed[i] = strings.TrimSpace(allowed[i])
	}

	return &Middleware{
		authService:    authService,
		profileService: profileService,
		sellerService:  sellerService,
		limiter:        limiter,
		origins:        origins,
		allowedOrigins: allowed,
		allowAll:       allowAll,
	}
}
