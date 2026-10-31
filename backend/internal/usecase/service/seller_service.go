package service

import (
	"context"

	"github.com/tiloneeee/ai-fraud-detection/backend/internal/domain/entity"
)

// CreateListingCommand creates a new product and its listing in one shop
type CreateListingCommand struct {
	ShopID        string
	ProductName   string
	CategoryCode  string
	CurrentPrice  string
	Currency      string
	StockQuantity *int
}

// UpdateListingCommand changes only the given fields
type UpdateListingCommand struct {
	ShopID        string
	ListingID     string
	CurrentPrice  *string
	StockQuantity *int
}

// DecisionCommand releases or keeps holding an order against one assessment
type DecisionCommand struct {
	SellerID     string
	ShopID       string
	OrderID      string
	AssessmentID string
	Action       string
	Reason       string
}

// FulfillmentCommand moves an order to a new fulfillment status
type FulfillmentCommand struct {
	ShopID  string
	OrderID string
	Status  string
}

// Dashboard counts a shop's orders by each status dimension
type Dashboard struct {
	ReviewStatus      map[string]int64
	FulfillmentStatus map[string]int64
	RiskLevel         map[string]int64
}

// SellerService defines seller portal operations; shop ownership is checked before these are called.
type SellerService interface {
	Network(ctx context.Context, shopID, orderID string) (*RiskNetwork, error)
	GetVerification(ctx context.Context, shopID, orderID string) (*VerificationView, error)
	RespondVerification(ctx context.Context, shopID, orderID, sellerID, verificationID, key string) (*entity.OrderVerification, error)
	ListShops(ctx context.Context, sellerID string) ([]entity.Shop, error)
	CreateShop(ctx context.Context, sellerID, displayName string) (*entity.Shop, error)
	// GetOwnedShop returns NOT_FOUND unless sellerID owns shopID.
	GetOwnedShop(ctx context.Context, sellerID, shopID string) (*entity.Shop, error)
	GetDashboard(ctx context.Context, shopID string) (*Dashboard, error)

	ListListings(ctx context.Context, shopID string, offset, limit int) ([]entity.ListingCard, int64, error)
	CreateListing(ctx context.Context, cmd CreateListingCommand) (*entity.ListingCard, error)
	UpdateListing(ctx context.Context, cmd UpdateListingCommand) (*entity.ListingCard, error)

	// ListOrders requires f.ShopID.
	ListOrders(ctx context.Context, f entity.OrderFilter, offset, limit int) ([]entity.OrderSummary, int64, error)
	GetOrder(ctx context.Context, shopID, orderID string) (*entity.OrderDetail, error)
	Score(ctx context.Context, shopID, orderID string, force bool) (*entity.OrderDetail, error)
	Explain(ctx context.Context, shopID, orderID string) (*RiskExplanation, error)
	Decide(ctx context.Context, cmd DecisionCommand) (*entity.OrderDetail, error)
	UpdateFulfillment(ctx context.Context, cmd FulfillmentCommand) (*entity.OrderDetail, error)
}

type VerificationView struct {
	Verification *entity.OrderVerification `json:"verification"`
	PhoneMasked  string                    `json:"phone_masked"`
}
