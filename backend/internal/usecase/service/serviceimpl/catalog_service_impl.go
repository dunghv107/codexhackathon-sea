package serviceimpl

import (
	"context"

	"github.com/tiloneeee/ai-fraud-detection/backend/internal/domain/entity"
	"github.com/tiloneeee/ai-fraud-detection/backend/internal/domain/repository"
	"github.com/tiloneeee/ai-fraud-detection/backend/internal/usecase/service"
)

type catalogServiceImpl struct {
	catalogRepo repository.CatalogRepository
}

// NewCatalogService creates a new catalog service
func NewCatalogService(catalogRepo repository.CatalogRepository) service.CatalogService {
	return &catalogServiceImpl{catalogRepo: catalogRepo}
}

func (s *catalogServiceImpl) ListCategories(ctx context.Context) ([]entity.Category, error) {
	return s.catalogRepo.ListCategories(ctx)
}

func (s *catalogServiceImpl) ListShops(ctx context.Context, offset, limit int) ([]entity.ShopCard, int64, error) {
	return s.catalogRepo.ListShops(ctx, offset, limit)
}

func (s *catalogServiceImpl) GetShop(ctx context.Context, shopID string) (*entity.ShopCard, error) {
	return s.catalogRepo.FindShopCard(ctx, shopID)
}

func (s *catalogServiceImpl) ListListings(ctx context.Context, shopID, categoryCode string, offset, limit int) ([]entity.ListingCard, int64, error) {
	return s.catalogRepo.ListListings(ctx, entity.ListingFilter{ShopID: shopID, CategoryCode: categoryCode, PricedOnly: true}, offset, limit)
}
