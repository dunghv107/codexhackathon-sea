package persistence_test

import (
	"context"
	"encoding/json"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"github.com/tiloneeee/ai-fraud-detection/backend/internal/domain/entity"
	"github.com/tiloneeee/ai-fraud-detection/backend/internal/infra/persistence"
	"github.com/tiloneeee/ai-fraud-detection/backend/internal/usecase/service"
	"github.com/tiloneeee/ai-fraud-detection/backend/internal/usecase/service/serviceimpl"
)

// Each run owns an isolated schema; neither dataset files nor existing data are loaded or changed.
func TestOlistReviewIntegration(t *testing.T) {
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL not set; PostgreSQL integration not run")
	}
	for _, schemaFile := range []string{"../../../../olist_cod/olist_cod/schema.sql", "../../../migrations/000002_create_sfd_schema.up.sql"} {
		t.Run(filepath.Base(schemaFile), func(t *testing.T) {
			schema := "olist_test_" + strings.ReplaceAll(uuid.NewString(), "-", "")
			uri, err := url.Parse(dsn)
			if err != nil {
				t.Fatal(err)
			}
			q := uri.Query()
			q.Set("search_path", schema+",public")
			uri.RawQuery = q.Encode()
			db, err := gorm.Open(postgres.Open(uri.String()), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
			if err != nil {
				t.Fatal(err)
			}
			sqlDB, err := db.DB()
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { db.Exec("ROLLBACK"); db.Exec("DROP SCHEMA IF EXISTS " + schema + " CASCADE"); sqlDB.Close() })
			exec := func(sql string, args ...any) {
				t.Helper()
				if err := db.Exec(sql, args...).Error; err != nil {
					t.Fatal(err)
				}
			}
			for _, path := range []string{schemaFile, "../../../migrations/000004_align_olist_cod.up.sql"} {
				ddl, err := os.ReadFile(path)
				if err != nil {
					t.Fatal(err)
				}
				exec(strings.ReplaceAll(string(ddl), "sfd", schema))
			}
			// Reattaching a canonical database is safe and does not recreate source tables.
			ddl, err := os.ReadFile("../../../migrations/000004_align_olist_cod.up.sql")
			if err != nil {
				t.Fatal(err)
			}
			exec(strings.ReplaceAll(string(ddl), "sfd", schema))
			exec(`INSERT INTO data_batches VALUES ('sim','test',true,'isolated test',NULL,now());
INSERT INTO buyers(buyer_id,batch_id,source_record_id) VALUES ('b','sim','b');
INSERT INTO sellers(seller_id,display_name,batch_id,source_record_id) VALUES ('seller','Seller','app','seller');
INSERT INTO shops(shop_id,display_name,owner_actor_ref,owner_seller_id,batch_id,source_record_id) VALUES ('s','Shop','source-actor','seller','sim','s'),('other','Other',NULL,NULL,'sim','other');
INSERT INTO products(product_id,batch_id,source_record_id) VALUES ('p','sim','p');
INSERT INTO listings(listing_id,shop_id,product_id,current_price,currency,stock_quantity,batch_id,source_record_id) VALUES ('l','s','p',100,'VND',5,'sim','l');
INSERT INTO checkouts VALUES ('c','b','2026-01-01','cod','VND',100,'sim','c');
INSERT INTO checkout_addresses(checkout_id,ward_code,house_no,parse_ok,batch_id,source_record_id) VALUES ('c','ward','12',true,'sim','c');
INSERT INTO orders(order_id,checkout_id,shop_id,batch_id,source_record_id) VALUES ('o','c','s','sim','o');
INSERT INTO order_items VALUES ('i','o','s','l',1,100,0,'sim','i');
INSERT INTO payments VALUES ('pay','c','cod',100,'pending',NULL,'sim','pay');
INSERT INTO rule_versions(rule_version,config,config_sha256,activated_at,approved_by,batch_id) VALUES ('v','{"rules":[{"rule_id":"R3","description":"Verified buyer failures","enabled":true}]}',repeat('a',64),'2026-01-01','test','sim');
INSERT INTO risk_assessments(assessment_id,order_id,assessed_at,input_cutoff_at,rule_version,score,risk_level,feature_snapshot,reasons,batch_id) VALUES ('a','o','2026-01-01','2026-01-01','v',80,'high','{"R3":{"value":3}}','[{"rule_id":"R3","contribution":40}]','sim');
INSERT INTO cases VALUES ('case','o','delivery','2026-01-02','2026-01-02','sim','case');
INSERT INTO case_resolutions VALUES ('resolution','case','verified','seller','Seller error','test','2026-01-02','2026-01-02','sim','resolution');`)
			ctx := context.Background()
			orders := persistence.NewOrderRepository(db)
			catalog := persistence.NewCatalogRepository(db)
			seller := serviceimpl.NewSellerService(catalog, orders, persistence.NewTransactor(db))
			rows, total, err := orders.ListOrders(ctx, entity.OrderFilter{ShopID: "s", BatchID: "sim", RiskLevel: "high"}, 0, 20)
			if err != nil || total != 1 || len(rows) != 1 {
				t.Fatalf("list: %v %d %v", rows, total, err)
			}
			if !rows[0].IsSynthetic || rows[0].BatchID != "sim" {
				t.Fatal("provenance missing")
			}
			if _, err := seller.GetOrder(ctx, "other", "o"); err == nil {
				t.Fatal("foreign shop can read order")
			}
			shops, err := catalog.ListShopsBySeller(ctx, "seller")
			if err != nil || len(shops) != 1 || shops[0].AssessedOrders != 1 {
				t.Fatalf("shops: %v %v", shops, err)
			}
			if _, err := catalog.FindOwnedShop(ctx, "other", "seller"); err == nil {
				t.Fatal("foreign shop accessible")
			}
			detail, err := seller.GetOrder(ctx, "s", "o")
			if err != nil {
				t.Fatal(err)
			}
			if detail.LatestAssessment.BatchID != "sim" || !json.Valid(detail.LatestAssessment.RuleConfig) || !strings.Contains(string(detail.Cases), "Seller error") || detail.Address.WardCode == nil {
				t.Fatal("incomplete Olist detail")
			}
			cmd := service.DecisionCommand{ShopID: "s", SellerID: "seller", OrderID: "o", AssessmentID: "a", Action: entity.DecisionHold, Reason: "Manual review"}
			if _, err := seller.Decide(ctx, cmd); err != nil {
				t.Fatal(err)
			}
			if _, err := seller.UpdateFulfillment(ctx, service.FulfillmentCommand{ShopID: "s", OrderID: "o", Status: "packing"}); err == nil {
				t.Fatal("held order packed")
			}
			cmd.AssessmentID = "stale"
			if _, err := seller.Decide(ctx, cmd); err == nil {
				t.Fatal("stale assessment accepted")
			}
			cmd.AssessmentID = "a"
			cmd.Action = entity.DecisionRelease
			if _, err := seller.Decide(ctx, cmd); err != nil {
				t.Fatal(err)
			}
			for _, status := range []string{"packing", "shipped", "delivered"} {
				if _, err := seller.UpdateFulfillment(ctx, service.FulfillmentCommand{ShopID: "s", OrderID: "o", Status: status}); err != nil {
					t.Fatal(err)
				}
			}
			detail, err = seller.GetOrder(ctx, "s", "o")
			if err != nil {
				t.Fatal(err)
			}
			if len(detail.Decisions) != 2 || detail.Decisions[0].BatchID != "sim" || len(detail.Events) != 1 || detail.Events[0].BatchID != "sim" || detail.Events[0].AvailableAt == nil {
				t.Fatal("decision/event provenance missing")
			}
			if _, err := seller.Decide(ctx, cmd); err == nil {
				t.Fatal("delivered order review allowed")
			}
			// Database rejects a decision or assessment with a mismatched batch and a missing cutoff.
			if err := db.Exec("UPDATE review_decisions SET batch_id='app'").Error; err == nil {
				t.Fatal("cross-batch decision accepted")
			}
			if err := db.Exec("UPDATE risk_assessments SET batch_id='app'").Error; err == nil {
				t.Fatal("cross-batch assessment accepted")
			}
			if err := db.Exec("UPDATE risk_assessments SET input_cutoff_at=NULL").Error; err == nil {
				t.Fatal("unknown cutoff accepted")
			}
			buyer := serviceimpl.NewBuyerService(persistence.NewAddressRepository(db), catalog, orders, persistence.NewTransactor(db))
			addr, err := buyer.SaveAddress(ctx, service.SaveAddressCommand{BuyerID: "b", Label: "Home", AddressRaw: "12 Test", Region: "Test", CountryCode: "VN"})
			if err != nil {
				t.Fatal(err)
			}
			result, err := buyer.Checkout(ctx, service.CheckoutCommand{BuyerID: "b", SavedAddressID: addr.SavedAddressID, PaymentMethod: "cod", Items: []service.CheckoutItem{{ListingID: "l", Quantity: 1}}})
			if err != nil {
				t.Fatal(err)
			}
			var snapshot struct {
				BatchID     string
				AvailableAt time.Time
				PhoneKey    *string
			}
			if err := db.Table("checkout_identity_snapshots").Where("checkout_id=?", result.Checkout.CheckoutID).Take(&snapshot).Error; err != nil {
				t.Fatal(err)
			}
			if snapshot.BatchID != "app" || snapshot.AvailableAt.IsZero() || snapshot.PhoneKey != nil {
				t.Fatal("new identity should exist with unknown keys")
			}
			fresh, err := seller.GetOrder(ctx, "s", result.Orders[0].OrderID)
			if err != nil {
				t.Fatal(err)
			}
			if fresh.LatestAssessment != nil {
				t.Fatal("checkout invented an assessment")
			}
			if _, err := buyer.CancelOrder(ctx, "b", fresh.OrderID, "Changed mind"); err != nil {
				t.Fatal(err)
			}
		})
	}
}
