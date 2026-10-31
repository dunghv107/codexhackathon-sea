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

func (c *Client) request(ctx context.Context, shopID, orderID, method, action string, result any) error {
	if c.baseURL == "" {
		return service.ErrRiskUnavailable
	}
	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+"/api/v1/orders/"+url.PathEscape(orderID)+"/"+action, nil)
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

func (c *Client) Score(ctx context.Context, shopID, orderID string, force bool) (*service.ScoreAssessment, error) {
	var result struct {
		service.ScoreAssessment
		Score *int `json:"score"`
	}
	if err := c.request(ctx, shopID, orderID, http.MethodPost, "score?force="+strconv.FormatBool(force), &result); err != nil {
		return nil, err
	}
	if result.AssessmentID == "" || result.OrderID != orderID || result.Score == nil || *result.Score < 0 || *result.Score > 100 || (result.RiskLevel != "low" && result.RiskLevel != "medium" && result.RiskLevel != "high") {
		return nil, service.ErrRiskUnavailable
	}
	result.ScoreAssessment.Score = *result.Score
	return &result.ScoreAssessment, nil
}

func (c *Client) Explain(ctx context.Context, shopID, orderID string) (*service.RiskExplanation, error) {
	var result service.RiskExplanation
	if err := c.request(ctx, shopID, orderID, http.MethodPost, "explain", &result); err != nil {
		return nil, err
	}
	if result.AssessmentID == "" || len(result.Sentences) == 0 || len(result.Sentences) > 3 {
		return nil, service.ErrRiskUnavailable
	}
	return &result, nil
}

func (c *Client) Network(ctx context.Context, shopID, orderID string) (*service.RiskNetwork, error) {
	var result service.RiskNetwork
	if err := c.request(ctx, shopID, orderID, http.MethodGet, "network", &result); err != nil {
		return nil, err
	}
	if result.OrderID != orderID || result.InputCutoffAt.IsZero() || len(result.Nodes) == 0 || len(result.Nodes) > 200 || len(result.Edges) > 600 {
		return nil, service.ErrRiskUnavailable
	}
	ids := map[string]bool{}
	for _, node := range result.Nodes {
		if node.ID == "" || ids[node.ID] || node.Count < 1 || (node.Kind != "checkout" && node.Kind != "identity" && node.Kind != "aggregate") {
			return nil, service.ErrRiskUnavailable
		}
		ids[node.ID] = true
	}
	for _, edge := range result.Edges {
		if !ids[edge.Source] || !ids[edge.Target] {
			return nil, service.ErrRiskUnavailable
		}
	}
	return &result, nil
}
