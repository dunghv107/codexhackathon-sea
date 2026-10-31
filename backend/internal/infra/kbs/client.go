package kbs

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/tiloneeee/ai-fraud-detection/backend/internal/usecase/service"
	apperror "github.com/tiloneeee/ai-fraud-detection/backend/pkg/error"
)

type Client struct {
	baseURL string
	http    *http.Client
}

func NewClient(baseURL string) (*Client, error) {
	if baseURL != "" {
		parsed, err := url.Parse(baseURL)
		if err != nil || parsed.Host == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" || (parsed.Path != "" && parsed.Path != "/") {
			return nil, errors.New("KBS_URL must be an http(s) origin without credentials or a path")
		}
	}
	return &Client{baseURL: strings.TrimRight(baseURL, "/"), http: &http.Client{
		Timeout:       10 * time.Second,
		CheckRedirect: func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse },
	}}, nil
}

func (c *Client) request(ctx context.Context, shopID, orderID, action string, result any) error {
	if c.baseURL == "" {
		return service.ErrRiskUnavailable
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/api/v1/orders/"+url.PathEscape(orderID)+"/"+action, nil)
	if err != nil {
		return service.ErrRiskUnavailable
	}
	req.Header.Set("X-Shop-ID", shopID)
	response, err := c.http.Do(req)
	if err != nil {
		return service.ErrRiskUnavailable
	}
	defer response.Body.Close()
	switch response.StatusCode {
	case http.StatusOK:
		if err := json.NewDecoder(io.LimitReader(response.Body, 1<<20)).Decode(result); err != nil {
			return service.ErrRiskUnavailable
		}
		return nil
	case http.StatusNotFound:
		return apperror.ErrNotFound.WithMessage("Không tìm thấy đơn trong phạm vi KBS")
	case http.StatusConflict:
		return service.ErrInvalidState.WithMessage("KBS chưa thể xử lý trạng thái hoặc phiên bản rule hiện tại")
	default:
		return service.ErrRiskUnavailable
	}
}

func (c *Client) Score(ctx context.Context, shopID, orderID string, force bool) error {
	var result struct {
		AssessmentID string `json:"assessment_id"`
		OrderID      string `json:"order_id"`
	}
	if err := c.request(ctx, shopID, orderID, "score?force="+strconv.FormatBool(force), &result); err != nil {
		return err
	}
	if result.AssessmentID == "" || result.OrderID != orderID {
		return service.ErrRiskUnavailable
	}
	return nil
}

func (c *Client) Explain(ctx context.Context, shopID, orderID string) (*service.RiskExplanation, error) {
	var result service.RiskExplanation
	if err := c.request(ctx, shopID, orderID, "explain", &result); err != nil {
		return nil, err
	}
	if result.AssessmentID == "" || len(result.Sentences) == 0 || len(result.Sentences) > 3 {
		return nil, service.ErrRiskUnavailable
	}
	return &result, nil
}
