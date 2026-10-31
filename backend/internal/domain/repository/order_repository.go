package repository

import (
	"context"
	"encoding/json"

	"github.com/tiloneeee/ai-fraud-detection/backend/internal/domain/entity"
)

// OrderRepository covers checkouts, orders and their review/fulfillment history
type OrderRepository interface {
	CreateCheckout(ctx context.Context, checkout *entity.Checkout, addr *entity.CheckoutAddress, payment *entity.Payment, contact *entity.CheckoutContact) error
	CreateOrder(ctx context.Context, order *entity.Order, items []entity.OrderItem) error
	CreateEvent(ctx context.Context, event *entity.OrderEvent) error
	CreateDecision(ctx context.Context, decision *entity.ReviewDecision) error
	UpdateOrder(ctx context.Context, order *entity.Order, columns ...string) error

	// ListOrders is newest first; f.BuyerID scopes through checkouts.buyer_id, f.ShopID through orders.shop_id.
	ListOrders(ctx context.Context, f entity.OrderFilter, offset, limit int) ([]entity.OrderSummary, int64, error)
	// LockOrder selects the order FOR UPDATE within the same buyer/shop scope; NOT_FOUND otherwise.
	LockOrder(ctx context.Context, f entity.OrderFilter) (*entity.Order, error)
	ListItems(ctx context.Context, orderID string) ([]entity.OrderItemView, error)
	FindCheckoutAddress(ctx context.Context, checkoutID string) (*entity.CheckoutAddress, error)
	ListEvents(ctx context.Context, orderID string) ([]entity.OrderEvent, error)
	// FindLatestAssessment returns nil, nil when the order has not been assessed.
	FindLatestAssessment(ctx context.Context, orderID string) (*entity.RiskAssessment, error)
	ListCases(ctx context.Context, orderID string) (json.RawMessage, error)
	ListDecisions(ctx context.Context, orderID string) ([]entity.ReviewDecision, error)
	// PaymentReady reports whether the checkout is COD or has a succeeded payment.
	PaymentReady(ctx context.Context, checkoutID string) (bool, error)
	CountByStatus(ctx context.Context, shopID string) ([]entity.StatusCount, error)
}
