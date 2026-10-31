package entity

import (
	"time"

	"gorm.io/gorm"
)

// User roles
const (
	UserRoleUser        = "USER"
	UserRoleAdmin       = "ADMIN"
	UserRoleSystemAdmin = "SYSTEM_ADMIN"
)

// User statuses
const (
	UserStatusActive   = "ACTIVE"
	UserStatusInactive = "INACTIVE"
)

// User represents the user entity
type User struct {
	ID        uint           `gorm:"primaryKey" json:"id"`
	Username  string         `gorm:"uniqueIndex;size:50;not null" json:"username"`
	Email     string         `gorm:"uniqueIndex;size:255;not null" json:"email"`
	Password  string         `gorm:"size:255;not null" json:"-"`
	Role      string         `gorm:"size:20;default:'USER'" json:"role"`
	Status    string         `gorm:"size:20;default:'ACTIVE'" json:"status"`
	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
	DeletedAt gorm.DeletedAt `gorm:"index" json:"deleted_at,omitempty"`
}

// UserSession belongs to one login; only the hash of its refresh token is stored.
type UserSession struct {
	ID               string `gorm:"type:uuid;primaryKey"`
	UserID           uint
	RefreshTokenHash string
	ExpiresAt        time.Time
	CreatedAt        time.Time
}

// IsValidUserRole checks if the role is valid
func IsValidUserRole(role string) bool {
	switch role {
	case UserRoleUser, UserRoleAdmin, UserRoleSystemAdmin:
		return true
	default:
		return false
	}
}

// IsValidUserStatus checks if the status is valid
func IsValidUserStatus(status string) bool {
	switch status {
	case UserStatusActive, UserStatusInactive:
		return true
	default:
		return false
	}
}
