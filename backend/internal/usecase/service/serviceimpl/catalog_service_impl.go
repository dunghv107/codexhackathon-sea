package serviceimpl

import (
	"context"
	"strings"
	"unicode/utf8"

	apperror "github.com/tiloneeee/ai-fraud-detection/backend/pkg/error"

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

func (s *catalogServiceImpl) ListListings(ctx context.Context, f entity.ListingFilter, offset, limit int) ([]entity.ListingCard, int64, error) {
	f.Query = strings.TrimSpace(f.Query)
	f.Currency = strings.ToUpper(strings.TrimSpace(f.Currency))
	if utf8.RuneCountInString(f.Query) > 120 || (f.Currency != "" && !currencyCode.MatchString(f.Currency)) {
		return nil, 0, apperror.ErrValidation.WithMessage("Từ khóa hoặc mã tiền tệ không hợp lệ")
	}
	switch f.Sort {
	case "", "name":
	case "price_asc", "price_desc":
		if f.Currency == "" {
			return nil, 0, apperror.ErrValidation.WithMessage("Chọn tiền tệ trước khi sắp xếp theo giá")
		}
	default:
		return nil, 0, apperror.ErrValidation.WithMessage("Cách sắp xếp không hợp lệ")
	}
	f.PricedOnly = true
	return s.catalogRepo.ListListings(ctx, f, offset, limit)
}
func (s *catalogServiceImpl) GetListing(ctx context.Context, listingID string) (*entity.ListingCard, error) {
	items, _, err := s.catalogRepo.ListListings(ctx, entity.ListingFilter{ListingID: listingID, PricedOnly: true}, 0, 1)
	if err != nil {
		return nil, err
	}
	if len(items) == 0 {
		return nil, apperror.ErrNotFound.WithMessage("Không tìm thấy sản phẩm")
	}
	return &items[0], nil
}
