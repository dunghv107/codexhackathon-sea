package serviceimpl

import (
	"testing"

	"github.com/tiloneeee/ai-fraud-detection/backend/internal/domain/entity"
	"github.com/tiloneeee/ai-fraud-detection/backend/internal/usecase/service"
	apperror "github.com/tiloneeee/ai-fraud-detection/backend/pkg/error"
)

func TestCheckFulfillment_TransitionTable(t *testing.T) {
	statuses := []string{
		entity.FulfillmentUnfulfilled, entity.FulfillmentPacking, entity.FulfillmentShipped,
		entity.FulfillmentDelivered, entity.FulfillmentCanceled, entity.FulfillmentReturned,
	}
	valid := map[[2]string]bool{
		{entity.FulfillmentUnfulfilled, entity.FulfillmentPacking}:  true,
		{entity.FulfillmentPacking, entity.FulfillmentShipped}:      true,
		{entity.FulfillmentShipped, entity.FulfillmentDelivered}:    true,
		{entity.FulfillmentUnfulfilled, entity.FulfillmentCanceled}: true,
		{entity.FulfillmentPacking, entity.FulfillmentCanceled}:     true,
		{entity.FulfillmentDelivered, entity.FulfillmentReturned}:   true,
	}
	for _, from := range statuses {
		for _, to := range statuses {
			order := &entity.Order{FulfillmentStatus: from, ReviewStatus: entity.ReviewReleased}
			err := checkFulfillment(order, to, true)
			if valid[[2]string{from, to}] {
				if err != nil {
					t.Errorf("%s -> %s: %v, want ok", from, to, err)
				}
			} else if !isCode(err, service.ErrInvalidTransition) {
				t.Errorf("%s -> %s: %v, want INVALID_TRANSITION", from, to, err)
			}
		}
	}
}

func TestCheckFulfillment_PackingGate(t *testing.T) {
	held := &entity.Order{FulfillmentStatus: entity.FulfillmentUnfulfilled, ReviewStatus: entity.ReviewHold}
	if err := checkFulfillment(held, entity.FulfillmentPacking, true); !isCode(err, service.ErrInvalidTransition) {
		t.Errorf("packing a held order: %v, want INVALID_TRANSITION", err)
	}
	released := &entity.Order{FulfillmentStatus: entity.FulfillmentUnfulfilled, ReviewStatus: entity.ReviewReleased}
	if err := checkFulfillment(released, entity.FulfillmentPacking, false); !isCode(err, service.ErrPaymentNotReady) {
		t.Errorf("packing without payment: %v, want PAYMENT_NOT_READY", err)
	}
	// Canceling a held order is allowed (no review gate).
	if err := checkFulfillment(held, entity.FulfillmentCanceled, false); err != nil {
		t.Errorf("cancel held order: %v", err)
	}
	if fulfillmentEvent(entity.FulfillmentDelivered) != entity.EventDeliverySucceeded ||
		fulfillmentEvent(entity.FulfillmentCanceled) != entity.EventCanceled ||
		fulfillmentEvent(entity.FulfillmentShipped) != "" {
		t.Error("fulfillmentEvent mapping is wrong")
	}
}

func TestCheckDecision(t *testing.T) {
	open := &entity.Order{FulfillmentStatus: entity.FulfillmentUnfulfilled, ReviewStatus: entity.ReviewPending}
	packed := &entity.Order{FulfillmentStatus: entity.FulfillmentPacking, ReviewStatus: entity.ReviewReleased}
	latest := &entity.RiskAssessment{AssessmentID: "a2"}

	cases := []struct {
		name         string
		order        *entity.Order
		latest       *entity.RiskAssessment
		assessmentID string
		action       string
		reason       string
		wantErr      *apperror.AppError
		wantStatus   string
	}{
		{"release", open, latest, "a2", entity.DecisionRelease, "ok", nil, entity.ReviewReleased},
		{"keep hold", open, latest, "a2", entity.DecisionKeepHold, "check phone", nil, entity.ReviewHold},
		{"blank reason", open, latest, "a2", entity.DecisionRelease, "  ", apperror.ErrValidation, ""},
		{"unknown action", open, latest, "a2", "hold", "x", apperror.ErrValidation, ""},
		{"not assessed", open, nil, "a2", entity.DecisionRelease, "x", service.ErrNotAssessed, ""},
		{"stale", open, latest, "a1", entity.DecisionRelease, "x", service.ErrStaleAssessment, ""},
		{"already packing", packed, latest, "a2", entity.DecisionKeepHold, "x", service.ErrInvalidState, ""},
	}
	for _, c := range cases {
		status, err := checkDecision(c.order, c.latest, c.assessmentID, c.action, c.reason)
		if c.wantErr != nil {
			if !isCode(err, c.wantErr) {
				t.Errorf("%s: %v, want %s", c.name, err, c.wantErr.Code)
			}
			continue
		}
		if err != nil || status != c.wantStatus {
			t.Errorf("%s: %q, %v; want %q", c.name, status, err, c.wantStatus)
		}
	}
}

func TestAddressKeyAndAuthSubject(t *testing.T) {
	if got := addressKey("  Rua  das Flores,\t10 \n SP "); got != "rua das flores, 10 sp" {
		t.Errorf("addressKey = %q", got)
	}
	if got := authSubject(42); got != "local:42" {
		t.Errorf("authSubject = %q", got)
	}
}
