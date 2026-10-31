package entity

import "time"

// Account statuses (buyers.status, sellers.status)
const (
	AccountStatusActive    = "active"
	AccountStatusSuspended = "suspended"
)

// AppBatchID marks rows created through this API; their IDs are "app:<uuid>".
const AppBatchID = "app"

// Buyer is a purchasing account; AuthSubject = "local:<users.id>" links it to a login.
type Buyer struct {
	BuyerID          string `gorm:"primaryKey"`
	AuthSubject      *string
	Status           string
	DisplayName      *string
	AccountCreatedAt *time.Time
	BatchID          string
	SourceRecordID   string
}

func (Buyer) TableName() string { return "buyers" }

// Seller is a merchant account that owns shops.
type Seller struct {
	SellerID         string `gorm:"primaryKey"`
	AuthSubject      *string
	DisplayName      string
	AccountCreatedAt *time.Time
	Status           string
	BatchID          string
	SourceRecordID   string
}

func (Seller) TableName() string { return "sellers" }

// SavedAddress is the buyer's current address book; orders keep their own snapshot.
type SavedAddress struct {
	SavedAddressID string `gorm:"primaryKey"`
	BuyerID        string
	Label          string
	AddressRaw     string
	AddressKey     string
	Region         string
	CountryCode    string
	IsDefault      bool
	CreatedAt      time.Time
	BatchID        string
}

func (SavedAddress) TableName() string { return "saved_addresses" }
