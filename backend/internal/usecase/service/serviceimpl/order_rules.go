package serviceimpl

import (
	"context"
	"strings"

	"github.com/google/uuid"

	"github.com/tiloneeee/ai-fraud-detection/backend/internal/domain/entity"
	"github.com/tiloneeee/ai-fraud-detection/backend/internal/domain/repository"
	"github.com/tiloneeee/ai-fraud-detection/backend/internal/usecase/service"
	apperror "github.com/tiloneeee/ai-fraud-detection/backend/pkg/error"
)

// newAppID returns an ID for a row created through the API (batch "app").
func newAppID() string {
	return "app:" + uuid.NewString()
}

// fulfillmentTransitions is the seller state machine; the DB CHECK does not enforce it.
var fulfillmentTransitions = map[string][]string{
	entity.FulfillmentUnfulfilled: {entity.FulfillmentPacking, entity.FulfillmentCanceled},
	entity.FulfillmentPacking:     {entity.FulfillmentShipped, entity.FulfillmentCanceled},
	entity.FulfillmentShipped:     {entity.FulfillmentDelivered},
	entity.FulfillmentDelivered:   {entity.FulfillmentReturned},
}

// checkFulfillment validates moving order to status `to`; paymentReady only matters for packing.
func checkFulfillment(order *entity.Order, to string, paymentReady bool) error {
	allowed := false
	for _, s := range fulfillmentTransitions[order.FulfillmentStatus] {
		allowed = allowed || s == to
	}
	if !allowed {
		return service.ErrInvalidTransition
	}
	if to == entity.FulfillmentPacking {
		if order.ReviewStatus != entity.ReviewReleased {
			return service.ErrInvalidTransition.WithMessage("Đơn chưa được release, không thể đóng gói")
		}
		if !paymentReady {
			return service.ErrPaymentNotReady
		}
	}
	return nil
}

// fulfillmentEvent maps a new status to its order_events type; "" when the schema has none.
func fulfillmentEvent(to string) string {
	switch to {
	case entity.FulfillmentDelivered:
		return entity.EventDeliverySucceeded
	case entity.FulfillmentCanceled:
		return entity.EventCanceled
	}
	return ""
}

// checkDecision validates a seller decision against the locked order and its latest assessment,
// returning the new review_status.
func checkDecision(order *entity.Order, latest *entity.RiskAssessment, assessmentID, action, reason string) (string, error) {
	var next string
	switch action {
	case entity.DecisionRelease:
		next = entity.ReviewReleased
	case entity.DecisionHold, entity.DecisionKeepHold:
		next = entity.ReviewHold
	default:
		return "", apperror.ErrValidation.WithMessage("Hành động không hợp lệ")
	}
	if strings.TrimSpace(reason) == "" {
		return "", apperror.ErrValidation.WithMessage("Lý do không được để trống")
	}
	if latest == nil {
		return "", service.ErrNotAssessed
	}
	if latest.AssessmentID != assessmentID {
		return "", service.ErrStaleAssessment
	}
	if order.FulfillmentStatus != entity.FulfillmentUnfulfilled {
		return "", service.ErrInvalidState
	}
	return next, nil
}

// loadOrderDetail reads one order inside the buyer/shop scope of f; review data is loaded only for sellers.
func loadOrderDetail(ctx context.Context, orderRepo repository.OrderRepository, f entity.OrderFilter, forSeller bool) (*entity.OrderDetail, error) {
	orders, _, err := orderRepo.ListOrders(ctx, f, 0, 1)
	if err != nil {
		return nil, err
	}
	if len(orders) == 0 {
		return nil, apperror.ErrNotFound.WithMessage("Không tìm thấy đơn hàng")
	}
	detail := &entity.OrderDetail{OrderSummary: orders[0]}

	if detail.Items, err = orderRepo.ListItems(ctx, f.OrderID); err != nil {
		return nil, err
	}
	addr, err := orderRepo.FindCheckoutAddress(ctx, detail.CheckoutID)
	if err != nil {
		return nil, err
	}
	detail.Address = *addr
	if detail.Events, err = orderRepo.ListEvents(ctx, f.OrderID); err != nil {
		return nil, err
	}
	if !forSeller {
		return detail, nil
	}
	if detail.LatestAssessment, err = orderRepo.FindLatestAssessment(ctx, f.OrderID); err != nil {
		return nil, err
	}
	if detail.Cases, err = orderRepo.ListCases(ctx, f.OrderID); err != nil {
		return nil, err
	}
	if detail.Decisions, err = orderRepo.ListDecisions(ctx, f.OrderID); err != nil {
		return nil, err
	}
	return detail, nil
}
