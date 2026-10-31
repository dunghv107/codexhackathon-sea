package service

import (
	"context"
	"net/http"

	apperror "github.com/tiloneeee/ai-fraud-detection/backend/pkg/error"
)

// RiskClient calls the internal KBS; scores and policy holds remain owned by KBS.
type RiskClient interface {
	Score(ctx context.Context, shopID, orderID string, force bool) error
	Explain(ctx context.Context, shopID, orderID string) (*RiskExplanation, error)
}

type RiskExplanation struct {
	AssessmentID string   `json:"assessment_id"`
	Source       string   `json:"source"`
	Sentences    []string `json:"sentences"`
}

var ErrRiskUnavailable = &apperror.AppError{
	Code: "KBS_UNAVAILABLE", Message: "Không thể chấm điểm lúc này; đơn đã lưu có thể được chấm lại", HTTPStatus: http.StatusServiceUnavailable,
}
var ErrRiskBatch = &apperror.AppError{
	Code: "UNSUPPORTED_RISK_BATCH", Message: "Thao tác này chỉ hỗ trợ dữ liệu demo olist_vn_sim; dữ liệu cũ không bị đổi batch", HTTPStatus: http.StatusConflict,
}
