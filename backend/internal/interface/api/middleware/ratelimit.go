package middleware

import (
	"context"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	goredis "github.com/redis/go-redis/v9"
	"github.com/thienel/tlog"
	"go.uber.org/zap"

	apperror "github.com/tiloneeee/ai-fraud-detection/backend/pkg/error"
	"github.com/tiloneeee/ai-fraud-detection/backend/pkg/response"
)

// RateLimiter is a fixed-window counter in Redis: INCR key, PEXPIRE on first hit.
// Key = "rl:<scope>:<client IP>:<window>". It runs before Auth, so it is per IP, not per user.
type RateLimiter struct {
	window time.Duration
	count  func(ctx context.Context, key string) (n, ttlMs int64, err error) // Redis; tests swap it
}

// NewRateLimiter creates a Redis-backed rate limiter
func NewRateLimiter(rdb *goredis.Client) *RateLimiter {
	r := &RateLimiter{window: time.Minute}
	r.count = func(ctx context.Context, key string) (int64, int64, error) {
		res, err := incrScript.Run(ctx, rdb, []string{key}, r.window.Milliseconds()).Slice()
		if err != nil {
			return 0, 0, err
		}
		return res[0].(int64), res[1].(int64), nil
	}
	return r
}

// incrScript increments and sets the TTL atomically on the first hit of a window.
var incrScript = goredis.NewScript(`
local n = redis.call('INCR', KEYS[1])
if n == 1 then redis.call('PEXPIRE', KEYS[1], ARGV[1]) end
local ttl = redis.call('PTTL', KEYS[1])
return {n, ttl}
`)

// RateLimit allows perMinute requests per subject under scope. A nil limiter (disabled) passes
// everything; a Redis error fails open and only logs, so a Redis outage does not take the API down.
func (m *Middleware) RateLimit(scope string, perMinute int) gin.HandlerFunc {
	r := m.limiter
	if r == nil || perMinute <= 0 {
		return func(c *gin.Context) { c.Next() }
	}
	return func(c *gin.Context) {
		subject := c.ClientIP()
		window := time.Now().Unix() / int64(r.window.Seconds())
		key := "rl:" + scope + ":" + subject + ":" + strconv.FormatInt(window, 10)

		n, ttlMs, err := r.count(c.Request.Context(), key)
		if err != nil {
			tlog.Warn("rate limiter unavailable, failing open", zap.Error(err))
			c.Next()
			return
		}
		remaining := max(int64(perMinute)-n, 0)
		resetSec := strconv.FormatInt((ttlMs+999)/1000, 10)
		c.Header("X-RateLimit-Limit", strconv.Itoa(perMinute))
		c.Header("X-RateLimit-Remaining", strconv.FormatInt(remaining, 10))
		c.Header("X-RateLimit-Reset", resetSec)
		if n > int64(perMinute) {
			c.Header("Retry-After", resetSec)
			response.WriteErrorResponse(c, apperror.ErrTooManyRequests)
			c.Abort()
			return
		}
		c.Next()
	}
}
