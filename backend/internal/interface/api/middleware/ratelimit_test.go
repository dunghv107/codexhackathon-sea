package middleware

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
)

func limitedRouter(limiter *RateLimiter, perMinute int) *gin.Engine {
	gin.SetMode(gin.TestMode)
	mw := &Middleware{limiter: limiter}
	r := gin.New()
	r.GET("/x", mw.RateLimit("test", perMinute), func(c *gin.Context) { c.Status(http.StatusOK) })
	return r
}

func get(r *gin.Engine, ip string) int {
	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	req.RemoteAddr = ip + ":1234"
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w.Code
}

func TestRateLimit_PerIP(t *testing.T) {
	hits := map[string]int64{} // stands in for Redis
	limiter := &RateLimiter{window: time.Minute, count: func(_ context.Context, key string) (int64, int64, error) {
		hits[key]++
		return hits[key], 60_000, nil
	}}
	r := limitedRouter(limiter, 2)

	if got := []int{get(r, "10.0.0.1"), get(r, "10.0.0.1"), get(r, "10.0.0.1")}; got[0] != 200 || got[1] != 200 || got[2] != 429 {
		t.Fatalf("same IP: %v, want [200 200 429]", got)
	}
	if code := get(r, "10.0.0.2"); code != 200 {
		t.Fatalf("other IP: %d, want 200", code)
	}
}

func TestRateLimit_DisabledOrRedisDownPasses(t *testing.T) {
	down := &RateLimiter{window: time.Minute, count: func(context.Context, string) (int64, int64, error) {
		return 0, 0, context.DeadlineExceeded
	}}
	for name, limiter := range map[string]*RateLimiter{"disabled": nil, "redis down": down} {
		r := limitedRouter(limiter, 1)
		for i := 0; i < 3; i++ {
			if code := get(r, "10.0.0.1"); code != 200 {
				t.Fatalf("%s request %d: %d, want 200", name, i, code)
			}
		}
	}
}
