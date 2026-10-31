package kbs

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

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
			fmt.Fprint(w, `{"assessment_id":"a","order_id":"live:order","score":30,"risk_level":"medium"}`)
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
	if score, err := client.Score(context.Background(), "shop-a", "live:order", true); err != nil || score.Score != 30 || score.RiskLevel != "medium" {
		t.Fatalf("score contract: %v %v", score, err)
	}
	result, err := client.Explain(context.Background(), "shop-a", "live:order")
	if err != nil || result.AssessmentID != "a" {
		t.Fatalf("explanation: %v %v", result, err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := client.Score(ctx, "shop-a", "live:order", true); err != service.ErrRiskUnavailable {
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
		{200, `{"assessment_id":"a","order_id":"o","risk_level":"medium"}`},
		{200, `{"assessment_id":"a","order_id":"o","risk_level":"invalid","score":30}`},
		{200, `{"assessment_id":"a","order_id":"o","risk_level":"high","score":101}`},
	} {
		t.Run(fmt.Sprint(tc.status, tc.body), func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(tc.status); fmt.Fprint(w, tc.body) }))
			defer server.Close()
			client, err := NewClient(server.URL)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := client.Score(context.Background(), "s", "o", false); err != service.ErrRiskUnavailable {
				t.Fatalf("got %v", err)
			}
		})
	}
	disabled, _ := NewClient("")
	if _, err := disabled.Score(context.Background(), "s", "o", false); err != service.ErrRiskUnavailable {
		t.Fatal(err)
	}
	for _, origin := range []string{"ftp://localhost", "http://user:secret@localhost", "http://localhost/path"} {
		if _, err := NewClient(origin); err == nil {
			t.Fatal("accepted invalid origin")
		}
	}
}

func TestNetworkContractAndBounds(t *testing.T) {
	for _, size := range []int{1, 200, 201} {
		t.Run(fmt.Sprint(size), func(t *testing.T) {
			network := service.RiskNetwork{OrderID: "o", InputCutoffAt: time.Now(), Truncated: true, Edges: []service.NetworkEdge{}}
			for i := 0; i < size; i++ {
				network.Nodes = append(network.Nodes, service.NetworkNode{ID: fmt.Sprint(i), Kind: "checkout", Label: "Safe label", Count: 1})
			}
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != "GET" || r.URL.Path != "/api/v1/orders/o/network" || r.Header.Get("X-Shop-ID") != "s" {
					t.Error("network method/scope mismatch")
				}
				json.NewEncoder(w).Encode(network)
			}))
			defer server.Close()
			client, _ := NewClient(server.URL)
			result, err := client.Network(context.Background(), "s", "o")
			if size > 200 {
				if err == nil {
					t.Fatal("unbounded network accepted")
				}
				return
			}
			if err != nil || len(result.Nodes) != size || !result.Truncated {
				t.Fatal("network contract lost")
			}
			network.Edges = []service.NetworkEdge{{Source: "0", Target: "foreign"}}
			if _, err := client.Network(context.Background(), "s", "o"); err == nil {
				t.Fatal("dangling edge accepted")
			}
		})
	}
}

func TestNetworkRejectsInvalidGraph(t *testing.T) {
	for _, kind := range []string{"duplicate", "empty-id", "zero-count", "unknown-kind", "missing-cutoff", "foreign-order", "edge-limit"} {
		t.Run(kind, func(t *testing.T) {
			network := service.RiskNetwork{OrderID: "o", InputCutoffAt: time.Now(), Nodes: []service.NetworkNode{{ID: "n", Kind: "identity", Count: 1}}}
			switch kind {
			case "duplicate":
				network.Nodes = append(network.Nodes, network.Nodes[0])
			case "empty-id":
				network.Nodes[0].ID = ""
			case "zero-count":
				network.Nodes[0].Count = 0
			case "unknown-kind":
				network.Nodes[0].Kind = "private-phone"
			case "missing-cutoff":
				network.InputCutoffAt = time.Time{}
			case "foreign-order":
				network.OrderID = "foreign"
			case "edge-limit":
				network.Edges = make([]service.NetworkEdge, 601)
			}
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { json.NewEncoder(w).Encode(network) }))
			defer server.Close()
			client, _ := NewClient(server.URL)
			if _, err := client.Network(context.Background(), "s", "o"); err != service.ErrRiskUnavailable {
				t.Fatalf("invalid network accepted: %v", err)
			}
		})
	}
	disabled, _ := NewClient("")
	if _, err := disabled.Network(context.Background(), "s", "o"); err != service.ErrRiskUnavailable {
		t.Fatalf("disabled KBS returned network: %v", err)
	}
}
