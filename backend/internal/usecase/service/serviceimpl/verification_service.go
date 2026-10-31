package serviceimpl

import (
	"context"
	"time"

	"github.com/tiloneeee/ai-fraud-detection/backend/internal/domain/entity"
	"github.com/tiloneeee/ai-fraud-detection/backend/internal/domain/repository"
	"github.com/tiloneeee/ai-fraud-detection/backend/internal/usecase/service"
	apperror "github.com/tiloneeee/ai-fraud-detection/backend/pkg/error"
)

type VerificationService struct {
	repo    repository.VerificationRepository
	orders  repository.OrderRepository
	tx      repository.Transactor
	Enabled bool
}

func NewVerificationService(repo repository.VerificationRepository, orders repository.OrderRepository, tx repository.Transactor, enabled bool) *VerificationService {
	return &VerificationService{repo: repo, orders: orders, tx: tx, Enabled: enabled}
}

func verificationEligible(order *entity.Order, latest *entity.RiskAssessment, cod bool) bool {
	return order.BatchID == entity.DemoBatchID && cod && latest != nil && latest.RiskLevel == "medium" &&
		order.FulfillmentStatus == entity.FulfillmentUnfulfilled && (order.ReviewStatus == entity.ReviewPending || order.ReviewStatus == entity.ReviewHold)
}

func (s *VerificationService) EnsureForOrder(ctx context.Context, shopID, orderID string) error {
	if !s.Enabled {
		return nil
	}
	return s.tx.WithinTransaction(ctx, func(ctx context.Context) error {
		order, err := s.orders.LockOrder(ctx, entity.OrderFilter{ShopID: shopID, OrderID: orderID})
		if err != nil {
			return err
		}
		latest, err := s.orders.FindLatestAssessment(ctx, orderID)
		if err != nil {
			return err
		}
		cod, err := s.repo.IsCOD(ctx, order.CheckoutID)
		if err != nil {
			return err
		}
		if !verificationEligible(order, latest, cod) {
			return nil
		}
		row, err := s.repo.Find(ctx, orderID)
		if err != nil {
			return err
		}
		if row != nil && row.Status != "pending" {
			return nil
		}
		now := time.Now()
		if row == nil {
			row = &entity.OrderVerification{VerificationID: newAppID(), BatchID: entity.DemoBatchID, OrderID: orderID, Status: "pending", Channel: "demo", CreatedAt: now}
		}
		row.AssessmentID, row.UpdatedAt = latest.AssessmentID, now
		return s.repo.Save(ctx, row)
	})
}

func (s *VerificationService) Respond(ctx context.Context, shopID, orderID, sellerID, verificationID, key string) (*entity.OrderVerification, error) {
	if !s.Enabled {
		return nil, apperror.ErrNotFound
	}
	next := map[string]string{"1": "confirmed", "2": "declined", "0": "unanswered"}[key]
	if next == "" || verificationID == "" {
		return nil, apperror.ErrValidation
	}
	var row *entity.OrderVerification
	err := s.tx.WithinTransaction(ctx, func(ctx context.Context) error {
		// All verification writes take this order lock, including EnsureForOrder.
		order, err := s.orders.LockOrder(ctx, entity.OrderFilter{ShopID: shopID, OrderID: orderID})
		if err != nil {
			return err
		}
		row, err = s.repo.Find(ctx, orderID)
		if err != nil {
			return err
		}
		if row == nil || row.VerificationID != verificationID {
			return apperror.ErrNotFound
		}
		if row.Status != "pending" {
			if row.Status == next {
				return nil
			}
			return service.ErrInvalidState
		}
		latest, err := s.orders.FindLatestAssessment(ctx, orderID)
		if err != nil {
			return err
		}
		cod, err := s.repo.IsCOD(ctx, order.CheckoutID)
		if err != nil {
			return err
		}
		if !verificationEligible(order, latest, cod) {
			return service.ErrInvalidState
		}
		if row.AssessmentID != latest.AssessmentID {
			return service.ErrStaleAssessment
		}
		now, source := time.Now(), "dtmf"
		row.Status, row.ResponseSource, row.AnsweredAt, row.UpdatedAt, row.HandledBySellerID = next, &source, &now, now, &sellerID
		if err := s.repo.Save(ctx, row); err != nil {
			return err
		}
		if next == "confirmed" {
			reason := "Demo simulated phone confirmation"
			return s.orders.CreateEvent(ctx, &entity.OrderEvent{EventID: newAppID(), OrderID: orderID, EventType: "confirmed", ActorType: "platform", Reason: &reason, OccurredAt: now, AvailableAt: &now, BatchID: entity.DemoBatchID, SourceRecordID: "verification:" + row.VerificationID})
		}
		return nil
	})
	return row, err
}
