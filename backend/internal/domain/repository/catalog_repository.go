package repository

import (
	"context"

	"github.com/tiloneeee/ai-fraud-detection/backend/internal/domain/entity"
)

// CatalogRepository covers shops, products and listings
type CatalogRepository interface {
	ListCategories(ctx context.Context) ([]entity.Category, error)
	FindCategoryName(ctx context.Context, code string) (*string, error)
	ListShops(ctx context.Context, offset, limit int) ([]entity.ShopCard, int64, error)
	FindShopCard(ctx context.Context, shopID string) (*entity.ShopCard, error)
	ListListings(ctx context.Context, f entity.ListingFilter, offset, limit int) ([]entity.ListingCard, int64, error)

	ListShopsBySeller(ctx context.Context, sellerID string) ([]entity.Shop, error)
	// FindOwnedShop returns NOT_FOUND when the shop does not belong to sellerID.
	FindOwnedShop(ctx context.Context, shopID, sellerID string) (*entity.Shop, error)
	CreateShop(ctx context.Context, shop *entity.Shop) error
	CreateProduct(ctx context.Context, product *entity.Product) error
	CreateListing(ctx context.Context, listing *entity.Listing) error
	// LockListing / LockListings take FOR UPDATE row locks; call inside a transaction.
	LockListing(ctx context.Context, shopID, listingID string) (*entity.Listing, error)
	LockListings(ctx context.Context, listingIDs []string) ([]entity.ListingCard, error)
	UpdateListing(ctx context.Context, listing *entity.Listing, columns ...string) error
}
