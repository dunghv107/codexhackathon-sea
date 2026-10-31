package repository

import (
	"context"

	"github.com/tiloneeee/ai-fraud-detection/backend/internal/domain/entity"
)

// ProfileRepository resolves buyer/seller accounts by auth_subject
type ProfileRepository interface {
	FindBuyerByAuthSubject(ctx context.Context, subject string) (*entity.Buyer, error)
	FindSellerByAuthSubject(ctx context.Context, subject string) (*entity.Seller, error)
}
