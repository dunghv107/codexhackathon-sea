package handler

import (
	"github.com/gin-gonic/gin"

	"github.com/tiloneeee/ai-fraud-detection/backend/internal/domain/entity"
	"github.com/tiloneeee/ai-fraud-detection/backend/internal/interface/api/dto"
	"github.com/tiloneeee/ai-fraud-detection/backend/internal/usecase/service"
	"github.com/tiloneeee/ai-fraud-detection/backend/pkg/response"
)

// CatalogHandler interface (public, no login)
type CatalogHandler interface {
	ListCategories(c *gin.Context)
	ListShops(c *gin.Context)
	GetShop(c *gin.Context)
	ListListings(c *gin.Context)
	GetListing(c *gin.Context)
}

type catalogHandlerImpl struct {
	catalogService service.CatalogService
}

// NewCatalogHandler creates a new catalog handler
func NewCatalogHandler(catalogService service.CatalogService) CatalogHandler {
	return &catalogHandlerImpl{catalogService: catalogService}
}

func (h *catalogHandlerImpl) ListCategories(c *gin.Context) {
	cats, err := h.catalogService.ListCategories(c.Request.Context())
	if err != nil {
		response.WriteErrorResponse(c, err)
		return
	}
	response.OK(c, mapSlice(cats, func(cat *entity.Category) dto.CategoryResponse {
		return dto.CategoryResponse{CategoryCode: cat.CategoryCode, CategoryNameEn: cat.CategoryNameEn}
	}), "")
}

func toShopCardResponse(s *entity.ShopCard) dto.ShopCardResponse {
	return dto.ShopCardResponse{ShopID: s.ShopID, DisplayName: s.DisplayName, ListingCount: s.ListingCount}
}

func (h *catalogHandlerImpl) ListShops(c *gin.Context) {
	offset, limit := pagination(c)
	shops, total, err := h.catalogService.ListShops(c.Request.Context(), offset, limit)
	if err != nil {
		response.WriteErrorResponse(c, err)
		return
	}
	response.OK(c, toListResponse(mapSlice(shops, toShopCardResponse), total, offset, limit), "")
}

func (h *catalogHandlerImpl) GetShop(c *gin.Context) {
	shop, err := h.catalogService.GetShop(c.Request.Context(), c.Param("shopId"))
	if err != nil {
		response.WriteErrorResponse(c, err)
		return
	}
	response.OK(c, toShopCardResponse(shop), "")
}

func (h *catalogHandlerImpl) ListListings(c *gin.Context) {
	offset, limit := pagination(c)
	items, total, err := h.catalogService.ListListings(c.Request.Context(), entity.ListingFilter{ShopID: c.Query("shop_id"), CategoryCode: c.Query("category_code"), Query: c.Query("q"), Sort: c.Query("sort"), Currency: c.Query("currency"), InStock: c.Query("in_stock") == "true"}, offset, limit)
	if err != nil {
		response.WriteErrorResponse(c, err)
		return
	}
	response.OK(c, toListResponse(mapSlice(items, toListingCardResponse), total, offset, limit), "")
}

func (h *catalogHandlerImpl) GetListing(c *gin.Context) {
	item, err := h.catalogService.GetListing(c.Request.Context(), c.Param("listingId"))
	if err != nil {
		response.WriteErrorResponse(c, err)
		return
	}
	response.OK(c, toListingCardResponse(item), "")
}
