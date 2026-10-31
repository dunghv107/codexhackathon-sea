package entity

import (
	"encoding/json"
	"time"
)

// Fulfillment statuses (orders.fulfillment_status)
const (
	FulfillmentUnfulfilled = "unfulfilled"
	FulfillmentPacking     = "packing"
	FulfillmentShipped     = "shipped"
	FulfillmentDelivered   = "delivered"
	FulfillmentCanceled    = "canceled"
	FulfillmentReturned    = "returned"
)

// Review statuses (orders.review_status): a gate before fulfillment, not a fraud verdict.
const (
	ReviewPending  = "pending"
	ReviewHold     = "hold"
	ReviewReleased = "released"
)

// Risk levels (risk_assessments.risk_level); RiskUnassessed is a filter value only.
const (
	RiskLow        = "low"
	RiskMedium     = "medium"
	RiskHigh       = "high"
	RiskUnassessed = "unassessed"
)

// Review decision actions
const (
	DecisionHold     = "hold"
	DecisionRelease  = "release"
	DecisionKeepHold = "keep_hold"
)

// Order event types and actor types used by the app
const (
	EventCreated           = "created"
	EventCanceled          = "canceled"
	EventDeliverySucceeded = "delivery_succeeded"

	ActorBuyer  = "buyer"
	ActorSeller = "seller"
)

// Payment statuses used by the app
const (
	PaymentPending   = "pending"
	PaymentSucceeded = "succeeded"
	PaymentMethodCOD = "cod"
)

// Checkout is one purchase across shops; Order rows split it per shop.
type Checkout struct {
	CheckoutID     string `gorm:"primaryKey"`
	BuyerID        string
	PlacedAt       time.Time
	PaymentMethod  string
	Currency       string
	TotalAmount    string
	BatchID        string
	SourceRecordID string
}

func (Checkout) TableName() string { return "checkouts" }

// CheckoutAddress snapshots the delivery address at checkout time.
type CheckoutAddress struct {
	CheckoutID     string `gorm:"primaryKey"`
	WardCode       *string
	HouseNo        *string
	ParseOK        *bool
	AddressRaw     *string
	AddressKey     *string
	PostalPrefix   *string
	City           *string
	Region         *string
	CountryCode    *string
	BatchID        string
	SourceRecordID string
}

func (CheckoutAddress) TableName() string { return "checkout_addresses" }

// Payment is a checkout-level payment line.
type Payment struct {
	PaymentID      string `gorm:"primaryKey"`
	CheckoutID     string
	Method         string
	Amount         string
	Status         string
	PaidAt         *time.Time
	BatchID        string
	SourceRecordID string
}

func (Payment) TableName() string { return "payments" }

// Order is one shop's part of a checkout; the unit of Hold/Release.
type Order struct {
	OrderID           string `gorm:"primaryKey"`
	CheckoutID        string
	ShopID            string
	FulfillmentStatus string
	ReviewStatus      string
	BatchID           string
	SourceRecordID    string
}

func (Order) TableName() string { return "orders" }

// OrderItem is a line with its price snapshot.
type OrderItem struct {
	ItemID         string `gorm:"primaryKey"`
	OrderID        string
	ShopID         string
	ListingID      string
	Quantity       int
	UnitPrice      string
	FreightAmount  string
	BatchID        string
	SourceRecordID string
}

func (OrderItem) TableName() string { return "order_items" }

// OrderEvent is a timeline entry; history is never derived from the current status.
type OrderEvent struct {
	EventID        string `gorm:"primaryKey"`
	OrderID        string
	EventType      string
	ActorType      string
	Reason         *string
	OccurredAt     time.Time
	AvailableAt    *time.Time
	BatchID        string
	SourceRecordID string
}

func (OrderEvent) TableName() string { return "order_events" }

// RiskAssessment is an immutable scoring snapshot.
type RiskAssessment struct {
	BatchID         string
	RuleConfig      json.RawMessage
	AssessmentID    string
	OrderID         string
	AssessedAt      time.Time
	InputCutoffAt   *time.Time
	RuleVersion     string
	Score           int
	RiskLevel       string
	FeatureSnapshot json.RawMessage
	Reasons         json.RawMessage
}

// ReviewDecision logs a change to the review gate.
type ReviewDecision struct {
	BatchID      string
	DecisionID   string `gorm:"primaryKey"`
	OrderID      string
	AssessmentID string
	ActorType    string
	ActorRef     string
	SellerID     *string
	Action       string
	Reason       string
	CreatedAt    time.Time
}

func (ReviewDecision) TableName() string { return "review_decisions" }

// OrderSummary is an order joined with checkout, shop, totals and latest assessment (read model).
type OrderSummary struct {
	BatchID           string
	IsSynthetic       bool
	SourceSystem      string
	OrderID           string
	CheckoutID        string
	PlacedAt          time.Time
	ShopID            string
	ShopName          string
	PaymentMethod     string
	PaymentStatus     string
	ItemCount         int64
	OrderTotal        string
	Currency          string
	ReviewStatus      string
	FulfillmentStatus string
	RiskLevel         *string
	Score             *int
}

// OrderFilter scopes order queries; BuyerID or ShopID must be set by the caller.
type OrderFilter struct {
	DisplayStatus     string
	BatchID           string
	BuyerID           string
	ShopID            string
	OrderID           string
	ReviewStatus      string
	FulfillmentStatus string
	RiskLevel         string
}

// OrderItemView is an order line with product info (read model)
type OrderItemView struct {
	ItemID         string
	ListingID      string
	ProductName    *string
	CategoryNameEn *string
	Quantity       int
	UnitPrice      string
	FreightAmount  string
}

// OrderDetail bundles everything an order page shows; callers pick what each role may see.
type OrderDetail struct {
	PhoneMasked  string
	DemoMode     bool
	Verification *OrderVerification
	OrderSummary
	Items            []OrderItemView
	Address          CheckoutAddress
	Events           []OrderEvent
	Cases            json.RawMessage
	LatestAssessment *RiskAssessment
	Decisions        []ReviewDecision
}

// StatusCount counts a shop's orders per status combination (read model)
type StatusCount struct {
	ReviewStatus      string
	FulfillmentStatus string
	RiskLevel         *string
	Count             int64
}

// DisplayStatus is the buyer-facing status; it hides the review gate behind "confirming".
func (o OrderSummary) DisplayStatus() string {
	if o.FulfillmentStatus == FulfillmentUnfulfilled {
		if o.ReviewStatus == ReviewReleased {
			return "confirmed"
		}
		return "confirming"
	}
	return o.FulfillmentStatus
}
