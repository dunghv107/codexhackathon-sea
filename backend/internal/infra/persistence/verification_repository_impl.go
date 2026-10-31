package persistence

import (
	"context"
	"errors"

	"github.com/tiloneeee/ai-fraud-detection/backend/internal/domain/entity"
	"github.com/tiloneeee/ai-fraud-detection/backend/internal/domain/repository"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

type verificationRepository struct{ db *gorm.DB }

func NewVerificationRepository(db *gorm.DB) repository.VerificationRepository {
	return &verificationRepository{db}
}

func (r *verificationRepository) Find(ctx context.Context, orderID string) (*entity.OrderVerification, error) {
	var row entity.OrderVerification
	err := dbFrom(ctx, r.db).Where("order_id = ? AND batch_id = ?", orderID, entity.DemoBatchID).Take(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	return &row, err
}

func (r *verificationRepository) Save(ctx context.Context, v *entity.OrderVerification) error {
	return dbFrom(ctx, r.db).Save(v).Error
}

func (r *verificationRepository) IsCOD(ctx context.Context, checkoutID string) (bool, error) {
	var count int64
	err := dbFrom(ctx, r.db).Table("checkouts").Where("checkout_id = ? AND batch_id = ? AND payment_method = 'cod'", checkoutID, entity.DemoBatchID).Count(&count).Error
	return count == 1, err
}

func (r *verificationRepository) MaskedPhone(ctx context.Context, checkoutID string) (string, error) {
	var masked string
	err := dbFrom(ctx, r.db).Table("checkout_contacts").Select("'***' || right(phone_e164, 4)").Where("checkout_id = ? AND batch_id = ?", checkoutID, entity.DemoBatchID).Scan(&masked).Error
	return masked, err
}

func (r *verificationRepository) DemoPhoneKey(ctx context.Context, phone string) (string, error) {
	var key string
	// Do not interpolate raw contact into SQL logs, including slow/error queries.
	err := dbFrom(ctx, r.db).Session(&gorm.Session{Logger: logger.Default.LogMode(logger.Silent)}).Table("demo_phone_mappings m").Select("m.phone_key").
		Where("m.batch_id = ? AND m.phone_e164 = ?", entity.DemoBatchID, phone).
		Where("EXISTS (SELECT 1 FROM checkout_identity_snapshots i WHERE i.batch_id = m.batch_id AND i.phone_key = m.phone_key)").Scan(&key).Error
	return key, err
}
