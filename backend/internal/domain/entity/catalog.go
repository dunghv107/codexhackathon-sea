package entity

// Shop belongs to exactly one seller.
type Shop struct {
	ShopID         string `gorm:"primaryKey"`
	DisplayName    string
	OwnerSellerID  *string
	OwnerActorRef  *string
	AssessedOrders int64 `gorm:"->;-:migration"`
	BatchID        string
	SourceRecordID string
}

func (Shop) TableName() string { return "shops" }

// Product holds catalog info; Olist products have no name.
type Product struct {
	ProductID      string `gorm:"primaryKey"`
	DisplayName    *string
	CategoryCode   *string
	CategoryNameEn *string
	BatchID        string
	SourceRecordID string
}

func (Product) TableName() string { return "products" }

// Listing is a product sold by a shop. Money columns are numeric(18,2) carried as "123.45" strings.
type Listing struct {
	Title          *string
	Description    *string
	ImageURL       *string
	ListingID      string `gorm:"primaryKey"`
	ShopID         string
	ProductID      string
	CurrentPrice   *string
	Currency       string
	StockQuantity  *int
	BatchID        string
	SourceRecordID string
}

func (Listing) TableName() string { return "listings" }

// Category is a distinct product category (read model)
type Category struct {
	CategoryCode   string
	CategoryNameEn *string
}

// ShopCard is a shop with its count of priced listings (read model)
type ShopCard struct {
	ShopID       string
	DisplayName  string
	ListingCount int64
}

// ListingCard joins listing, shop and product (read model)
type ListingCard struct {
	Description    *string
	ImageURL       *string
	BatchID        string
	ShopBatchID    string
	ProductBatchID string
	ListingID      string
	ShopID         string
	ShopName       string
	ProductID      string
	ProductName    *string
	CategoryCode   *string
	CategoryNameEn *string
	CurrentPrice   *string
	Currency       string
	StockQuantity  *int
}

// ListingFilter narrows listing queries; empty fields are ignored.
type ListingFilter struct {
	Query        string
	Sort         string
	Currency     string
	InStock      bool
	ShopID       string
	CategoryCode string
	ListingID    string
	PricedOnly   bool
}
