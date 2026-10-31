package entity

import "testing"

func TestOrderSummary_DisplayStatus(t *testing.T) {
	cases := []struct{ review, fulfillment, want string }{
		{ReviewPending, FulfillmentUnfulfilled, "confirming"},
		{ReviewHold, FulfillmentUnfulfilled, "confirming"},
		{ReviewReleased, FulfillmentUnfulfilled, "confirmed"},
		{ReviewReleased, FulfillmentPacking, "packing"},
		{ReviewReleased, FulfillmentShipped, "shipped"},
		{ReviewReleased, FulfillmentDelivered, "delivered"},
		{ReviewHold, FulfillmentCanceled, "canceled"},
		{ReviewReleased, FulfillmentReturned, "returned"},
	}
	for _, c := range cases {
		got := OrderSummary{ReviewStatus: c.review, FulfillmentStatus: c.fulfillment}.DisplayStatus()
		if got != c.want {
			t.Errorf("(%s, %s) = %q, want %q", c.review, c.fulfillment, got, c.want)
		}
	}
}
