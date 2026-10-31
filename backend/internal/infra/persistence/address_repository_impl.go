package persistence

import (
	"context"

	"gorm.io/gorm"

	"github.com/tiloneeee/ai-fraud-detection/backend/internal/domain/entity"
	"github.com/tiloneeee/ai-fraud-detection/backend/internal/domain/repository"
	apperror "github.com/tiloneeee/ai-fraud-detection/backend/pkg/error"
)

type addressRepositoryImpl struct {
	db *gorm.DB
}

// NewAddressRepository creates a new saved-address repository
func NewAddressRepository(db *gorm.DB) repository.AddressRepository {
	return &addressRepositoryImpl{db: db}
}

func (r *addressRepositoryImpl) ListByBuyer(ctx context.Context, buyerID string) ([]entity.SavedAddress, error) {
	var addrs []entity.SavedAddress
	if err := dbFrom(ctx, r.db).Where("buyer_id = ?", buyerID).
		Order("is_default DESC, created_at DESC").Find(&addrs).Error; err != nil {
		return nil, wrapListError(err, "địa chỉ")
	}
	return addrs, nil
}

func (r *addressRepositoryImpl) FindOwned(ctx context.Context, buyerID, id string) (*entity.SavedAddress, error) {
	var addr entity.SavedAddress
	if err := dbFrom(ctx, r.db).Where("saved_address_id = ? AND buyer_id = ?", id, buyerID).First(&addr).Error; err != nil {
		return nil, wrapFindError(err, "địa chỉ")
	}
	return &addr, nil
}

func (r *addressRepositoryImpl) Create(ctx context.Context, addr *entity.SavedAddress) error {
	if err := dbFrom(ctx, r.db).Create(addr).Error; err != nil {
		return wrapCreateError(err, "địa chỉ")
	}
	return nil
}

func (r *addressRepositoryImpl) Update(ctx context.Context, addr *entity.SavedAddress) error {
	if err := dbFrom(ctx, r.db).Model(addr).
		Select("label", "address_raw", "address_key", "region", "country_code", "is_default").
		Updates(addr).Error; err != nil {
		return wrapUpdateError(err, "địa chỉ")
	}
	return nil
}

func (r *addressRepositoryImpl) Delete(ctx context.Context, buyerID, id string) error {
	res := dbFrom(ctx, r.db).Where("saved_address_id = ? AND buyer_id = ?", id, buyerID).Delete(&entity.SavedAddress{})
	if res.Error != nil {
		return wrapDeleteError(res.Error, "địa chỉ")
	}
	if res.RowsAffected == 0 {
		return apperror.ErrNotFound.WithMessage("Không tìm thấy địa chỉ")
	}
	return nil
}

func (r *addressRepositoryImpl) ClearDefault(ctx context.Context, buyerID, exceptID string) error {
	// Lock the buyer row so concurrent default changes run one at a time; otherwise a
	// waiting tx misses the other's new default row and trips the one-default index.
	if err := dbFrom(ctx, r.db).Exec("SELECT 1 FROM buyers WHERE buyer_id = ? FOR UPDATE", buyerID).Error; err != nil {
		return err
	}
	if err := dbFrom(ctx, r.db).Model(&entity.SavedAddress{}).
		Where("buyer_id = ? AND is_default AND saved_address_id <> ?", buyerID, exceptID).
		Update("is_default", false).Error; err != nil {
		return wrapUpdateError(err, "địa chỉ")
	}
	return nil
}
