package serviceimpl

import (
	"context"
	"github.com/tiloneeee/ai-fraud-detection/backend/internal/domain/entity"
	"github.com/tiloneeee/ai-fraud-detection/backend/internal/domain/repository"
	"strings"
	"testing"
)

type phoneRepo struct {
	repository.VerificationRepository
	calls int
}

func (r *phoneRepo) DemoPhoneKey(_ context.Context, phone string) (string, error) {
	r.calls++
	if phone == "+84900000001" {
		return "sim:phone", nil
	}
	return "", nil
}
func TestPhoneIdentity(t *testing.T) {
	repo := &phoneRepo{}
	if _, err := NewPhoneIdentityService(repo, "short", false); err == nil {
		t.Fatal("weak secret accepted")
	}
	svc, _ := NewPhoneIdentityService(repo, strings.Repeat("x", 32), false)
	var key string
	for _, raw := range []string{"0900000001", "+84 900-000-001", "84900000001", " (090) 000 0001 "} {
		contact, err := svc.Resolve(context.Background(), raw)
		if err != nil || contact.PhoneE164 != "+84900000001" || contact.KeySource != "hmac" || !strings.HasPrefix(contact.PhoneKey, "hmac:v1:") {
			t.Fatalf("normalization failed: %v", err)
		}
		if key != "" && key != contact.PhoneKey {
			t.Fatal("unstable HMAC")
		}
		key = contact.PhoneKey
	}
	if repo.calls != 0 {
		t.Fatal("demo mapping used while disabled")
	}
	for _, raw := range []string{"", "090", "+12025550123", "0900000001x", "++84900000001", strings.Repeat("0", 41)} {
		if _, err := svc.Resolve(context.Background(), raw); err == nil {
			t.Fatal("invalid phone accepted")
		}
	}
	demo, _ := NewPhoneIdentityService(repo, strings.Repeat("x", 32), true)
	mapped, err := demo.Resolve(context.Background(), "0900000001")
	if err != nil || mapped.PhoneKey != "sim:phone" || mapped.KeySource != "demo_mapping" {
		t.Fatal("demo mapping not used")
	}
	unknown, err := demo.Resolve(context.Background(), "0900000002")
	if err != nil || unknown.KeySource != "hmac" || unknown.PhoneKey == key {
		t.Fatal("unknown demo phone must use HMAC")
	}
	rotated, _ := NewPhoneIdentityService(repo, strings.Repeat("y", 32), false)
	changed, _ := rotated.Resolve(context.Background(), "0900000001")
	if changed.PhoneKey == key {
		t.Fatal("HMAC secret ignored")
	}
}

func TestVerificationEligibility(t *testing.T) {
	for _, level := range []string{"low", "medium", "high"} {
		for _, cod := range []bool{true, false} {
			for _, review := range []string{"pending", "hold", "released"} {
				for _, fulfillment := range []string{"unfulfilled", "packing", "canceled"} {
					order := &entity.Order{BatchID: entity.DemoBatchID, ReviewStatus: review, FulfillmentStatus: fulfillment}
					want := level == "medium" && cod && review != "released" && fulfillment == "unfulfilled"
					if verificationEligible(order, &entity.RiskAssessment{RiskLevel: level}, cod) != want {
						t.Fatal("eligibility mismatch")
					}
					if verificationEligible(order, nil, cod) {
						t.Fatal("unassessed eligible")
					}
					order.BatchID = "olist_raw"
					if verificationEligible(order, &entity.RiskAssessment{RiskLevel: level}, cod) {
						t.Fatal("raw batch eligible")
					}
				}
			}
		}
	}
}
