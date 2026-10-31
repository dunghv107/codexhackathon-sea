package service

import (
	"context"

	"github.com/tiloneeee/ai-fraud-detection/backend/internal/domain/entity"
)

// Profiles holds the buyer/seller accounts linked to one login; either may be nil.
type Profiles struct {
	Buyer  *entity.Buyer
	Seller *entity.Seller
}

// ProfileService resolves marketplace accounts from the JWT user id (auth_subject = "local:<id>")
type ProfileService interface {
	GetProfiles(ctx context.Context, userID uint) (*Profiles, error)
	// ResolveBuyer returns ErrNoBuyerProfile when the login has no buyer account.
	ResolveBuyer(ctx context.Context, userID uint) (*entity.Buyer, error)
	// ResolveSeller returns ErrNoSellerProfile when the login has no seller account.
	ResolveSeller(ctx context.Context, userID uint) (*entity.Seller, error)
}
