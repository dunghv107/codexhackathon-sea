package persistence_test

import (
	"context"
	"strings"
	"testing"

	"github.com/tiloneeee/ai-fraud-detection/backend/internal/domain/entity"
	"github.com/tiloneeee/ai-fraud-detection/backend/internal/infra/persistence"
	"github.com/tiloneeee/ai-fraud-detection/backend/internal/usecase/service"
	"github.com/tiloneeee/ai-fraud-detection/backend/internal/usecase/service/serviceimpl"
	"gorm.io/gorm"
)

// Called for both canonical Olist and migrated schemas; all writes roll back.
func testMarketplaceCatalog(t *testing.T, db *gorm.DB) {
	t.Helper()
	tx := db.Begin()
	if tx.Error != nil {
		t.Fatal(tx.Error)
	}
	defer tx.Rollback()
	ctx := context.Background()
	repo := persistence.NewCatalogRepository(tx)
	orders := persistence.NewOrderRepository(tx)
	catalog := serviceimpl.NewCatalogService(repo)
	seller := serviceimpl.NewSellerService(repo, orders, persistence.NewTransactor(tx), nil, nil)
	stock := 3
	item, err := seller.CreateListing(ctx, service.CreateListingCommand{ShopID: "s", ProductName: "Đèn 100% cotton", CurrentPrice: "250000", StockQuantity: &stock, Description: "Mô tả mới", ImageURL: "https://example.com/lamp.jpg"})
	if err != nil {
		t.Fatal(err)
	}
	if item.Currency != "VND" || item.Description == nil || *item.Description != "Mô tả mới" {
		t.Fatalf("incomplete listing: %+v", item)
	}
	cheaper, err := seller.CreateListing(ctx, service.CreateListingCommand{ShopID: "s", ProductName: "Túi vải", CurrentPrice: "50000", StockQuantity: &stock})
	if err != nil {
		t.Fatal(err)
	}
	for _, q := range []string{"100%", "cotton"} {
		rows, total, err := catalog.ListListings(ctx, entity.ListingFilter{Query: q}, 0, 20)
		if err != nil || total != 1 || rows[0].ListingID != item.ListingID {
			t.Fatalf("literal search %q: %d %v", q, total, err)
		}
	}
	_, total, err := catalog.ListListings(ctx, entity.ListingFilter{Query: "' OR 1=1 --"}, 0, 20)
	if err != nil || total != 0 {
		t.Fatalf("unsafe query: %d %v", total, err)
	}
	rows, total, err := catalog.ListListings(ctx, entity.ListingFilter{Currency: "vnd", Sort: "price_desc", InStock: true}, 0, 1)
	if err != nil || total != 3 || len(rows) != 1 || rows[0].ListingID != item.ListingID {
		t.Fatalf("sort/pagination: %+v %d %v", rows, total, err)
	}
	if _, _, err = catalog.ListListings(ctx, entity.ListingFilter{Sort: "price_asc"}, 0, 20); err == nil {
		t.Fatal("cross-currency price sorting allowed")
	}
	if _, _, err = catalog.ListListings(ctx, entity.ListingFilter{Sort: "price; DROP TABLE listings"}, 0, 20); err == nil {
		t.Fatal("unvalidated sorting")
	}
	if _, _, err = catalog.ListListings(ctx, entity.ListingFilter{Query: strings.Repeat("x", 121)}, 0, 20); err == nil {
		t.Fatal("unbounded search")
	}
	title, desc, image := "Tên đã sửa", "", ""
	zero := 0
	updated, err := seller.UpdateListing(ctx, service.UpdateListingCommand{ShopID: "s", ListingID: item.ListingID, ProductName: &title, Description: &desc, ImageURL: &image, StockQuantity: &zero})
	if err != nil || *updated.ProductName != title || *updated.Description != "" || *updated.ImageURL != "" {
		t.Fatalf("edit/clear fields: %+v %v", updated, err)
	}
	rows, total, err = catalog.ListListings(ctx, entity.ListingFilter{Query: title, InStock: true}, 0, 20)
	if err != nil || total != 0 {
		t.Fatalf("stock filter: %+v %v", rows, err)
	}
	if _, err = seller.UpdateListing(ctx, service.UpdateListingCommand{ShopID: "other", ListingID: cheaper.ListingID, ProductName: &title}); err == nil {
		t.Fatal("foreign shop edited listing")
	}
	original := entity.Product{}
	if err = tx.First(&original, "product_id=?", item.ProductID).Error; err != nil || *original.DisplayName != "Đèn 100% cotton" {
		t.Fatal("listing edit mutated source product")
	}
	if _, err = catalog.GetListing(ctx, "missing"); err == nil {
		t.Fatal("missing listing not rejected")
	}
	for _, uri := range []string{"javascript:alert(1)", "data:image/png;base64,abc", "http://example.com/image.jpg", "https://name:password@example.com/image.jpg"} {
		if _, err = seller.UpdateListing(ctx, service.UpdateListingCommand{ShopID: "s", ListingID: item.ListingID, ImageURL: &uri}); err == nil {
			t.Fatalf("unsafe image accepted: %s", uri)
		}
	}
	filtered, n, err := orders.ListOrders(ctx, entity.OrderFilter{BuyerID: "b", DisplayStatus: "confirming"}, 0, 20)
	if err != nil || n != 1 || filtered[0].DisplayStatus() != "confirming" {
		t.Fatalf("buyer status: %d %v", n, err)
	}
	_, n, err = orders.ListOrders(ctx, entity.OrderFilter{BuyerID: "foreign", DisplayStatus: "confirming"}, 0, 20)
	if err != nil || n != 0 {
		t.Fatal("buyer filter leaked orders")
	}
}
