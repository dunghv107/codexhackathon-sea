package dto

import (
	"encoding/json"
	"github.com/tiloneeee/ai-fraud-detection/backend/internal/domain/entity"
	"time"
)

// BuyerProfileResponse is a buyer account summary
type BuyerProfileResponse struct {
	BuyerID     string  `json:"buyer_id"`
	DisplayName *string `json:"display_name"`
	Status      string  `json:"status"`
}

// SellerProfileResponse is a seller account summary
type SellerProfileResponse struct {
	SellerID    string `json:"seller_id"`
	DisplayName string `json:"display_name"`
	Status      string `json:"status"`
}

// ProfilesResponse lists the marketplace accounts linked to the login
type ProfilesResponse struct {
	Buyer  *BuyerProfileResponse  `json:"buyer"`
	Seller *SellerProfileResponse `json:"seller"`
}

// CategoryResponse represents a product category
type CategoryResponse struct {
	CategoryCode   string  `json:"category_code"`
	CategoryNameEn *string `json:"category_name_en"`
}

// ShopCardResponse represents a public shop
type ShopCardResponse struct {
	ShopID       string `json:"shop_id"`
	DisplayName  string `json:"display_name"`
	ListingCount int64  `json:"listing_count"`
}

// ShopResponse represents a seller's own shop
type ShopResponse struct {
	BatchID        string `json:"batch_id"`
	AssessedOrders int64  `json:"assessed_orders"`
	ShopID         string `json:"shop_id"`
	DisplayName    string `json:"display_name"`
}

// CreateShopRequest represents shop creation request
type CreateShopRequest struct {
	DisplayName string `json:"display_name" binding:"required"`
}

// ListingCardResponse is ListingCard (catalog) and SellerListing
type ListingCardResponse struct {
	Description    *string `json:"description"`
	ImageURL       *string `json:"image_url"`
	ListingID      string  `json:"listing_id"`
	ShopID         string  `json:"shop_id"`
	ShopName       string  `json:"shop_name"`
	ProductID      string  `json:"product_id"`
	ProductName    *string `json:"product_name"`
	CategoryCode   *string `json:"category_code"`
	CategoryNameEn *string `json:"category_name_en"`
	CurrentPrice   *string `json:"current_price"`
	Currency       string  `json:"currency"`
	StockQuantity  *int    `json:"stock_quantity"`
}

// CreateListingRequest represents listing creation request
type CreateListingRequest struct {
	Description   string `json:"description" binding:"max=5000"`
	ImageURL      string `json:"image_url" binding:"max=2048"`
	ProductName   string `json:"product_name" binding:"required"`
	CategoryCode  string `json:"category_code"`
	CurrentPrice  string `json:"current_price" binding:"required"`
	Currency      string `json:"currency"`
	StockQuantity *int   `json:"stock_quantity"`
}

// UpdateListingRequest represents listing patch request
type UpdateListingRequest struct {
	ProductName   *string `json:"product_name" binding:"omitempty,max=200"`
	Description   *string `json:"description" binding:"omitempty,max=5000"`
	ImageURL      *string `json:"image_url" binding:"omitempty,max=2048"`
	CurrentPrice  *string `json:"current_price"`
	StockQuantity *int    `json:"stock_quantity"`
}

// SavedAddressRequest represents address create/update request
type SavedAddressRequest struct {
	Label       string `json:"label" binding:"required"`
	AddressRaw  string `json:"address_raw" binding:"required"`
	Region      string `json:"region" binding:"required"`
	CountryCode string `json:"country_code" binding:"required,len=2"`
	IsDefault   bool   `json:"is_default"`
}

// SavedAddressResponse represents a saved address
type SavedAddressResponse struct {
	SavedAddressID string    `json:"saved_address_id"`
	Label          string    `json:"label"`
	AddressRaw     string    `json:"address_raw"`
	Region         string    `json:"region"`
	CountryCode    string    `json:"country_code"`
	IsDefault      bool      `json:"is_default"`
	CreatedAt      time.Time `json:"created_at"`
}

// CheckoutItemRequest is one requested line
type CheckoutItemRequest struct {
	ListingID string `json:"listing_id" binding:"required"`
	Quantity  int    `json:"quantity"`
}

// CheckoutRequest represents checkout request
type CheckoutRequest struct {
	Phone          string                `json:"phone" binding:"required,max=40"`
	SavedAddressID string                `json:"saved_address_id" binding:"required"`
	PaymentMethod  string                `json:"payment_method" binding:"required"`
	Items          []CheckoutItemRequest `json:"items" binding:"dive"`
}

// CheckoutOrderResponse is one order created by a checkout
type CheckoutOrderResponse struct {
	OrderID    string `json:"order_id"`
	ShopID     string `json:"shop_id"`
	ShopName   string `json:"shop_name"`
	OrderTotal string `json:"order_total"`
}

// CheckoutResponse represents checkout result
type CheckoutResponse struct {
	CheckoutID  string                  `json:"checkout_id"`
	PlacedAt    time.Time               `json:"placed_at"`
	TotalAmount string                  `json:"total_amount"`
	Currency    string                  `json:"currency"`
	Orders      []CheckoutOrderResponse `json:"orders"`
}

// ReasonRequest carries a free-text reason
type ReasonRequest struct {
	Reason string `json:"reason"`
}

// OrderItemResponse is an order line
type OrderItemResponse struct {
	ItemID         string  `json:"item_id"`
	ListingID      string  `json:"listing_id"`
	ProductName    *string `json:"product_name"`
	CategoryNameEn *string `json:"category_name_en"`
	Quantity       int     `json:"quantity"`
	UnitPrice      string  `json:"unit_price"`
	FreightAmount  string  `json:"freight_amount"`
}

// OrderAddressResponse is the delivery address snapshot
type OrderAddressResponse struct {
	WardCode     *string `json:"ward_code"`
	HouseNo      *string `json:"house_no"`
	ParseOK      *bool   `json:"parse_ok"`
	AddressRaw   *string `json:"address_raw"`
	City         *string `json:"city"`
	Region       *string `json:"region"`
	PostalPrefix *string `json:"postal_prefix"`
	CountryCode  *string `json:"country_code"`
}

// OrderPaymentResponse exposes only method and status, never amounts
type OrderPaymentResponse struct {
	Method string `json:"method"`
	Status string `json:"status"`
}

// BuyerOrderSummary never carries review_status, score or risk_level.
type BuyerOrderSummary struct {
	OrderID           string    `json:"order_id"`
	CheckoutID        string    `json:"checkout_id"`
	PlacedAt          time.Time `json:"placed_at"`
	ShopID            string    `json:"shop_id"`
	ShopName          string    `json:"shop_name"`
	ItemCount         int64     `json:"item_count"`
	OrderTotal        string    `json:"order_total"`
	Currency          string    `json:"currency"`
	FulfillmentStatus string    `json:"fulfillment_status"`
	DisplayStatus     string    `json:"display_status"`
}

// BuyerOrderEvent is a buyer-visible timeline entry
type BuyerOrderEvent struct {
	EventType  string    `json:"event_type"`
	OccurredAt time.Time `json:"occurred_at"`
}

// BuyerOrderDetail represents an order seen by its buyer
type BuyerOrderDetail struct {
	BuyerOrderSummary
	Items   []OrderItemResponse  `json:"items"`
	Address OrderAddressResponse `json:"address"`
	Payment OrderPaymentResponse `json:"payment"`
	Events  []BuyerOrderEvent    `json:"events"`
}

// SellerOrderSummary represents an order in the seller's list
type SellerOrderSummary struct {
	BatchID           string    `json:"batch_id"`
	IsSynthetic       bool      `json:"is_synthetic"`
	SourceSystem      string    `json:"source_system"`
	OrderID           string    `json:"order_id"`
	CheckoutID        string    `json:"checkout_id"`
	BuyerID           string    `json:"buyer_id"`
	BuyerDisplayName  string    `json:"buyer_display_name"`
	PlacedAt          time.Time `json:"placed_at"`
	PaymentMethod     string    `json:"payment_method"`
	ItemCount         int64     `json:"item_count"`
	OrderTotal        string    `json:"order_total"`
	Currency          string    `json:"currency"`
	ReviewStatus      string    `json:"review_status"`
	FulfillmentStatus string    `json:"fulfillment_status"`
	RiskLevel         *string   `json:"risk_level"`
	Score             *int      `json:"score"`
}

// SellerOrderEvent is a full timeline entry
type SellerOrderEvent struct {
	AvailableAt *time.Time `json:"available_at"`
	EventType   string     `json:"event_type"`
	ActorType   string     `json:"actor_type"`
	Reason      *string    `json:"reason"`
	OccurredAt  time.Time  `json:"occurred_at"`
}

// AssessmentResponse is the stored risk snapshot, never recomputed
type AssessmentResponse struct {
	RuleConfig      json.RawMessage `json:"rule_config"`
	AssessmentID    string          `json:"assessment_id"`
	AssessedAt      time.Time       `json:"assessed_at"`
	InputCutoffAt   *time.Time      `json:"input_cutoff_at"`
	RuleVersion     string          `json:"rule_version"`
	Score           int             `json:"score"`
	RiskLevel       string          `json:"risk_level"`
	FeatureSnapshot json.RawMessage `json:"feature_snapshot"`
	Reasons         json.RawMessage `json:"reasons"`
}

// DecisionResponse is one review decision
type DecisionResponse struct {
	DecisionID   string    `json:"decision_id"`
	AssessmentID string    `json:"assessment_id"`
	ActorType    string    `json:"actor_type"`
	Action       string    `json:"action"`
	Reason       string    `json:"reason"`
	CreatedAt    time.Time `json:"created_at"`
}

// SellerOrderDetail represents an order seen by the shop's seller
type SellerOrderDetail struct {
	PhoneMasked  string                    `json:"phone_masked"`
	DemoMode     bool                      `json:"demo_mode"`
	Verification *entity.OrderVerification `json:"verification"`
	Cases        json.RawMessage           `json:"cases"`
	SellerOrderSummary
	Items            []OrderItemResponse  `json:"items"`
	Address          OrderAddressResponse `json:"address"`
	Payment          OrderPaymentResponse `json:"payment"`
	Events           []SellerOrderEvent   `json:"events"`
	LatestAssessment *AssessmentResponse  `json:"latest_assessment"`
	Decisions        []DecisionResponse   `json:"decisions"`
}

// DecisionRequest represents a seller review decision
type DecisionRequest struct {
	AssessmentID string `json:"assessment_id" binding:"required"`
	Action       string `json:"action" binding:"required"`
	Reason       string `json:"reason"`
}

// FulfillmentRequest represents a fulfillment status change
type FulfillmentRequest struct {
	Status string `json:"status" binding:"required"`
}

// DashboardResponse counts a shop's orders per status dimension
type DashboardResponse struct {
	ReviewStatus      map[string]int64 `json:"review_status"`
	FulfillmentStatus map[string]int64 `json:"fulfillment_status"`
	RiskLevel         map[string]int64 `json:"risk_level"`
}
