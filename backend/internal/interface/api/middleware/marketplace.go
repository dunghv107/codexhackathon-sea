package middleware

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/tiloneeee/ai-fraud-detection/backend/internal/domain/entity"
	"github.com/tiloneeee/ai-fraud-detection/backend/internal/usecase/service"
	"github.com/tiloneeee/ai-fraud-detection/backend/pkg/response"
)

const (
	BuyerContextKey  ContextKey = "buyer"
	SellerContextKey ContextKey = "seller"
	ShopContextKey   ContextKey = "shop"
)

// isWrite reports whether the request changes data (blocked for suspended accounts).
func isWrite(c *gin.Context) bool {
	switch c.Request.Method {
	case http.MethodGet, http.MethodHead, http.MethodOptions:
		return false
	}
	return true
}

// RequireBuyer resolves the buyer linked to the JWT user (403 NO_BUYER_PROFILE); must run after Auth.
func (m *Middleware) RequireBuyer() gin.HandlerFunc {
	return func(c *gin.Context) {
		buyer, err := m.profileService.ResolveBuyer(c.Request.Context(), GetUserID(c))
		if err != nil {
			response.WriteErrorResponse(c, err)
			c.Abort()
			return
		}
		if isWrite(c) && buyer.Status != entity.AccountStatusActive {
			response.WriteErrorResponse(c, service.ErrAccountSuspended)
			c.Abort()
			return
		}
		c.Set(string(BuyerContextKey), buyer)
		c.Next()
	}
}

// RequireSeller resolves the seller linked to the JWT user (403 NO_SELLER_PROFILE); must run after Auth.
func (m *Middleware) RequireSeller() gin.HandlerFunc {
	return func(c *gin.Context) {
		seller, err := m.profileService.ResolveSeller(c.Request.Context(), GetUserID(c))
		if err != nil {
			response.WriteErrorResponse(c, err)
			c.Abort()
			return
		}
		if isWrite(c) && seller.Status != entity.AccountStatusActive {
			response.WriteErrorResponse(c, service.ErrAccountSuspended)
			c.Abort()
			return
		}
		c.Set(string(SellerContextKey), seller)
		c.Next()
	}
}

// RequireOwnedShop loads :shopId only if the current seller owns it, else 404; must run after RequireSeller.
func (m *Middleware) RequireOwnedShop() gin.HandlerFunc {
	return func(c *gin.Context) {
		shop, err := m.sellerService.GetOwnedShop(c.Request.Context(), GetSeller(c).SellerID, c.Param("shopId"))
		if err != nil {
			response.WriteErrorResponse(c, err)
			c.Abort()
			return
		}
		c.Set(string(ShopContextKey), shop)
		c.Next()
	}
}

// GetBuyer retrieves the buyer set by RequireBuyer
func GetBuyer(c *gin.Context) *entity.Buyer {
	v, _ := c.Get(string(BuyerContextKey))
	buyer, _ := v.(*entity.Buyer)
	return buyer
}

// GetSeller retrieves the seller set by RequireSeller
func GetSeller(c *gin.Context) *entity.Seller {
	v, _ := c.Get(string(SellerContextKey))
	seller, _ := v.(*entity.Seller)
	return seller
}

// GetShop retrieves the owned shop set by RequireOwnedShop
func GetShop(c *gin.Context) *entity.Shop {
	v, _ := c.Get(string(ShopContextKey))
	shop, _ := v.(*entity.Shop)
	return shop
}
