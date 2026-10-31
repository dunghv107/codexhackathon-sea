package handler

import (
	"github.com/gin-gonic/gin"
	"strconv"

	"github.com/tiloneeee/ai-fraud-detection/backend/internal/domain/entity"
	"github.com/tiloneeee/ai-fraud-detection/backend/internal/interface/api/dto"
	"github.com/tiloneeee/ai-fraud-detection/backend/internal/interface/api/middleware"
	"github.com/tiloneeee/ai-fraud-detection/backend/internal/usecase/service"
	apperror "github.com/tiloneeee/ai-fraud-detection/backend/pkg/error"
	"github.com/tiloneeee/ai-fraud-detection/backend/pkg/response"
)

// SellerHandler interface; /shops/:shopId routes run after RequireOwnedShop
type SellerHandler interface {
	Network(c *gin.Context)
	GetVerification(c *gin.Context)
	RespondVerification(c *gin.Context)
	ListShops(c *gin.Context)
	CreateShop(c *gin.Context)
	GetDashboard(c *gin.Context)
	ListListings(c *gin.Context)
	CreateListing(c *gin.Context)
	UpdateListing(c *gin.Context)
	ListOrders(c *gin.Context)
	GetOrder(c *gin.Context)
	Score(c *gin.Context)
	Explain(c *gin.Context)
	Decide(c *gin.Context)
	UpdateFulfillment(c *gin.Context)
}

type sellerHandlerImpl struct {
	sellerService service.SellerService
}

// NewSellerHandler creates a new seller handler
func NewSellerHandler(sellerService service.SellerService) SellerHandler {
	return &sellerHandlerImpl{sellerService: sellerService}
}

func toShopResponse(s *entity.Shop) dto.ShopResponse {
	return dto.ShopResponse{ShopID: s.ShopID, DisplayName: s.DisplayName, BatchID: s.BatchID, AssessedOrders: s.AssessedOrders}
}

func (h *sellerHandlerImpl) ListShops(c *gin.Context) {
	shops, err := h.sellerService.ListShops(c.Request.Context(), middleware.GetSeller(c).SellerID)
	if err != nil {
		response.WriteErrorResponse(c, err)
		return
	}
	response.OK(c, mapSlice(shops, toShopResponse), "")
}

func (h *sellerHandlerImpl) CreateShop(c *gin.Context) {
	var req dto.CreateShopRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.WriteErrorResponse(c, apperror.ErrValidation.WithMessage("Dữ liệu không hợp lệ"))
		return
	}
	shop, err := h.sellerService.CreateShop(c.Request.Context(), middleware.GetSeller(c).SellerID, req.DisplayName)
	if err != nil {
		response.WriteErrorResponse(c, err)
		return
	}
	response.Created(c, toShopResponse(shop), "Tạo shop thành công")
}

func (h *sellerHandlerImpl) GetDashboard(c *gin.Context) {
	d, err := h.sellerService.GetDashboard(c.Request.Context(), middleware.GetShop(c).ShopID)
	if err != nil {
		response.WriteErrorResponse(c, err)
		return
	}
	response.OK(c, dto.DashboardResponse{ReviewStatus: d.ReviewStatus, FulfillmentStatus: d.FulfillmentStatus, RiskLevel: d.RiskLevel}, "")
}

func (h *sellerHandlerImpl) ListListings(c *gin.Context) {
	offset, limit := pagination(c)
	items, total, err := h.sellerService.ListListings(c.Request.Context(), middleware.GetShop(c).ShopID, offset, limit)
	if err != nil {
		response.WriteErrorResponse(c, err)
		return
	}
	response.OK(c, toListResponse(mapSlice(items, toListingCardResponse), total, offset, limit), "")
}

func (h *sellerHandlerImpl) CreateListing(c *gin.Context) {
	var req dto.CreateListingRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.WriteErrorResponse(c, apperror.ErrValidation.WithMessage("Dữ liệu không hợp lệ"))
		return
	}
	listing, err := h.sellerService.CreateListing(c.Request.Context(), service.CreateListingCommand{
		Description: req.Description, ImageURL: req.ImageURL,
		ShopID:        middleware.GetShop(c).ShopID,
		ProductName:   req.ProductName,
		CategoryCode:  req.CategoryCode,
		CurrentPrice:  req.CurrentPrice,
		Currency:      req.Currency,
		StockQuantity: req.StockQuantity,
	})
	if err != nil {
		response.WriteErrorResponse(c, err)
		return
	}
	response.Created(c, toListingCardResponse(listing), "Đã thêm sản phẩm")
}

func (h *sellerHandlerImpl) UpdateListing(c *gin.Context) {
	var req dto.UpdateListingRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.WriteErrorResponse(c, apperror.ErrValidation.WithMessage("Dữ liệu không hợp lệ"))
		return
	}
	listing, err := h.sellerService.UpdateListing(c.Request.Context(), service.UpdateListingCommand{
		ProductName: req.ProductName, Description: req.Description, ImageURL: req.ImageURL,
		ShopID:        middleware.GetShop(c).ShopID,
		ListingID:     c.Param("listingId"),
		CurrentPrice:  req.CurrentPrice,
		StockQuantity: req.StockQuantity,
	})
	if err != nil {
		response.WriteErrorResponse(c, err)
		return
	}
	response.OK(c, toListingCardResponse(listing), "Đã cập nhật sản phẩm")
}

func (h *sellerHandlerImpl) ListOrders(c *gin.Context) {
	offset, limit := pagination(c)
	orders, total, err := h.sellerService.ListOrders(c.Request.Context(), entity.OrderFilter{
		ShopID:            middleware.GetShop(c).ShopID,
		BatchID:           c.Query("batch_id"),
		OrderID:           c.Query("order_id"),
		ReviewStatus:      c.Query("review_status"),
		FulfillmentStatus: c.Query("fulfillment_status"),
		RiskLevel:         c.Query("risk_level"),
	}, offset, limit)
	if err != nil {
		response.WriteErrorResponse(c, err)
		return
	}
	response.OK(c, toListResponse(mapSlice(orders, toSellerOrderSummary), total, offset, limit), "")
}

func (h *sellerHandlerImpl) GetOrder(c *gin.Context) {
	detail, err := h.sellerService.GetOrder(c.Request.Context(), middleware.GetShop(c).ShopID, c.Param("orderId"))
	if err != nil {
		response.WriteErrorResponse(c, err)
		return
	}
	response.OK(c, toSellerOrderDetail(detail), "")
}

func (h *sellerHandlerImpl) Score(c *gin.Context) {
	force, err := strconv.ParseBool(c.DefaultQuery("force", "false"))
	if err != nil {
		response.WriteErrorResponse(c, apperror.ErrValidation.WithMessage("force phải là boolean"))
		return
	}
	detail, err := h.sellerService.Score(c.Request.Context(), middleware.GetShop(c).ShopID, c.Param("orderId"), force)
	if err != nil {
		response.WriteErrorResponse(c, err)
		return
	}
	response.OK(c, toSellerOrderDetail(detail), "")
}

func (h *sellerHandlerImpl) Explain(c *gin.Context) {
	explanation, err := h.sellerService.Explain(c.Request.Context(), middleware.GetShop(c).ShopID, c.Param("orderId"))
	if err != nil {
		response.WriteErrorResponse(c, err)
		return
	}
	response.OK(c, explanation, "")
}

func (h *sellerHandlerImpl) Decide(c *gin.Context) {
	var req dto.DecisionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.WriteErrorResponse(c, apperror.ErrValidation.WithMessage("Dữ liệu không hợp lệ"))
		return
	}
	detail, err := h.sellerService.Decide(c.Request.Context(), service.DecisionCommand{
		SellerID:     middleware.GetSeller(c).SellerID,
		ShopID:       middleware.GetShop(c).ShopID,
		OrderID:      c.Param("orderId"),
		AssessmentID: req.AssessmentID,
		Action:       req.Action,
		Reason:       req.Reason,
	})
	if err != nil {
		response.WriteErrorResponse(c, err)
		return
	}
	response.OK(c, toSellerOrderDetail(detail), "Đã ghi nhận quyết định")
}

func (h *sellerHandlerImpl) UpdateFulfillment(c *gin.Context) {
	var req dto.FulfillmentRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.WriteErrorResponse(c, apperror.ErrValidation.WithMessage("Dữ liệu không hợp lệ"))
		return
	}
	detail, err := h.sellerService.UpdateFulfillment(c.Request.Context(), service.FulfillmentCommand{
		ShopID:  middleware.GetShop(c).ShopID,
		OrderID: c.Param("orderId"),
		Status:  req.Status,
	})
	if err != nil {
		response.WriteErrorResponse(c, err)
		return
	}
	response.OK(c, toSellerOrderDetail(detail), "Đã cập nhật trạng thái giao hàng")
}

func (h *sellerHandlerImpl) Network(c *gin.Context) {
	result, err := h.sellerService.Network(c.Request.Context(), middleware.GetShop(c).ShopID, c.Param("orderId"))
	if err != nil {
		response.WriteErrorResponse(c, err)
		return
	}
	response.OK(c, result, "")
}
func (h *sellerHandlerImpl) GetVerification(c *gin.Context) {
	result, err := h.sellerService.GetVerification(c.Request.Context(), middleware.GetShop(c).ShopID, c.Param("orderId"))
	if err != nil {
		response.WriteErrorResponse(c, err)
		return
	}
	response.OK(c, result, "")
}
func (h *sellerHandlerImpl) RespondVerification(c *gin.Context) {
	var req struct {
		VerificationID string `json:"verification_id" binding:"required"`
		Key            string `json:"key" binding:"required,oneof=0 1 2"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.WriteErrorResponse(c, apperror.ErrValidation)
		return
	}
	result, err := h.sellerService.RespondVerification(c.Request.Context(), middleware.GetShop(c).ShopID, c.Param("orderId"), middleware.GetSeller(c).SellerID, req.VerificationID, req.Key)
	if err != nil {
		response.WriteErrorResponse(c, err)
		return
	}
	response.OK(c, result, "")
}
