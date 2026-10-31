package service

import (
	"context"
	"encoding/json"
	"github.com/tiloneeee/ai-fraud-detection/backend/internal/domain/entity"
	"time"
)

type RiskEvidenceInput struct {
	RecordID   string          `json:"record_id"`
	Kind       string          `json:"kind"`
	OccurredAt time.Time       `json:"occurred_at"`
	SourceRef  string          `json:"source_ref"`
	Data       json.RawMessage `json:"data"`
}

type RiskDataService interface {
	Record(ctx context.Context, actorID uint, input RiskEvidenceInput) error
	ListPlatform(ctx context.Context, status string, offset, limit int) ([]entity.PlatformRiskObservation, int64, error)
}
