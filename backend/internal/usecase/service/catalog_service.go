package service

import (
	"context"

	"github.com/tiloneeee/ai-fraud-detection/backend/internal/domain/entity"
)

// CatalogService defines the public catalog
type CatalogService interface {
	ListCategories(ctx context.Context) ([]entity.Category, error)
	ListShops(ctx context.Context, offset, limit int) ([]entity.ShopCard, int64, error)
	GetShop(ctx context.Context, shopID string) (*entity.ShopCard, error)
	// ListListings returns only listings that have a current price.
	GetListing(ctx context.Context, listingID string) (*entity.ListingCard, error)
	ListListings(ctx context.Context, filter entity.ListingFilter, offset, limit int) ([]entity.ListingCard, int64, error)
}
