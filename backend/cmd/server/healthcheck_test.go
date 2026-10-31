package main

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestCheckHealth(t *testing.T) {
	for _, status := range []int{http.StatusOK, http.StatusServiceUnavailable, http.StatusFound} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Location", "/health")
				w.WriteHeader(status)
			}))
			defer server.Close()
			if err := checkHealth(server.URL); (err == nil) != (status == http.StatusOK) {
				t.Fatalf("status %d: unexpected healthcheck result: %v", status, err)
			}
		})
	}
	server := httptest.NewServer(http.NotFoundHandler())
	server.Close()
	if err := checkHealth(server.URL); err == nil {
		t.Fatal("a stopped server must fail its healthcheck")
	}
}
