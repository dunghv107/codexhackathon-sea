package service

import (
	"context"

	"github.com/tiloneeee/ai-fraud-detection/backend/internal/domain/entity"
)

// SaveAddressCommand creates (ID empty) or updates a saved address
type SaveAddressCommand struct {
	BuyerID     string
	ID          string
	Label       string
	AddressRaw  string
	Region      string
	CountryCode string
	IsDefault   bool
}

// CheckoutItem is one requested line
type CheckoutItem struct {
	ListingID string
	Quantity  int
}

// CheckoutCommand places one checkout split into one order per shop
type CheckoutCommand struct {
	BuyerID        string
	SavedAddressID string
	PaymentMethod  string
	Items          []CheckoutItem
}

// CheckoutOrder is one created order in the checkout result
type CheckoutOrder struct {
	OrderID    string
	ShopID     string
	ShopName   string
	OrderTotal string
}

// CheckoutResult is the created checkout and its orders
type CheckoutResult struct {
	Checkout entity.Checkout
	Orders   []CheckoutOrder
}

// BuyerService defines buyer portal operations; every call is scoped by BuyerID from the session.
type BuyerService interface {
	ListAddresses(ctx context.Context, buyerID string) ([]entity.SavedAddress, error)
	SaveAddress(ctx context.Context, cmd SaveAddressCommand) (*entity.SavedAddress, error)
	DeleteAddress(ctx context.Context, buyerID, id string) error

	Checkout(ctx context.Context, cmd CheckoutCommand) (*CheckoutResult, error)
	ListOrders(ctx context.Context, buyerID string, offset, limit int) ([]entity.OrderSummary, int64, error)
	GetOrder(ctx context.Context, buyerID, orderID string) (*entity.OrderDetail, error)
	CancelOrder(ctx context.Context, buyerID, orderID, reason string) (*entity.OrderDetail, error)
}
