package repository

import (
	"context"

	"github.com/tiloneeee/ai-fraud-detection/backend/internal/domain/entity"
)

// AddressRepository manages a buyer's saved addresses
type AddressRepository interface {
	ListByBuyer(ctx context.Context, buyerID string) ([]entity.SavedAddress, error)
	// FindOwned returns NOT_FOUND when the address does not belong to buyerID.
	FindOwned(ctx context.Context, buyerID, id string) (*entity.SavedAddress, error)
	Create(ctx context.Context, addr *entity.SavedAddress) error
	Update(ctx context.Context, addr *entity.SavedAddress) error
	Delete(ctx context.Context, buyerID, id string) error
	// ClearDefault locks the buyer row, then unsets is_default on every address of buyerID except exceptID.
	// Must run inside a transaction.
	ClearDefault(ctx context.Context, buyerID, exceptID string) error
}
