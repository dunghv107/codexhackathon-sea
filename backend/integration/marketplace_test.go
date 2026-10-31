package integration_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/tiloneeee/ai-fraud-detection/backend/internal/domain/entity"
	"github.com/tiloneeee/ai-fraud-detection/backend/internal/infra/persistence"
	"github.com/tiloneeee/ai-fraud-detection/backend/internal/interface/api/handler"
	"github.com/tiloneeee/ai-fraud-detection/backend/internal/interface/api/middleware"
	"github.com/tiloneeee/ai-fraud-detection/backend/internal/interface/api/router"
	"github.com/tiloneeee/ai-fraud-detection/backend/internal/usecase/service"
	"github.com/tiloneeee/ai-fraud-detection/backend/internal/usecase/service/serviceimpl"
	"github.com/tiloneeee/ai-fraud-detection/backend/pkg/query"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

type riskStub struct {
	calls int
	err   error
}

func (r *riskStub) Score(_ context.Context, _ string, orderID string, _ bool) (*service.ScoreAssessment, error) {
	r.calls++
	return &service.ScoreAssessment{AssessmentID: "assessment", OrderID: orderID, RiskLevel: "medium", Score: 50}, r.err
}
func (r *riskStub) Network(_ context.Context, _ string, orderID string) (*service.RiskNetwork, error) {
	return &service.RiskNetwork{OrderID: orderID, InputCutoffAt: time.Now(), Nodes: []service.NetworkNode{{ID: "order", Kind: "checkout", Label: "Current checkout", Count: 1}}, Edges: []service.NetworkEdge{}}, r.err
}
func (r *riskStub) Explain(context.Context, string, string) (*service.RiskExplanation, error) {
	return &service.RiskExplanation{AssessmentID: "assessment", Source: "rules", Sentences: []string{"Verified evidence"}}, r.err
}

type fixture struct {
	t      *testing.T
	db     *gorm.DB
	engine *gin.Engine
	token  string
	risk   *riskStub
	users  service.UserService
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL is required for isolated PostgreSQL integration")
	}
	schema := "coverage_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	u, e := url.Parse(dsn)
	if e != nil {
		t.Fatal(e)
	}
	q := u.Query()
	q.Set("search_path", schema+",public")
	u.RawQuery = q.Encode()
	db, e := gorm.Open(postgres.Open(u.String()), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if e != nil {
		t.Fatal(e)
	}
	sql, e := db.DB()
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() { db.Exec("ROLLBACK"); db.Exec("DROP SCHEMA IF EXISTS " + schema + " CASCADE"); sql.Close() })
	for _, name := range []string{"000002_create_sfd_schema.up.sql", "000001_create_users.up.sql", "000003_seed_app_batch.up.sql", "000004_align_olist_cod.up.sql", "000005_demo_scoring_batch.up.sql", "000006_phone_verifications.up.sql"} {
		b, e := os.ReadFile(filepath.Join("..", "migrations", name))
		if e != nil {
			t.Fatal(e)
		}
		if e = db.Exec(strings.ReplaceAll(string(b), "sfd", schema)).Error; e != nil {
			t.Fatal(name, e)
		}
	}
	ur := persistence.NewUserRepository(db)
	tx := persistence.NewTransactor(db)
	users := serviceimpl.NewUserService(ur, tx)
	jwt := serviceimpl.NewJWTService("test-only-signing-key-not-a-production-secret", 10, 1)
	auth := serviceimpl.NewAuthService(ur, jwt)
	profile := serviceimpl.NewProfileService(persistence.NewProfileRepository(db))
	catalog := persistence.NewCatalogRepository(db)
	orders := persistence.NewOrderRepository(db)
	risk := &riskStub{}
	verificationRepo := persistence.NewVerificationRepository(db)
	phone, e := serviceimpl.NewPhoneIdentityService(verificationRepo, "test-phone-hmac-key-only-not-production", true)
	if e != nil {
		t.Fatal(e)
	}
	verification := serviceimpl.NewVerificationService(verificationRepo, orders, tx, true)
	buyer := serviceimpl.NewBuyerService(persistence.NewAddressRepository(db), catalog, orders, tx, risk, phone, verification)
	seller := serviceimpl.NewSellerService(catalog, orders, tx, risk, verification)
	gin.SetMode(gin.TestMode)
	engine := router.SetupRouter(handler.NewAuthHandler(auth, users), handler.NewUserHandler(users), handler.NewProfileHandler(profile), handler.NewCatalogHandler(serviceimpl.NewCatalogService(catalog)), handler.NewBuyerHandler(buyer), handler.NewSellerHandler(seller), middleware.New(auth, profile, seller, nil, "https://portal.test"), 10000)
	f := &fixture{t: t, db: db, engine: engine, risk: risk, users: users}
	admin, e := users.Create(context.Background(), service.CreateUserCommand{Username: "admin", Email: "admin@example.test", Password: "test-password", Role: entity.UserRoleAdmin})
	if e != nil {
		t.Fatal(e)
	}
	f.exec("INSERT INTO buyers(buyer_id,auth_subject,batch_id,source_record_id) VALUES ('buyer',?,'olist_vn_sim','buyer')", fmt.Sprintf("local:%d", admin.ID))
	f.exec("INSERT INTO sellers(seller_id,auth_subject,display_name,batch_id,source_record_id) VALUES ('seller',?,'Seller','olist_vn_sim','seller')", fmt.Sprintf("local:%d", admin.ID))
	login := f.request("POST", "/api/auth/login", `{"username":"admin","password":"test-password"}`, 200)
	f.token = login["access_token"].(string)
	return f
}
func (f *fixture) exec(sql string, args ...any) {
	f.t.Helper()
	if e := f.db.Exec(sql, args...).Error; e != nil {
		f.t.Fatal(e)
	}
}
func (f *fixture) request(method, path, body string, status int) map[string]any {
	f.t.Helper()
	w := httptest.NewRecorder()
	r := httptest.NewRequest(method, path, strings.NewReader(body))
	r.Header.Set("Content-Type", "application/json")
	r.Header.Set("Origin", "https://portal.test")
	if f.token != "" {
		r.Header.Set("Authorization", "Bearer "+f.token)
	}
	f.engine.ServeHTTP(w, r)
	if w.Code != status {
		f.t.Fatalf("%s %s got %d want %d: %s", method, path, w.Code, status, w.Body.String())
	}
	if status == 204 {
		return nil
	}
	var envelope map[string]any
	if e := json.Unmarshal(w.Body.Bytes(), &envelope); e != nil {
		f.t.Fatal(e)
	}
	if status >= 400 {
		if envelope["is_success"] != false {
			f.t.Fatal(envelope)
		}
		return envelope
	}
	if path == "/health" {
		return envelope
	}
	if envelope["is_success"] != true {
		f.t.Fatal(envelope)
	}
	if m, ok := envelope["data"].(map[string]any); ok {
		return m
	}
	return envelope
}
func TestMarketplacePurchaseAndReviewLifecycle(t *testing.T) {
	f := newFixture(t)
	f.request("GET", "/health", "", 200)
	f.request("GET", "/api/profiles", "", 200)
	f.request("GET", "/api/auth/me", "", 200)
	shop := f.request("POST", "/api/seller/shops", `{"display_name":"Test shop"}`, 201)["shop_id"].(string)
	base := "/api/seller/shops/" + shop
	f.request("GET", "/api/seller/shops", "", 200)
	f.request("GET", base+"/dashboard", "", 200)
	f.request("GET", "/api/catalog/categories", "", 200)
	f.request("GET", "/api/catalog/shops", "", 200)
	f.request("GET", "/api/catalog/shops/"+shop, "", 200)
	listing := f.request("POST", base+"/listings", `{"product_name":"Book","current_price":"10.50","stock_quantity":10}`, 201)
	id := listing["listing_id"].(string)
	if listing["currency"] != "VND" {
		t.Fatal("default listing currency", listing)
	}
	f.request("PATCH", base+"/listings/"+id, `{"current_price":"12.00","stock_quantity":8}`, 200)
	f.request("GET", base+"/listings", "", 200)
	f.request("GET", "/api/catalog/listings?shop_id="+shop+"&category_code=missing", "", 200)
	addr := f.request("POST", "/api/buyer/addresses", `{"label":"Home","address_raw":" 12 Main Street ","region":"City","country_code":"VN","is_default":true}`, 201)["saved_address_id"].(string)
	f.request("PUT", "/api/buyer/addresses/"+addr, `{"label":"Home","address_raw":"12 Main Street","region":"City","country_code":"VN","is_default":true}`, 200)
	f.request("GET", "/api/buyer/addresses", "", 200)
	payload := fmt.Sprintf(`{"phone":"0900000001","saved_address_id":%q,"payment_method":"cod","items":[{"listing_id":%q,"quantity":2}]}`, addr, id)
	checkout := f.request("POST", "/api/buyer/checkouts", payload, 201)
	if checkout["total_amount"] != "24.00" {
		t.Fatal("server price calculation", checkout)
	}
	order := checkout["orders"].([]any)[0].(map[string]any)["order_id"].(string)
	path := base + "/orders/" + order
	if f.risk.calls != 1 {
		t.Fatal("checkout must score exactly once")
	}
	f.request("GET", "/api/buyer/orders", "", 200)
	f.request("GET", "/api/buyer/orders/"+order, "", 200)
	f.request("GET", base+"/orders?review_status=pending&fulfillment_status=unfulfilled&risk_level=unassessed&batch_id=olist_vn_sim", "", 200)
	f.request("GET", path, "", 200)
	f.request("POST", path+"/score?force=true", "", 200)
	f.request("POST", path+"/explain", "", 409)
	if network := f.request("GET", path+"/network", "", 200); network["order_id"] != order {
		t.Fatal("network lost the authorized order scope", network)
	}
	f.risk.err = service.ErrRiskUnavailable
	f.request("GET", path+"/network", "", 503)
	f.risk.err = nil
	verification := f.request("GET", path+"/verification", "", 200)
	if verification["phone_masked"] != "***0001" || verification["verification"] != nil {
		t.Fatal("unassessed checkout must expose only a masked phone", verification)
	}
	f.request("POST", path+"/verification/respond", `{"verification_id":"missing","key":"3"}`, 400)
	f.request("POST", path+"/verification/respond", `{"verification_id":"missing","key":"1"}`, 404)
	f.request("GET", base+"/orders/missing/network", "", 404)
	f.request("GET", base+"/orders/missing/verification", "", 404)
	f.exec(`INSERT INTO rule_versions(rule_version,config,config_sha256,activated_at,approved_by,batch_id) VALUES ('v','{}',repeat('a',64),now(),'test','olist_vn_sim')`)
	f.exec(`INSERT INTO risk_assessments(assessment_id,order_id,assessed_at,input_cutoff_at,rule_version,score,risk_level,feature_snapshot,reasons,batch_id) VALUES ('assessment',?,now(),now(),'v',50,'medium','{}','[]','olist_vn_sim')`, order)
	f.request("POST", path+"/score", "", 200)
	verification = f.request("GET", path+"/verification", "", 200)
	verificationID := verification["verification"].(map[string]any)["verification_id"].(string)
	answer := fmt.Sprintf(`{"verification_id":%q,"key":"1"}`, verificationID)
	for range 2 {
		confirmed := f.request("POST", path+"/verification/respond", answer, 200)
		if confirmed["status"] != "confirmed" {
			t.Fatal("phone answer was not saved", confirmed)
		}
		for _, private := range []string{"phone_e164", "phone_key", "handled_by_seller_id"} {
			if _, present := confirmed[private]; present {
				t.Fatal("verification leaked private field", private)
			}
		}
	}
	var confirmations int64
	f.db.Table("order_events").Where("order_id=? AND event_type='confirmed'", order).Count(&confirmations)
	if confirmations != 1 {
		t.Fatal("retry duplicated phone confirmation", confirmations)
	}
	f.exec(`UPDATE risk_assessments SET score=80,risk_level='high' WHERE assessment_id='assessment'`)
	f.request("POST", path+"/explain", "", 200)
	f.request("GET", base+"/dashboard", "", 200)
	f.request("POST", path+"/decision", `{"assessment_id":"assessment","action":"hold","reason":"Review evidence"}`, 200)
	f.request("POST", path+"/fulfillment", `{"status":"packing"}`, 409)
	f.request("POST", path+"/decision", `{"assessment_id":"assessment","action":"release","reason":"Verified"}`, 200)
	for _, s := range []string{"packing", "shipped", "delivered", "returned"} {
		f.request("POST", path+"/fulfillment", fmt.Sprintf(`{"status":%q}`, s), 200)
	}
	f.request("GET", "/api/buyer/orders/"+order, "", 200)
	// Cancellation records the event; stock remains reserved under the current demo contract.
	canceled := f.request("POST", "/api/buyer/checkouts", payload, 201)["orders"].([]any)[0].(map[string]any)["order_id"].(string)
	f.request("POST", "/api/buyer/orders/"+canceled+"/cancel", `{"reason":"Changed mind"}`, 200)
	var stock int
	f.db.Table("listings").Select("stock_quantity").Where("listing_id=?", id).Scan(&stock)
	if stock != 4 {
		t.Fatal("cancel changed stock unexpectedly", stock)
	}
	f.request("DELETE", "/api/buyer/addresses/"+addr, "", 204)
}
func TestAdminLifecycleAndAuthentication(t *testing.T) {
	f := newFixture(t)
	u := f.request("POST", "/api/users", `{"username":"member","email":"member@example.test","password":"test-password"}`, 201)
	id := fmt.Sprint(u["id"])
	path := "/api/users/" + id
	if _, ok := u["password"]; ok {
		t.Fatal("password leaked")
	}
	f.request("GET", "/api/users?sort=-id&username[like]=member&page=1&limit=1", "", 200)
	found := f.request("GET", "/api/users?search=member", "", 200)
	if found["total"] != float64(1) {
		t.Fatal("user search did not match email/username", found)
	}
	f.request("GET", path, "", 200)
	for _, body := range []string{`{"role":"invalid"}`, `{"status":"invalid"}`, `{"username":"admin"}`, `{"email":"admin@example.test"}`} {
		expected := 400
		if strings.Contains(body, "admin") {
			expected = 409
		}
		f.request("PUT", path, body, expected)
	}
	f.request("PUT", path, `{"username":"changed","email":"changed@example.test","role":"USER","status":"INACTIVE"}`, 200)
	f.request("POST", "/api/auth/login", `{"username":"changed","password":"test-password"}`, 403)
	f.request("PUT", path, `{"status":"ACTIVE"}`, 200)
	login := f.request("POST", "/api/auth/login", `{"username":"changed","password":"test-password"}`, 200)
	refresh := login["refresh_token"].(string)
	f.token = login["access_token"].(string)
	f.request("GET", "/api/users", "", 403)
	f.request("GET", "/api/buyer/addresses", "", 403)
	f.request("GET", "/api/seller/shops", "", 403)
	f.request("GET", "/api/profiles", "", 200)
	refreshed := f.request("POST", "/api/auth/refresh", fmt.Sprintf(`{"refresh_token":%q}`, refresh), 200)
	if refreshed["refresh_token"] == refresh {
		t.Fatal("refresh must rotate")
	}
	f.request("POST", "/api/auth/refresh", fmt.Sprintf(`{"refresh_token":%q}`, refresh), 401)
	f.request("POST", "/api/auth/logout", "", 200)
	f.request("POST", "/api/auth/refresh", fmt.Sprintf(`{"refresh_token":%q}`, refreshed["refresh_token"]), 401)
	f.token = ""
	f.request("GET", "/api/auth/me", "", 401)
	f.token = "invalid"
	f.request("GET", "/api/auth/me", "", 401)
	admin := f.request("POST", "/api/auth/login", `{"username":"admin","password":"test-password"}`, 200)
	f.token = admin["access_token"].(string)
	f.request("DELETE", path, "", 204)
	f.request("GET", path, "", 404)
	f.request("DELETE", path, "", 404)
	// Deleted usernames/emails stay reserved and cannot be used to take over an identity.
	f.request("POST", "/api/users", `{"username":"changed","email":"fresh@example.test","password":"test-password"}`, 409)
	f.request("POST", "/api/users", `{"username":"fresh","email":"changed@example.test","password":"test-password"}`, 409)
	for _, method := range []string{"GET", "PUT", "DELETE"} {
		f.request(method, "/api/users/not-number", `{}`, 400)
	}
	f.request("PUT", "/api/users/999999", `{}`, 404)
	f.request("POST", "/api/users", `{"username":"fresh","email":"fresh@example.test","password":"test-password","role":"bad"}`, 400)
	f.request("POST", "/api/auth/login", `{"username":"missing","password":"wrong"}`, 401)
	f.request("POST", "/api/auth/login", `{"username":"admin","password":"wrong"}`, 401)
	for _, path := range []string{"/api/auth/login", "/api/auth/refresh", "/api/users"} {
		f.request("POST", path, `{`, 400)
	}
	f.request("PUT", "/api/users/1", `{`, 400)
}
func TestValidationAndTenantBoundaries(t *testing.T) {
	f := newFixture(t)
	shop := f.request("POST", "/api/seller/shops", `{"display_name":"Shop"}`, 201)["shop_id"].(string)
	base := "/api/seller/shops/" + shop
	for _, body := range []string{`{`, `{}`, `{"display_name":" "}`} {
		f.request("POST", "/api/seller/shops", body, 400)
	}
	for _, body := range []string{`{`, `{}`, `{"product_name":" ","current_price":"1"}`, `{"product_name":"N","current_price":"bad"}`, `{"product_name":"N","current_price":"1","currency":"bad-code"}`, `{"product_name":"N","current_price":"1","stock_quantity":-1}`, `{"product_name":"N","current_price":"1","category_code":"missing"}`} {
		f.request("POST", base+"/listings", body, 400)
	}
	for _, body := range []string{`{`, `{}`, `{"current_price":"bad"}`, `{"stock_quantity":-1}`} {
		f.request("PATCH", base+"/listings/missing", body, 400)
	}
	f.request("PATCH", base+"/listings/missing", `{"stock_quantity":1}`, 404)
	for _, route := range []string{"/api/catalog/shops/missing", base + "/orders/missing", "/api/seller/shops/foreign/orders", "/api/buyer/orders/missing"} {
		f.request("GET", route, "", 404)
	}
	for _, route := range []string{"/api/buyer/addresses", "/api/buyer/checkouts", base + "/orders/missing/decision", base + "/orders/missing/fulfillment", "/api/buyer/orders/missing/cancel"} {
		f.request("POST", route, `{`, 400)
	}
	f.request("PUT", "/api/buyer/addresses/missing", `{`, 400)
	address := `{"label":"Home","address_raw":"Address","region":"City","country_code":"VN"}`
	f.request("PUT", "/api/buyer/addresses/missing", address, 404)
	f.request("DELETE", "/api/buyer/addresses/missing", "", 404)
	for _, body := range []string{`{"label":" ","address_raw":"a","region":"b","country_code":"VN"}`, `{"label":"Home","address_raw":"a","region":"b","country_code":"11"}`} {
		f.request("POST", "/api/buyer/addresses", body, 400)
	}
	for _, route := range []string{base + "/orders/missing/score", base + "/orders/missing/explain"} {
		f.request("POST", route, "", 404)
	}
	f.request("POST", base+"/orders/missing/score?force=bad", "", 400)
	f.request("POST", base+"/orders/missing/decision", `{"assessment_id":"a","action":"hold","reason":"why"}`, 404)
	f.request("POST", base+"/orders/missing/fulfillment", `{"status":"packing"}`, 404)
	f.request("POST", "/api/buyer/orders/missing/cancel", `{"reason":"Changed mind"}`, 404)
	// Suspended accounts may inspect history but cannot mutate it.
	f.exec("UPDATE buyers SET status='suspended'")
	f.request("GET", "/api/buyer/addresses", "", 200)
	f.request("POST", "/api/buyer/addresses", address, 403)
	f.exec("UPDATE sellers SET status='suspended'")
	f.request("GET", "/api/seller/shops", "", 200)
	f.request("POST", "/api/seller/shops", `{"display_name":"Blocked"}`, 403)
}

func TestCheckoutRejectsInvalidAndUnavailableLinesAtomically(t *testing.T) {
	f := newFixture(t)
	shop := f.request("POST", "/api/seller/shops", `{"display_name":"Shop"}`, 201)["shop_id"].(string)
	base := "/api/seller/shops/" + shop
	a := f.request("POST", "/api/buyer/addresses", `{"label":"Home","address_raw":"12 Main","region":"City","country_code":"VN"}`, 201)["saved_address_id"].(string)
	id := f.request("POST", base+"/listings", `{"product_name":"Book","current_price":"10","stock_quantity":5}`, 201)["listing_id"].(string)
	for _, tc := range []struct {
		method, items string
		status        int
	}{{"invalid", `[]`, 400}, {"cod", `[]`, 400}, {"cod", fmt.Sprintf(`[{"listing_id":%q,"quantity":0}]`, id), 400}, {"cod", fmt.Sprintf(`[{"listing_id":%q,"quantity":100}]`, id), 400}, {"cod", fmt.Sprintf(`[{"listing_id":%q,"quantity":1},{"listing_id":%q,"quantity":1}]`, id, id), 400}, {"cod", `[{"listing_id":"missing","quantity":1}]`, 404}, {"cod", fmt.Sprintf(`[{"listing_id":%q,"quantity":6}]`, id), 409}} {
		f.request("POST", "/api/buyer/checkouts", fmt.Sprintf(`{"phone":"0900000001","saved_address_id":%q,"payment_method":%q,"items":%s}`, a, tc.method, tc.items), tc.status)
	}
	payload := func(address string, ids ...string) string {
		items := []map[string]any{}
		for _, id := range ids {
			items = append(items, map[string]any{"listing_id": id, "quantity": 1})
		}
		b, _ := json.Marshal(items)
		return fmt.Sprintf(`{"phone":"0900000001","saved_address_id":%q,"payment_method":"cod","items":%s}`, address, b)
	}
	f.request("POST", "/api/buyer/checkouts", payload("missing", id), 404)
	f.exec("UPDATE listings SET current_price=NULL WHERE listing_id=?", id)
	f.request("POST", "/api/buyer/checkouts", payload(a, id), 400)
	f.exec("UPDATE listings SET current_price=10 WHERE listing_id=?", id)
	second := f.request("POST", base+"/listings", `{"product_name":"Other","current_price":"5","currency":"USD","stock_quantity":5}`, 201)["listing_id"].(string)
	f.request("POST", "/api/buyer/checkouts", payload(a, id, second), 400)
	var count int64
	f.db.Table("checkouts").Count(&count)
	if count != 0 {
		t.Fatal("invalid checkout wrote records")
	}
	var stock int
	f.db.Table("listings").Select("stock_quantity").Where("listing_id=?", id).Scan(&stock)
	if stock != 5 {
		t.Fatal("failed checkout did not roll back stock")
	}
	f.exec("UPDATE listings SET currency='VND' WHERE listing_id=?", second)
	// Multiple lines in one shop create one order; KBS outage must not undo the committed checkout.
	f.risk.err = service.ErrRiskUnavailable
	checkout := f.request("POST", "/api/buyer/checkouts", payload(a, id, second), 201)
	if len(checkout["orders"].([]any)) != 1 || checkout["total_amount"] != "15.00" {
		t.Fatal(checkout)
	}
	order := checkout["orders"].([]any)[0].(map[string]any)["order_id"].(string)
	f.request("POST", base+"/orders/"+order+"/score", "", 503)
	f.request("POST", "/api/buyer/orders/"+order+"/cancel", `{}`, 400)
	f.request("POST", "/api/buyer/orders/"+order+"/cancel", `{"reason":"changed"}`, 200)
	f.request("POST", "/api/buyer/orders/"+order+"/cancel", `{"reason":"changed"}`, 409)
}
func TestDatabaseFailuresDoNotBecomeSuccessfulEmptyResponses(t *testing.T) {
	f := newFixture(t)
	shop := f.request("POST", "/api/seller/shops", `{"display_name":"Shop"}`, 201)["shop_id"].(string)
	base := "/api/seller/shops/" + shop
	for _, tc := range []struct {
		table  string
		routes []string
	}{
		{"products", []string{"/api/catalog/categories", "/api/catalog/listings", base + "/listings"}},
		{"orders", []string{base + "/orders", base + "/orders/unknown", base + "/dashboard", "/api/buyer/orders", "/api/buyer/orders/unknown"}},
		{"saved_addresses", []string{"/api/buyer/addresses"}},
		{"shops", []string{"/api/catalog/shops", "/api/catalog/shops/unknown", "/api/seller/shops"}},
	} {
		t.Run(tc.table, func(t *testing.T) {
			f.exec("ALTER TABLE " + tc.table + " RENAME TO unavailable_table")
			defer f.exec("ALTER TABLE unavailable_table RENAME TO " + tc.table)
			for _, path := range tc.routes {
				f.request("GET", path, "", 500)
			}
		})
	}
}

func TestRepositoryReadRestoreAndSafeUpdates(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	repo := persistence.NewUserRepository(f.db)
	admin, err := repo.FindByEmail(ctx, "admin@example.test")
	if err != nil {
		t.Fatal(err)
	}
	base := persistence.NewBaseRepository[entity.User](f.db, map[string]bool{"id": true}, "user")
	if exists, e := base.Exists(ctx, admin.ID); e != nil || !exists {
		t.Fatal(exists, e)
	}
	if exists, e := base.Exists(ctx, 999999); e != nil || exists {
		t.Fatal(exists, e)
	}
	if rows, total, e := base.List(ctx, 0, 10, query.NewQueryOptions()); e != nil || total != 1 || len(rows) != 1 {
		t.Fatal(total, e)
	}
	admin.Email = "updated@example.test"
	if e := base.Update(ctx, admin); e != nil {
		t.Fatal(e)
	}
	if e := repo.Delete(ctx, admin.ID); e != nil {
		t.Fatal(e)
	}
	if e := repo.Restore(ctx, admin.ID); e != nil {
		t.Fatal(e)
	}
	if _, e := repo.FindByID(ctx, admin.ID); e != nil {
		t.Fatal(e)
	}
	if _, e := repo.FindByEmail(ctx, "absent@example.test"); e == nil {
		t.Fatal("missing email accepted")
	}
	canceled, cancel := context.WithCancel(ctx)
	cancel()
	if e := repo.Restore(canceled, admin.ID); e == nil {
		t.Fatal("canceled restore accepted")
	}
	if _, e := repo.FindByEmail(canceled, admin.Email); e == nil {
		t.Fatal("canceled email lookup accepted")
	}
	if _, _, e := repo.ListWithQuery(canceled, 0, 10, query.NewQueryOptions()); e == nil {
		t.Fatal("canceled list accepted")
	}
}
