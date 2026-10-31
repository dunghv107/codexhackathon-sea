package kbs

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"time"

	"github.com/tiloneeee/ai-fraud-detection/backend/internal/usecase/service"
)

func (c *Client) VoicePreview(ctx context.Context, shopID, orderID string, input service.VoiceInput) (*service.VoiceResult, error) {
	if c.baseURL == "" {
		return nil, service.ErrVoiceUnavailable
	}
	body, err := json.Marshal(input)
	if err != nil {
		return nil, service.ErrVoiceUnavailable
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/api/v1/orders/"+url.PathEscape(orderID)+"/voice-preview", bytes.NewReader(body))
	if err != nil {
		return nil, service.ErrVoiceUnavailable
	}
	req.Header.Set("X-Shop-ID", shopID)
	req.Header.Set("Content-Type", "application/json")
	client := *c.http
	client.Timeout = 28 * time.Second
	res, err := client.Do(req)
	if err != nil {
		return nil, service.ErrVoiceUnavailable
	}
	defer res.Body.Close()
	var result service.VoiceResult
	if res.StatusCode != http.StatusOK || json.NewDecoder(io.LimitReader(res.Body, 16384)).Decode(&result) != nil {
		return nil, service.ErrVoiceUnavailable
	}
	if (result.Intent != "confirmed" && result.Intent != "declined" && result.Intent != "unclear") || len(result.Transcript) > 8000 || (result.Intent != "unclear" && result.Transcript == "") {
		return nil, service.ErrVoiceUnavailable
	}
	return &result, nil
}
