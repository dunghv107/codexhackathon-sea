package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/tiloneeee/ai-fraud-detection/backend/internal/domain/entity"
	"github.com/tiloneeee/ai-fraud-detection/backend/internal/interface/api/dto"
	"github.com/tiloneeee/ai-fraud-detection/backend/internal/interface/api/middleware"
	"github.com/tiloneeee/ai-fraud-detection/backend/internal/usecase/service"
	apperror "github.com/tiloneeee/ai-fraud-detection/backend/pkg/error"
	"github.com/tiloneeee/ai-fraud-detection/backend/pkg/response"
)

// BuyerHandler interface; every route runs after RequireBuyer
type BuyerHandler interface {
	ListAddresses(c *gin.Context)
	CreateAddress(c *gin.Context)
	UpdateAddress(c *gin.Context)
	DeleteAddress(c *gin.Context)
	Checkout(c *gin.Context)
	ListOrders(c *gin.Context)
	GetOrder(c *gin.Context)
	CancelOrder(c *gin.Context)
}

type buyerHandlerImpl struct {
	buyerService service.BuyerService
}

// NewBuyerHandler creates a new buyer handler
func NewBuyerHandler(buyerService service.BuyerService) BuyerHandler {
	return &buyerHandlerImpl{buyerService: buyerService}
}

func toSavedAddressResponse(a *entity.SavedAddress) dto.SavedAddressResponse {
	return dto.SavedAddressResponse{
		SavedAddressID: a.SavedAddressID,
		Label:          a.Label,
		AddressRaw:     a.AddressRaw,
		Region:         a.Region,
		CountryCode:    a.CountryCode,
		IsDefault:      a.IsDefault,
		CreatedAt:      a.CreatedAt,
	}
}

func (h *buyerHandlerImpl) ListAddresses(c *gin.Context) {
	addrs, err := h.buyerService.ListAddresses(c.Request.Context(), middleware.GetBuyer(c).BuyerID)
	if err != nil {
		response.WriteErrorResponse(c, err)
		return
	}
	response.OK(c, mapSlice(addrs, toSavedAddressResponse), "")
}

func (h *buyerHandlerImpl) saveAddress(c *gin.Context, id string) (*entity.SavedAddress, bool) {
	var req dto.SavedAddressRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.WriteErrorResponse(c, apperror.ErrValidation.WithMessage("Dữ liệu không hợp lệ"))
		return nil, false
	}
	addr, err := h.buyerService.SaveAddress(c.Request.Context(), service.SaveAddressCommand{
		BuyerID:     middleware.GetBuyer(c).BuyerID,
		ID:          id,
		Label:       req.Label,
		AddressRaw:  req.AddressRaw,
		Region:      req.Region,
		CountryCode: req.CountryCode,
		IsDefault:   req.IsDefault,
	})
	if err != nil {
		response.WriteErrorResponse(c, err)
		return nil, false
	}
	return addr, true
}

func (h *buyerHandlerImpl) CreateAddress(c *gin.Context) {
	if addr, ok := h.saveAddress(c, ""); ok {
		response.Created(c, toSavedAddressResponse(addr), "Đã thêm địa chỉ")
	}
}

func (h *buyerHandlerImpl) UpdateAddress(c *gin.Context) {
	if addr, ok := h.saveAddress(c, c.Param("id")); ok {
		response.OK(c, toSavedAddressResponse(addr), "Đã cập nhật địa chỉ")
	}
}

func (h *buyerHandlerImpl) DeleteAddress(c *gin.Context) {
	if err := h.buyerService.DeleteAddress(c.Request.Context(), middleware.GetBuyer(c).BuyerID, c.Param("id")); err != nil {
		response.WriteErrorResponse(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

func (h *buyerHandlerImpl) Checkout(c *gin.Context) {
	var req dto.CheckoutRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.WriteErrorResponse(c, apperror.ErrValidation.WithMessage("Dữ liệu không hợp lệ"))
		return
	}
	items := make([]service.CheckoutItem, len(req.Items))
	for i, it := range req.Items {
		items[i] = service.CheckoutItem{ListingID: it.ListingID, Quantity: it.Quantity}
	}

	result, err := h.buyerService.Checkout(c.Request.Context(), service.CheckoutCommand{
		Phone:          req.Phone,
		BuyerID:        middleware.GetBuyer(c).BuyerID,
		SavedAddressID: req.SavedAddressID,
		PaymentMethod:  req.PaymentMethod,
		Items:          items,
	})
	if err != nil {
		response.WriteErrorResponse(c, err)
		return
	}

	orders := make([]dto.CheckoutOrderResponse, len(result.Orders))
	for i, o := range result.Orders {
		orders[i] = dto.CheckoutOrderResponse{OrderID: o.OrderID, ShopID: o.ShopID, ShopName: o.ShopName, OrderTotal: o.OrderTotal}
	}
	response.Created(c, dto.CheckoutResponse{
		CheckoutID:  result.Checkout.CheckoutID,
		PlacedAt:    result.Checkout.PlacedAt,
		TotalAmount: result.Checkout.TotalAmount,
		Currency:    result.Checkout.Currency,
		Orders:      orders,
	}, "Đặt hàng thành công")
}

func (h *buyerHandlerImpl) ListOrders(c *gin.Context) {
	offset, limit := pagination(c)
	orders, total, err := h.buyerService.ListOrders(c.Request.Context(), middleware.GetBuyer(c).BuyerID, offset, limit)
	if err != nil {
		response.WriteErrorResponse(c, err)
		return
	}
	response.OK(c, toListResponse(mapSlice(orders, toBuyerOrderSummary), total, offset, limit), "")
}

func (h *buyerHandlerImpl) GetOrder(c *gin.Context) {
	detail, err := h.buyerService.GetOrder(c.Request.Context(), middleware.GetBuyer(c).BuyerID, c.Param("orderId"))
	if err != nil {
		response.WriteErrorResponse(c, err)
		return
	}
	response.OK(c, toBuyerOrderDetail(detail), "")
}

func (h *buyerHandlerImpl) CancelOrder(c *gin.Context) {
	var req dto.ReasonRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.WriteErrorResponse(c, apperror.ErrValidation.WithMessage("Dữ liệu không hợp lệ"))
		return
	}
	detail, err := h.buyerService.CancelOrder(c.Request.Context(), middleware.GetBuyer(c).BuyerID, c.Param("orderId"), req.Reason)
	if err != nil {
		response.WriteErrorResponse(c, err)
		return
	}
	response.OK(c, toBuyerOrderDetail(detail), "Đã hủy đơn hàng")
}
