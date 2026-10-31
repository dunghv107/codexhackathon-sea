package main

import (
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"syscall"
	"testing"
	"time"
)

func TestMainHealthcheckMode(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/health" {
			t.Error(r.URL.Path)
		}
		w.WriteHeader(200)
	}))
	defer server.Close()
	u, _ := url.Parse(server.URL)
	t.Setenv("PORT", u.Port())
	args := os.Args
	os.Args = []string{"server", "healthcheck"}
	defer func() { os.Args = args }()
	main()
}
func TestServerStartsAndShutsDownCleanly(t *testing.T) {
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("requires isolated PostgreSQL")
	}
	u, e := url.Parse(dsn)
	if e != nil {
		t.Fatal(e)
	}
	listener, e := net.Listen("tcp", "127.0.0.1:0")
	if e != nil {
		t.Fatal(e)
	}
	port := listener.Addr().(*net.TCPAddr).Port
	listener.Close()
	t.Chdir(t.TempDir())
	t.Setenv("DB_HOST", u.Hostname())
	t.Setenv("DB_PORT", u.Port())
	if u.Port() == "" {
		t.Setenv("DB_PORT", "5432")
	}
	t.Setenv("DB_USER", u.User.Username())
	password, _ := u.User.Password()
	t.Setenv("DB_PASSWORD", password)
	t.Setenv("DB_NAME", u.Path[1:])
	t.Setenv("DB_SSLMODE", "disable")
	t.Setenv("DB_TIMEZONE", "UTC")
	t.Setenv("ENV", "production")
	t.Setenv("PORT", fmt.Sprint(port))
	t.Setenv("JWT_SECRET", "test-only-signing-key-not-production")
	t.Setenv("KBS_URL", "")
	t.Setenv("PHONE_HMAC_SECRET", "test-phone-hmac-key-only-not-production")
	t.Setenv("LOG_FILE_PATH", filepath.Join(t.TempDir(), "app.log"))
	t.Setenv("LOG_ENABLE_CONSOLE", "false")
	if redisURL := os.Getenv("TEST_REDIS_URL"); redisURL != "" {
		t.Setenv("RATE_LIMIT_ENABLED", "true")
		t.Setenv("REDIS_URL", redisURL)
	} else {
		t.Setenv("RATE_LIMIT_ENABLED", "false")
	}
	done := make(chan struct{})
	go func() { main(); close(done) }()
	// Poll a bounded local health endpoint, then deliver the same signal as container shutdown.
	client := http.Client{Timeout: 200 * time.Millisecond}
	ready := false
	for deadline := time.Now().Add(10 * time.Second); time.Now().Before(deadline); {
		r, e := client.Get(fmt.Sprintf("http://127.0.0.1:%d/health", port))
		if e == nil {
			r.Body.Close()
			if r.StatusCode == 200 {
				ready = true
				break
			}
		}
		time.Sleep(20 * time.Millisecond)
	}
	if !ready {
		t.Fatal("server did not become healthy")
	}
	if e := syscall.Kill(os.Getpid(), syscall.SIGTERM); e != nil {
		t.Fatal(e)
	}
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("server did not stop after SIGTERM")
	}
}
