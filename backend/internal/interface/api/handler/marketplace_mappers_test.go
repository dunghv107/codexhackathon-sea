package handler

import (
	"encoding/json"
	"github.com/tiloneeee/ai-fraud-detection/backend/internal/domain/entity"
	"strings"
	"testing"
)

func TestPhoneAndRiskRemainSellerOnly(t *testing.T) {
	detail := &entity.OrderDetail{OrderSummary: entity.OrderSummary{ReviewStatus: "pending", FulfillmentStatus: "unfulfilled"}, PhoneMasked: "***0001", DemoMode: true, Verification: &entity.OrderVerification{VerificationID: "verification", Status: "confirmed", HandledBySellerID: new(string)}, LatestAssessment: &entity.RiskAssessment{Score: 30, RiskLevel: "medium"}}
	buyer, err := json.Marshal(toBuyerOrderDetail(detail))
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{"phone", "risk_level", "review_status", "assessment", "verification"} {
		if strings.Contains(string(buyer), forbidden) {
			t.Fatalf("buyer leaked %s", forbidden)
		}
	}
	if !strings.Contains(string(buyer), `"display_status":"confirming"`) {
		t.Fatal("verification changed buyer status")
	}
	seller, err := json.Marshal(toSellerOrderDetail(detail))
	if err != nil || !strings.Contains(string(seller), `"phone_masked":"***0001"`) || strings.Contains(string(seller), "handled_by_seller_id") {
		t.Fatal("seller DTO leakage")
	}
	contact, _ := json.Marshal(entity.CheckoutContact{PhoneE164: "+84900000001", PhoneKey: "private"})
	if string(contact) != "{}" {
		t.Fatal("contact serialized")
	}
}
