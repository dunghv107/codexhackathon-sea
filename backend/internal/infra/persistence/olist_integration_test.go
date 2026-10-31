package persistence_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"github.com/tiloneeee/ai-fraud-detection/backend/internal/domain/entity"
	"github.com/tiloneeee/ai-fraud-detection/backend/internal/infra/kbs"
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
			for _, path := range []string{schemaFile, "../../../migrations/000004_align_olist_cod.up.sql", "../../../migrations/000006_phone_verifications.up.sql", "../../../migrations/000007_listing_content.up.sql", "../../../migrations/000008_order_item_name.up.sql"} {
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
			phoneDDL, err := os.ReadFile("../../../migrations/000006_phone_verifications.up.sql")
			if err != nil {
				t.Fatal(err)
			}
			exec(strings.ReplaceAll(string(phoneDDL), "sfd", schema))
			riskDDL, err := os.ReadFile("../../../migrations/000012_adaptive_evidence.up.sql")
			if err != nil {
				t.Fatal(err)
			}
			exec(strings.ReplaceAll(string(riskDDL), "sfd", schema))
			exec(`INSERT INTO data_batches VALUES ('olist_vn_sim','test',true,'isolated test',NULL,now());
INSERT INTO buyers(buyer_id,batch_id,source_record_id) VALUES ('b','olist_vn_sim','b');
INSERT INTO sellers(seller_id,display_name,batch_id,source_record_id) VALUES ('seller','Seller','app','seller');
INSERT INTO shops(shop_id,display_name,owner_actor_ref,owner_seller_id,batch_id,source_record_id) VALUES ('s','Shop','source-actor','seller','olist_vn_sim','s'),('other','Other',NULL,NULL,'olist_vn_sim','other');
INSERT INTO products(product_id,batch_id,source_record_id) VALUES ('p','olist_vn_sim','p');
INSERT INTO listings(listing_id,shop_id,product_id,current_price,currency,stock_quantity,batch_id,source_record_id) VALUES ('l','s','p',100,'VND',5,'olist_vn_sim','l');
INSERT INTO checkouts VALUES ('c','b','2026-01-01','cod','VND',100,'olist_vn_sim','c');
INSERT INTO checkout_addresses(checkout_id,ward_code,house_no,parse_ok,batch_id,source_record_id) VALUES ('c','ward','12',true,'olist_vn_sim','c');
INSERT INTO orders(order_id,checkout_id,shop_id,batch_id,source_record_id) VALUES ('o','c','s','olist_vn_sim','o');
INSERT INTO order_items VALUES ('i','o','s','l',1,100,0,'olist_vn_sim','i');
INSERT INTO payments VALUES ('pay','c','cod',100,'pending',NULL,'olist_vn_sim','pay');
INSERT INTO rule_versions(rule_version,config,config_sha256,activated_at,approved_by,batch_id) VALUES ('v','{"rules":[{"rule_id":"R3","description":"Verified buyer failures","enabled":true}]}',repeat('a',64),'2026-01-01','test','olist_vn_sim');
INSERT INTO risk_assessments(assessment_id,order_id,assessed_at,input_cutoff_at,rule_version,score,risk_level,feature_snapshot,reasons,batch_id) VALUES ('a','o','2026-01-01','2026-01-01','v',80,'high','{"R3":{"value":3}}','[{"rule_id":"R3","contribution":40}]','olist_vn_sim');
INSERT INTO cases VALUES ('case','o','delivery','2026-01-02','2026-01-02','olist_vn_sim','case');
INSERT INTO case_resolutions VALUES ('resolution','case','verified','seller','Seller error','test','2026-01-02','2026-01-02','olist_vn_sim','resolution');`)
			batchMigration, err := os.ReadFile("../../../migrations/000005_demo_scoring_batch.up.sql")
			if err != nil {
				t.Fatal(err)
			}
			for i := 0; i < 2; i++ {
				exec(strings.ReplaceAll(string(batchMigration), "sfd", schema))
			}
			var source string
			if err := db.Table("data_batches").Select("source_system").Where("batch_id = ?", entity.DemoBatchID).Scan(&source).Error; err != nil || source != "test" {
				t.Fatal("migration changed existing provenance")
			}
			ctx := context.Background()
			orders := persistence.NewOrderRepository(db)
			catalog := persistence.NewCatalogRepository(db)
			testMarketplaceCatalog(t, db)
			var scoreCalls atomic.Int32
			scorer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				scoreCalls.Add(1)
				var count int64
				orderID := strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, "/api/v1/orders/"), "/score")
				if err := db.Table("orders").Where("order_id = ? AND shop_id = ?", orderID, r.Header.Get("X-Shop-ID")).Count(&count).Error; err != nil || count != 1 {
					t.Error("KBS was called before checkout committed or with the wrong shop")
				}
				w.WriteHeader(http.StatusServiceUnavailable)
			}))
			defer scorer.Close()
			risk, err := kbs.NewClient(scorer.URL)
			if err != nil {
				t.Fatal(err)
			}
			verificationRepo := persistence.NewVerificationRepository(db)
			verification := serviceimpl.NewVerificationService(verificationRepo, orders, persistence.NewTransactor(db), true)
			phone, err := serviceimpl.NewPhoneIdentityService(verificationRepo, strings.Repeat("test", 8), true)
			if err != nil {
				t.Fatal(err)
			}
			seller := serviceimpl.NewSellerService(catalog, orders, persistence.NewTransactor(db), risk, verification)
			rows, total, err := orders.ListOrders(ctx, entity.OrderFilter{ShopID: "s", BatchID: "olist_vn_sim", RiskLevel: "high"}, 0, 20)
			if err != nil || total != 1 || len(rows) != 1 {
				t.Fatalf("list: %v %d %v", rows, total, err)
			}
			if !rows[0].IsSynthetic || rows[0].BatchID != "olist_vn_sim" {
				t.Fatal("provenance missing")
			}
			if _, err := seller.GetOrder(ctx, "other", "o"); err == nil {
				t.Fatal("foreign shop can read order")
			}
			if _, err := seller.Score(ctx, "other", "o", true); err == nil || scoreCalls.Load() != 0 {
				t.Fatal("foreign shop reached scorer")
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
			if detail.LatestAssessment.BatchID != "olist_vn_sim" || !json.Valid(detail.LatestAssessment.RuleConfig) || !strings.Contains(string(detail.Cases), "Seller error") || detail.Address.WardCode == nil {
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
			if len(detail.Decisions) != 2 || detail.Decisions[0].BatchID != "olist_vn_sim" || len(detail.Events) != 3 || detail.Events[0].BatchID != "olist_vn_sim" || detail.Events[0].AvailableAt == nil {
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
			buyer := serviceimpl.NewBuyerService(persistence.NewAddressRepository(db), catalog, orders, persistence.NewTransactor(db), risk, phone, verification)
			addr, err := buyer.SaveAddress(ctx, service.SaveAddressCommand{BuyerID: "b", Label: "Home", AddressRaw: "12 Test", Region: "Test", CountryCode: "VN"})
			if err != nil {
				t.Fatal(err)
			}
			result, err := buyer.Checkout(ctx, service.CheckoutCommand{Phone: "0900000001", BuyerID: "b", SavedAddressID: addr.SavedAddressID, PaymentMethod: "cod", Items: []service.CheckoutItem{{ListingID: "l", Quantity: 1}}})
			if err != nil {
				t.Fatal(err)
			}
			if scoreCalls.Load() != 1 {
				t.Fatal("checkout did not call KBS")
			}
			var snapshot struct {
				BatchID     string
				AvailableAt time.Time
				PhoneKey    *string
			}
			if err := db.Table("checkout_identity_snapshots").Where("checkout_id=?", result.Checkout.CheckoutID).Take(&snapshot).Error; err != nil {
				t.Fatal(err)
			}
			if snapshot.BatchID != entity.DemoBatchID || snapshot.AvailableAt.IsZero() || (snapshot.PhoneKey == nil || !strings.HasPrefix(*snapshot.PhoneKey, "hmac:v1:")) {
				t.Fatal("new identity must contain phone HMAC")
			}
			exec("INSERT INTO buyers(buyer_id,batch_id,source_record_id) VALUES ('legacy','app','legacy')")
			legacyAddress, err := buyer.SaveAddress(ctx, service.SaveAddressCommand{BuyerID: "legacy", Label: "Home", AddressRaw: "Legacy", Region: "Test", CountryCode: "VN"})
			if err != nil {
				t.Fatal(err)
			}
			_, err = buyer.Checkout(ctx, service.CheckoutCommand{Phone: "0900000001", BuyerID: "legacy", SavedAddressID: legacyAddress.SavedAddressID, PaymentMethod: "cod", Items: []service.CheckoutItem{{ListingID: "l", Quantity: 1}}})
			if err == nil || scoreCalls.Load() != 1 {
				t.Fatal("legacy buyer checkout reached scorer")
			}
			var stock int
			if err := db.Table("listings").Select("stock_quantity").Where("listing_id = 'l'").Scan(&stock).Error; err != nil || stock != 4 {
				t.Fatal("rejected checkout changed stock")
			}
			fresh, err := seller.GetOrder(ctx, "s", result.Orders[0].OrderID)
			if err != nil {
				t.Fatal(err)
			}
			if !strings.HasPrefix(fresh.OrderID, "live:") || fresh.BatchID != entity.DemoBatchID || fresh.ReviewStatus != entity.ReviewPending {
				t.Fatal("checkout provenance/status mismatch")
			}
			if fresh.LatestAssessment != nil {
				t.Fatal("checkout invented an assessment")
			}
			if fresh.PhoneMasked != "***0001" || fresh.Verification != nil {
				t.Fatal("contact masking or verification mismatch")
			}
			checkVerification(t, db, fresh.OrderID, fresh.CheckoutID)
			if _, err := buyer.CancelOrder(ctx, "b", fresh.OrderID, "Changed mind"); err != nil {
				t.Fatal(err)
			}
			// A single checkout identity/contact serves all shop orders; mapping never rewrites old rows.
			exec(`INSERT INTO checkout_identity_snapshots(checkout_id,phone_key,available_at,batch_id) VALUES ('c','sim:existing','2026-01-01','olist_vn_sim')`)
			exec(`INSERT INTO demo_phone_mappings VALUES ('olist_vn_sim','+84900000002','sim:existing','medium','authored test')`)
			exec(`INSERT INTO listings(listing_id,shop_id,product_id,current_price,currency,batch_id,source_record_id) VALUES ('other-l','other','p',100,'VND','olist_vn_sim','other-l')`)
			multi, err := buyer.Checkout(ctx, service.CheckoutCommand{BuyerID: "b", SavedAddressID: addr.SavedAddressID, Phone: "0900000002", PaymentMethod: "cod", Items: []service.CheckoutItem{{ListingID: "l", Quantity: 1}, {ListingID: "other-l", Quantity: 1}}})
			if err != nil || len(multi.Orders) != 2 {
				t.Fatalf("multi-shop checkout: %v", err)
			}
			var key string
			if err := db.Table("checkout_identity_snapshots").Select("phone_key").Where("checkout_id=?", multi.Checkout.CheckoutID).Scan(&key).Error; err != nil || key != "sim:existing" {
				t.Fatal("demo mapping missing")
			}
			var count int64
			if err := db.Table("checkout_contacts").Where("checkout_id=?", multi.Checkout.CheckoutID).Count(&count).Error; err != nil || count != 1 {
				t.Fatal("contact not shared across shops")
			}
			if masked, err := verificationRepo.MaskedPhone(ctx, "c"); err != nil || masked != "" {
				t.Fatal("legacy contact invented")
			}
			// A storage failure after stock mutation must roll back checkout, contact and snapshot together.
			exec(`ALTER TABLE checkout_contacts ADD CONSTRAINT reject_test_phone CHECK (phone_e164 <> '+84900000003')`)
			var before int64
			db.Table("checkouts").Count(&before)
			_, err = buyer.Checkout(ctx, service.CheckoutCommand{BuyerID: "b", SavedAddressID: addr.SavedAddressID, Phone: "0900000003", PaymentMethod: "cod", Items: []service.CheckoutItem{{ListingID: "l", Quantity: 1}}})
			if err == nil {
				t.Fatal("contact failure ignored")
			}
			db.Table("checkouts").Count(&count)
			if count != before {
				t.Fatal("failed contact left a checkout")
			}
			if err := db.Table("listings").Select("stock_quantity").Where("listing_id='l'").Scan(&stock).Error; err != nil || stock != 3 {
				t.Fatal("failed contact consumed stock")
			}
		})
	}
}
