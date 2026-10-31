package handler

import (
	"github.com/gin-gonic/gin"

	"github.com/tiloneeee/ai-fraud-detection/backend/internal/domain/entity"
	"github.com/tiloneeee/ai-fraud-detection/backend/internal/interface/api/dto"
	"github.com/tiloneeee/ai-fraud-detection/backend/pkg/query"
)

// pagination reads page (from 1) and limit (default 20, max 100).
func pagination(c *gin.Context) (offset, limit int) {
	return query.GetPagination(map[string]string{"page": c.DefaultQuery("page", "1"), "limit": c.Query("limit")}, 20)
}

func toListResponse[T any](items []T, total int64, offset, limit int) dto.ListResponse[T] {
	return dto.ListResponse[T]{
		Items:      items,
		Total:      total,
		Page:       offset/limit + 1,
		Limit:      limit,
		TotalPages: int((total + int64(limit) - 1) / int64(limit)),
	}
}

func mapSlice[S, D any](src []S, fn func(*S) D) []D {
	out := make([]D, len(src))
	for i := range src {
		out[i] = fn(&src[i])
	}
	return out
}

func toListingCardResponse(l *entity.ListingCard) dto.ListingCardResponse {
	return dto.ListingCardResponse{
		Description: l.Description, ImageURL: l.ImageURL,
		ListingID:      l.ListingID,
		ShopID:         l.ShopID,
		ShopName:       l.ShopName,
		ProductID:      l.ProductID,
		ProductName:    l.ProductName,
		CategoryCode:   l.CategoryCode,
		CategoryNameEn: l.CategoryNameEn,
		CurrentPrice:   l.CurrentPrice,
		Currency:       l.Currency,
		StockQuantity:  l.StockQuantity,
	}
}

func toOrderItemResponse(i *entity.OrderItemView) dto.OrderItemResponse {
	return dto.OrderItemResponse{
		ItemID:         i.ItemID,
		ListingID:      i.ListingID,
		ProductName:    i.ProductName,
		CategoryNameEn: i.CategoryNameEn,
		Quantity:       i.Quantity,
		UnitPrice:      i.UnitPrice,
		FreightAmount:  i.FreightAmount,
	}
}

func toOrderAddressResponse(a *entity.CheckoutAddress) dto.OrderAddressResponse {
	return dto.OrderAddressResponse{
		WardCode: a.WardCode, HouseNo: a.HouseNo, ParseOK: a.ParseOK,
		AddressRaw:   a.AddressRaw,
		City:         a.City,
		Region:       a.Region,
		PostalPrefix: a.PostalPrefix,
		CountryCode:  a.CountryCode,
	}
}

func toBuyerOrderSummary(o *entity.OrderSummary) dto.BuyerOrderSummary {
	return dto.BuyerOrderSummary{
		OrderID:           o.OrderID,
		CheckoutID:        o.CheckoutID,
		PlacedAt:          o.PlacedAt,
		ShopID:            o.ShopID,
		ShopName:          o.ShopName,
		ItemCount:         o.ItemCount,
		OrderTotal:        o.OrderTotal,
		Currency:          o.Currency,
		FulfillmentStatus: o.FulfillmentStatus,
		DisplayStatus:     o.DisplayStatus(),
	}
}

func toBuyerOrderDetail(d *entity.OrderDetail) dto.BuyerOrderDetail {
	return dto.BuyerOrderDetail{
		BuyerOrderSummary: toBuyerOrderSummary(&d.OrderSummary),
		Items:             mapSlice(d.Items, toOrderItemResponse),
		Address:           toOrderAddressResponse(&d.Address),
		Payment:           dto.OrderPaymentResponse{Method: d.PaymentMethod, Status: d.PaymentStatus},
		Events: mapSlice(d.Events, func(e *entity.OrderEvent) dto.BuyerOrderEvent {
			return dto.BuyerOrderEvent{EventType: e.EventType, OccurredAt: e.OccurredAt}
		}),
	}
}

func toSellerOrderSummary(o *entity.OrderSummary) dto.SellerOrderSummary {
	return dto.SellerOrderSummary{
		BatchID: o.BatchID, IsSynthetic: o.IsSynthetic, SourceSystem: o.SourceSystem,
		OrderID:           o.OrderID,
		CheckoutID:        o.CheckoutID,
		PlacedAt:          o.PlacedAt,
		PaymentMethod:     o.PaymentMethod,
		ItemCount:         o.ItemCount,
		OrderTotal:        o.OrderTotal,
		Currency:          o.Currency,
		ReviewStatus:      o.ReviewStatus,
		FulfillmentStatus: o.FulfillmentStatus,
		RiskLevel:         o.RiskLevel,
		Score:             o.Score,
	}
}

func toSellerOrderDetail(d *entity.OrderDetail) dto.SellerOrderDetail {
	resp := dto.SellerOrderDetail{
		PhoneMasked: d.PhoneMasked, DemoMode: d.DemoMode, Verification: d.Verification,
		Cases:              d.Cases,
		SellerOrderSummary: toSellerOrderSummary(&d.OrderSummary),
		Items:              mapSlice(d.Items, toOrderItemResponse),
		Address:            toOrderAddressResponse(&d.Address),
		Payment:            dto.OrderPaymentResponse{Method: d.PaymentMethod, Status: d.PaymentStatus},
		Events: mapSlice(d.Events, func(e *entity.OrderEvent) dto.SellerOrderEvent {
			return dto.SellerOrderEvent{AvailableAt: e.AvailableAt, EventType: e.EventType, ActorType: e.ActorType, Reason: e.Reason, OccurredAt: e.OccurredAt}
		}),
		Decisions: mapSlice(d.Decisions, func(r *entity.ReviewDecision) dto.DecisionResponse {
			return dto.DecisionResponse{
				DecisionID:   r.DecisionID,
				AssessmentID: r.AssessmentID,
				ActorType:    r.ActorType,
				Action:       r.Action,
				Reason:       r.Reason,
				CreatedAt:    r.CreatedAt,
			}
		}),
	}
	if a := d.LatestAssessment; a != nil {
		resp.LatestAssessment = &dto.AssessmentResponse{
			RuleConfig:      a.RuleConfig,
			AssessmentID:    a.AssessmentID,
			AssessedAt:      a.AssessedAt,
			InputCutoffAt:   a.InputCutoffAt,
			RuleVersion:     a.RuleVersion,
			Score:           a.Score,
			RiskLevel:       a.RiskLevel,
			FeatureSnapshot: a.FeatureSnapshot,
			Reasons:         a.Reasons,
		}
	}
	return resp
}
