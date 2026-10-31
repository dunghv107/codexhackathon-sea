package router

import (
	"github.com/gin-gonic/gin"
	"github.com/thienel/tlog"

	"github.com/tiloneeee/ai-fraud-detection/backend/internal/interface/api/handler"
	"github.com/tiloneeee/ai-fraud-detection/backend/internal/interface/api/middleware"
)

type routeRegister struct {
	auth    handler.AuthHandler
	user    handler.UserHandler
	profile handler.ProfileHandler
	catalog handler.CatalogHandler
	buyer   handler.BuyerHandler
	seller  handler.SellerHandler
	mw      *middleware.Middleware
}

// SetupRouter configures all routes following THD-Checkin-App pattern
func SetupRouter(
	authHandler handler.AuthHandler,
	userHandler handler.UserHandler,
	profileHandler handler.ProfileHandler,
	catalogHandler handler.CatalogHandler,
	buyerHandler handler.BuyerHandler,
	sellerHandler handler.SellerHandler,
	mw *middleware.Middleware,
	rateLimitPerMin int,
) *gin.Engine {

	routes := routeRegister{
		auth:    authHandler,
		user:    userHandler,
		profile: profileHandler,
		catalog: catalogHandler,
		buyer:   buyerHandler,
		seller:  sellerHandler,
		mw:      mw,
	}

	router := gin.New()
	// Trust no proxy headers: ClientIP() is the TCP peer, so X-Forwarded-For cannot dodge the rate limit.
	// shortcut: no reverse proxy in front today, pass its CIDRs here once one is added.
	_ = router.SetTrustedProxies(nil) // nil cannot fail
	router.Use(gin.Recovery(), mw.CORS(), tlog.GinMiddleware(tlog.WithSkipPaths("/health")))

	// Health check
	router.GET("/health", func(c *gin.Context) {
		c.JSON(200, gin.H{"status": "ok"})
	})

	// Public API
	api := router.Group("/api", mw.RateLimit("api", rateLimitPerMin))
	{
		routes.registerAuthRoutes(api)
		routes.registerCatalogRoutes(api)
	}

	// Protected API
	protected := api.Group("", mw.Auth())
	{
		routes.registerUserRoutes(protected)
		protected.GET("/profiles", routes.profile.GetProfiles)
		routes.registerBuyerRoutes(protected)
		routes.registerSellerRoutes(protected)
	}

	return router
}

func (r *routeRegister) registerAuthRoutes(rg *gin.RouterGroup) {
	auth := rg.Group("/auth")
	{
		auth.POST("/login", r.auth.Login)
		auth.POST("/refresh", r.auth.Refresh)
	}

	// Protected auth routes
	authProtected := auth.Group("", r.mw.Auth())
	{
		authProtected.GET("/me", r.auth.GetMe)
		authProtected.POST("/logout", r.auth.Logout)
	}
}

func (r *routeRegister) registerUserRoutes(rg *gin.RouterGroup) {
	users := rg.Group("/users", r.mw.RequireAdmin())
	{
		users.GET("", r.user.List)
		users.GET("/:id", r.user.GetByID)
		users.POST("", r.user.Create)
		users.PUT("/:id", r.user.Update)
		users.DELETE("/:id", r.user.Delete)
	}
}

func (r *routeRegister) registerCatalogRoutes(rg *gin.RouterGroup) {
	catalog := rg.Group("/catalog")
	{
		catalog.GET("/categories", r.catalog.ListCategories)
		catalog.GET("/shops", r.catalog.ListShops)
		catalog.GET("/shops/:shopId", r.catalog.GetShop)
		catalog.GET("/listings", r.catalog.ListListings)
	}
}

func (r *routeRegister) registerBuyerRoutes(rg *gin.RouterGroup) {
	buyer := rg.Group("/buyer", r.mw.RequireBuyer())
	{
		buyer.GET("/addresses", r.buyer.ListAddresses)
		buyer.POST("/addresses", r.buyer.CreateAddress)
		buyer.PUT("/addresses/:id", r.buyer.UpdateAddress)
		buyer.DELETE("/addresses/:id", r.buyer.DeleteAddress)
		buyer.POST("/checkouts", r.buyer.Checkout)
		buyer.GET("/orders", r.buyer.ListOrders)
		buyer.GET("/orders/:orderId", r.buyer.GetOrder)
		buyer.POST("/orders/:orderId/cancel", r.buyer.CancelOrder)
	}
}

func (r *routeRegister) registerSellerRoutes(rg *gin.RouterGroup) {
	seller := rg.Group("/seller", r.mw.RequireSeller())
	{
		seller.GET("/shops", r.seller.ListShops)
		seller.POST("/shops", r.seller.CreateShop)
	}

	// Every shop route checks shops.owner_seller_id first; foreign shops are 404.
	shop := seller.Group("/shops/:shopId", r.mw.RequireOwnedShop())
	{
		shop.GET("/dashboard", r.seller.GetDashboard)
		shop.GET("/listings", r.seller.ListListings)
		shop.POST("/listings", r.seller.CreateListing)
		shop.PATCH("/listings/:listingId", r.seller.UpdateListing)
		shop.GET("/orders", r.seller.ListOrders)
		shop.GET("/orders/:orderId", r.seller.GetOrder)
		shop.POST("/orders/:orderId/decision", r.seller.Decide)
		shop.POST("/orders/:orderId/fulfillment", r.seller.UpdateFulfillment)
	}
}
