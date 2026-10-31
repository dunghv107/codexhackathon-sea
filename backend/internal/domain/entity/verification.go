package entity

import "time"

// Contact is server-only; never serialize it into a portal response.
type CheckoutContact struct {
	CheckoutID string    `json:"-"`
	BatchID    string    `json:"-"`
	PhoneE164  string    `json:"-"`
	KeySource  string    `json:"-"`
	CreatedAt  time.Time `json:"-"`
	PhoneKey   string    `gorm:"-" json:"-"`
}

func (CheckoutContact) TableName() string { return "checkout_contacts" }

type OrderVerification struct {
	VerificationID    string     `gorm:"primaryKey" json:"verification_id"`
	BatchID           string     `json:"-"`
	OrderID           string     `json:"-"`
	AssessmentID      string     `json:"assessment_id"`
	Status            string     `json:"status"`
	Channel           string     `json:"channel"`
	ResponseSource    *string    `json:"response_source"`
	AnsweredAt        *time.Time `json:"answered_at"`
	HandledBySellerID *string    `json:"-"`
	CreatedAt         time.Time  `json:"created_at"`
	UpdatedAt         time.Time  `json:"updated_at"`
}

func (OrderVerification) TableName() string { return "order_verifications" }
