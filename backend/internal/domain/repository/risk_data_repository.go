package repository

import (
	"context"
	"github.com/tiloneeee/ai-fraud-detection/backend/internal/domain/entity"
)

type RiskDataRepository interface {
	Record(ctx context.Context, record *entity.RiskEvidence) error
	ListPlatform(ctx context.Context, status string, offset, limit int) ([]entity.PlatformRiskObservation, int64, error)
}
