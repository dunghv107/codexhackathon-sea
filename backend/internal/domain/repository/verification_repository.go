package repository

import (
	"context"
	"github.com/tiloneeee/ai-fraud-detection/backend/internal/domain/entity"
)

type VerificationRepository interface {
	Find(ctx context.Context, orderID string) (*entity.OrderVerification, error)
	Save(ctx context.Context, verification *entity.OrderVerification) error
	IsCOD(ctx context.Context, checkoutID string) (bool, error)
	MaskedPhone(ctx context.Context, checkoutID string) (string, error)
	DemoPhoneKey(ctx context.Context, phone string) (string, error)
}
