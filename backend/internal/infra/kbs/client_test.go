package kbs

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/tiloneeee/ai-fraud-detection/backend/internal/usecase/service"
)

func TestClientContract(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" || r.Header.Get("X-Shop-ID") != "shop-a" {
			t.Error("incorrect trusted shop/method")
		}
		switch r.URL.Path {
		case "/api/v1/orders/live:order/score":
			if r.URL.Query().Get("force") != "true" {
				t.Error("force lost")
			}
			fmt.Fprint(w, `{"assessment_id":"a","order_id":"live:order"}`)
		case "/api/v1/orders/live:order/explain":
			fmt.Fprint(w, `{"assessment_id":"a","source":"template","sentences":["R3 fired"]}`)
		default:
			t.Error("unexpected path")
			w.WriteHeader(404)
		}
	}))
	defer server.Close()
	client, err := NewClient(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	if err := client.Score(context.Background(), "shop-a", "live:order", true); err != nil {
		t.Fatal(err)
	}
	result, err := client.Explain(context.Background(), "shop-a", "live:order")
	if err != nil || result.AssessmentID != "a" {
		t.Fatalf("explanation: %v %v", result, err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := client.Score(ctx, "shop-a", "live:order", true); err != service.ErrRiskUnavailable {
		t.Fatalf("canceled request: %v", err)
	}
}

func TestClientRejectsFailures(t *testing.T) {
	for _, tc := range []struct {
		status int
		body   string
	}{
		{503, "unavailable"}, {302, ""}, {200, "invalid"}, {200, `{}`},
		{200, `{"assessment_id":"a","order_id":"foreign"}`},
	} {
		t.Run(fmt.Sprint(tc.status, tc.body), func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(tc.status); fmt.Fprint(w, tc.body) }))
			defer server.Close()
			client, err := NewClient(server.URL)
			if err != nil {
				t.Fatal(err)
			}
			if err := client.Score(context.Background(), "s", "o", false); err != service.ErrRiskUnavailable {
				t.Fatalf("got %v", err)
			}
		})
	}
	disabled, _ := NewClient("")
	if err := disabled.Score(context.Background(), "s", "o", false); err != service.ErrRiskUnavailable {
		t.Fatal(err)
	}
	for _, origin := range []string{"ftp://localhost", "http://user:secret@localhost", "http://localhost/path"} {
		if _, err := NewClient(origin); err == nil {
			t.Fatal("accepted invalid origin")
		}
	}
}
