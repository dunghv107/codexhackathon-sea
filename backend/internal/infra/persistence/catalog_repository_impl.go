package persistence

import (
	"context"
	"strings"

	"gorm.io/gorm"

	"github.com/tiloneeee/ai-fraud-detection/backend/internal/domain/entity"
	"github.com/tiloneeee/ai-fraud-detection/backend/internal/domain/repository"
	apperror "github.com/tiloneeee/ai-fraud-detection/backend/pkg/error"
)

// listingCardSelect reads money as text so numeric(18,2) keeps its "123.45" form.
const listingCardSelect = `
SELECT l.listing_id, l.shop_id, s.display_name AS shop_name, l.product_id,
       l.batch_id, s.batch_id AS shop_batch_id, p.batch_id AS product_batch_id,
       p.display_name AS product_name, p.category_code, p.category_name_en,
       l.current_price::text AS current_price, l.currency, l.stock_quantity
FROM listings l
JOIN shops s ON s.shop_id = l.shop_id
JOIN products p ON p.product_id = l.product_id`

const listingSelect = `
SELECT listing_id, shop_id, product_id, current_price::text AS current_price, currency,
       stock_quantity, batch_id, source_record_id
FROM listings`

type catalogRepositoryImpl struct {
	db *gorm.DB
}

// NewCatalogRepository creates a new catalog repository
func NewCatalogRepository(db *gorm.DB) repository.CatalogRepository {
	return &catalogRepositoryImpl{db: db}
}

func (r *catalogRepositoryImpl) ListCategories(ctx context.Context) ([]entity.Category, error) {
	var cats []entity.Category
	if err := dbFrom(ctx, r.db).Raw(`
SELECT category_code, max(category_name_en) AS category_name_en
FROM products WHERE category_code IS NOT NULL
GROUP BY category_code ORDER BY category_code`).Scan(&cats).Error; err != nil {
		return nil, wrapListError(err, "danh mục")
	}
	return cats, nil
}

func (r *catalogRepositoryImpl) FindCategoryName(ctx context.Context, code string) (*string, error) {
	var cat entity.Category
	res := dbFrom(ctx, r.db).Raw(`
SELECT category_code, max(category_name_en) AS category_name_en
FROM products WHERE category_code = ? GROUP BY category_code`, code).Scan(&cat)
	if res.Error != nil {
		return nil, wrapFindError(res.Error, "danh mục")
	}
	if res.RowsAffected == 0 {
		return nil, apperror.ErrNotFound.WithMessage("Không tìm thấy danh mục")
	}
	return cat.CategoryNameEn, nil
}

func (r *catalogRepositoryImpl) ListShops(ctx context.Context, offset, limit int) ([]entity.ShopCard, int64, error) {
	var total int64
	if err := dbFrom(ctx, r.db).Model(&entity.Shop{}).Count(&total).Error; err != nil {
		return nil, 0, wrapListError(err, "shop")
	}
	var shops []entity.ShopCard
	if err := dbFrom(ctx, r.db).Raw(shopCardSQL("")+`
ORDER BY listing_count DESC, s.shop_id LIMIT ? OFFSET ?`, limit, offset).Scan(&shops).Error; err != nil {
		return nil, 0, wrapListError(err, "shop")
	}
	return shops, total, nil
}

func (r *catalogRepositoryImpl) FindShopCard(ctx context.Context, shopID string) (*entity.ShopCard, error) {
	var shop entity.ShopCard
	res := dbFrom(ctx, r.db).Raw(shopCardSQL("WHERE s.shop_id = ?"), shopID).Scan(&shop)
	if res.Error != nil {
		return nil, wrapFindError(res.Error, "shop")
	}
	if res.RowsAffected == 0 {
		return nil, apperror.ErrNotFound.WithMessage("Không tìm thấy shop")
	}
	return &shop, nil
}

func shopCardSQL(where string) string {
	return `
SELECT s.shop_id, s.display_name, count(l.listing_id) AS listing_count
FROM shops s
LEFT JOIN listings l ON l.shop_id = s.shop_id AND l.current_price IS NOT NULL
` + where + `
GROUP BY s.shop_id, s.display_name`
}

func (r *catalogRepositoryImpl) ListListings(ctx context.Context, f entity.ListingFilter, offset, limit int) ([]entity.ListingCard, int64, error) {
	var conds []string
	var args []any
	if f.PricedOnly {
		conds = append(conds, "l.current_price IS NOT NULL")
	}
	if f.ShopID != "" {
		conds, args = append(conds, "l.shop_id = ?"), append(args, f.ShopID)
	}
	if f.ListingID != "" {
		conds, args = append(conds, "l.listing_id = ?"), append(args, f.ListingID)
	}
	if f.CategoryCode != "" {
		conds, args = append(conds, "p.category_code = ?"), append(args, f.CategoryCode)
	}
	where := ""
	if len(conds) > 0 {
		where = "\nWHERE " + strings.Join(conds, " AND ")
	}

	var total int64
	if err := dbFrom(ctx, r.db).Raw(`SELECT count(*) FROM listings l JOIN products p ON p.product_id = l.product_id`+where, args...).
		Scan(&total).Error; err != nil {
		return nil, 0, wrapListError(err, "sản phẩm")
	}
	var items []entity.ListingCard
	if err := dbFrom(ctx, r.db).Raw(listingCardSelect+where+"\nORDER BY l.listing_id LIMIT ? OFFSET ?", append(args, limit, offset)...).
		Scan(&items).Error; err != nil {
		return nil, 0, wrapListError(err, "sản phẩm")
	}
	return items, total, nil
}

func (r *catalogRepositoryImpl) ListShopsBySeller(ctx context.Context, sellerID string) ([]entity.Shop, error) {
	var shops []entity.Shop
	if err := dbFrom(ctx, r.db).Raw(`SELECT s.*, (SELECT count(*) FROM orders o WHERE o.shop_id=s.shop_id AND EXISTS (SELECT 1 FROM risk_assessments a WHERE a.order_id=o.order_id)) AS assessed_orders FROM shops s WHERE s.owner_seller_id=? ORDER BY assessed_orders DESC, s.display_name, s.shop_id`, sellerID).Scan(&shops).Error; err != nil {
		return nil, wrapListError(err, "shop")
	}
	return shops, nil
}

func (r *catalogRepositoryImpl) FindOwnedShop(ctx context.Context, shopID, sellerID string) (*entity.Shop, error) {
	var shop entity.Shop
	if err := dbFrom(ctx, r.db).Where("shop_id = ? AND owner_seller_id = ?", shopID, sellerID).First(&shop).Error; err != nil {
		return nil, wrapFindError(err, "shop")
	}
	return &shop, nil
}

func (r *catalogRepositoryImpl) CreateShop(ctx context.Context, shop *entity.Shop) error {
	if err := dbFrom(ctx, r.db).Create(shop).Error; err != nil {
		return wrapCreateError(err, "shop")
	}
	return nil
}

func (r *catalogRepositoryImpl) CreateProduct(ctx context.Context, product *entity.Product) error {
	if err := dbFrom(ctx, r.db).Create(product).Error; err != nil {
		return wrapCreateError(err, "sản phẩm")
	}
	return nil
}

func (r *catalogRepositoryImpl) CreateListing(ctx context.Context, listing *entity.Listing) error {
	var count int64
	if err := dbFrom(ctx, r.db).Model(&entity.Shop{}).Where("shop_id = ? AND batch_id = ?", listing.ShopID, listing.BatchID).Count(&count).Error; err != nil {
		return err
	}
	if count != 1 {
		return apperror.ErrValidation.WithMessage("Shop phải thuộc batch demo olist_vn_sim để tạo listing mới")
	}
	if err := dbFrom(ctx, r.db).Create(listing).Error; err != nil {
		return wrapCreateError(err, "sản phẩm")
	}
	return nil
}

func (r *catalogRepositoryImpl) LockListing(ctx context.Context, shopID, listingID string) (*entity.Listing, error) {
	var listing entity.Listing
	res := dbFrom(ctx, r.db).Raw(listingSelect+" WHERE listing_id = ? AND shop_id = ? FOR UPDATE", listingID, shopID).Scan(&listing)
	if res.Error != nil {
		return nil, wrapFindError(res.Error, "sản phẩm")
	}
	if res.RowsAffected == 0 {
		return nil, apperror.ErrNotFound.WithMessage("Không tìm thấy sản phẩm")
	}
	return &listing, nil
}

func (r *catalogRepositoryImpl) LockListings(ctx context.Context, listingIDs []string) ([]entity.ListingCard, error) {
	var listings []entity.ListingCard
	// Fixed lock order avoids deadlocks between concurrent checkouts.
	if err := dbFrom(ctx, r.db).Raw(listingCardSelect+"\nWHERE l.listing_id IN ? ORDER BY l.listing_id FOR UPDATE OF l", listingIDs).
		Scan(&listings).Error; err != nil {
		return nil, wrapListError(err, "sản phẩm")
	}
	return listings, nil
}

func (r *catalogRepositoryImpl) UpdateListing(ctx context.Context, listing *entity.Listing, columns ...string) error {
	if err := dbFrom(ctx, r.db).Model(listing).Select(columns).Updates(listing).Error; err != nil {
		return wrapUpdateError(err, "sản phẩm")
	}
	return nil
}
