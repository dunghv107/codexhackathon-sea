package persistence

import (
	"context"

	"gorm.io/gorm"

	"github.com/tiloneeee/ai-fraud-detection/backend/internal/domain/entity"
	"github.com/tiloneeee/ai-fraud-detection/backend/internal/domain/repository"
)

type profileRepositoryImpl struct {
	db *gorm.DB
}

// NewProfileRepository creates a new profile repository
func NewProfileRepository(db *gorm.DB) repository.ProfileRepository {
	return &profileRepositoryImpl{db: db}
}

func (r *profileRepositoryImpl) FindBuyerByAuthSubject(ctx context.Context, subject string) (*entity.Buyer, error) {
	var buyer entity.Buyer
	if err := dbFrom(ctx, r.db).Where("auth_subject = ?", subject).First(&buyer).Error; err != nil {
		return nil, wrapFindError(err, "người mua")
	}
	return &buyer, nil
}

func (r *profileRepositoryImpl) FindSellerByAuthSubject(ctx context.Context, subject string) (*entity.Seller, error) {
	var seller entity.Seller
	if err := dbFrom(ctx, r.db).Where("auth_subject = ?", subject).First(&seller).Error; err != nil {
		return nil, wrapFindError(err, "người bán")
	}
	return &seller, nil
}
