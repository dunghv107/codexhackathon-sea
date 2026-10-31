package kbs

import (
	"context"
	"fmt"
	"github.com/tiloneeee/ai-fraud-detection/backend/internal/usecase/service"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestVoicePreviewValidatesProviderResult(t *testing.T) {
	for _, intent := range []string{"confirmed", "declined", "unclear", "release"} {
		t.Run(intent, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Header.Get("X-Shop-ID") != "shop" || r.URL.Path != "/api/v1/orders/order/voice-preview" {
					t.Error("Wrong ownership boundary")
				}
				fmt.Fprintf(w, `{"transcript":"test reply","intent":%q}`, intent)
			}))
			defer server.Close()
			client, _ := NewClient(server.URL)
			result, err := client.VoicePreview(context.Background(), "shop", "order", service.VoiceInput{Audio: "YQ==", MimeType: "audio/wav"})
			if intent == "release" {
				if err == nil {
					t.Fatal("Accepted unknown intention")
				}
				return
			}
			if err != nil || result.Intent != intent {
				t.Fatalf("Unexpected result: %v", err)
			}
		})
	}
}
