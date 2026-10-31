package integration_test

import (
	"fmt"
	"os"
	"strings"
	"testing"
)

func TestPurchasedNameSurvivesListingEdits(t *testing.T) {
	f := newFixture(t)
	shop := f.request("POST", "/api/seller/shops", `{"display_name":"Snapshot shop"}`, 201)["shop_id"].(string)
	base := "/api/seller/shops/" + shop
	listing := f.request("POST", base+"/listings", `{"product_name":"Original lamp","current_price":"10","stock_quantity":10}`, 201)["listing_id"].(string)
	address := f.request("POST", "/api/buyer/addresses", `{"label":"Home","address_raw":"12 Main Street","region":"City","country_code":"VN"}`, 201)["saved_address_id"].(string)
	buy := func() string {
		payload := fmt.Sprintf(`{"phone":"0900000001","saved_address_id":%q,"payment_method":"cod","items":[{"listing_id":%q,"quantity":1}]}`, address, listing)
		result := f.request("POST", "/api/buyer/checkouts", payload, 201)
		return result["orders"].([]any)[0].(map[string]any)["order_id"].(string)
	}
	first := buy()
	f.request("PATCH", base+"/listings/"+listing, `{"product_name":"Renamed lamp"}`, 200)
	second := buy()
	f.request("PATCH", base+"/listings/"+listing, `{"product_name":"Replacement chair","current_price":"20"}`, 200)
	for _, tc := range []struct{ order, name string }{{first, "Original lamp"}, {second, "Renamed lamp"}} {
		for _, route := range []string{"/api/buyer/orders/", base + "/orders/"} {
			detail := f.request("GET", route+tc.order, "", 200)
			item := detail["items"].([]any)[0].(map[string]any)
			if item["product_name"] != tc.name || item["unit_price"] != "10.00" {
				t.Fatalf("purchased item changed: name=%v price=%v", item["product_name"], item["unit_price"])
			}
		}
	}
	if current := f.request("GET", "/api/catalog/listings/"+listing, "", 200); current["product_name"] != "Replacement chair" {
		t.Fatal("catalog did not keep the current listing title")
	}

	// Model an existing pre-migration row. Reattaching must backfill missing names
	// without overwriting already captured names, then later edits stay isolated.
	f.exec("UPDATE order_items SET product_name=NULL WHERE order_id=?", first)
	ddl, err := os.ReadFile("../migrations/000008_order_item_name.up.sql")
	if err != nil {
		t.Fatal(err)
	}
	var schema string
	if err := f.db.Raw("SELECT current_schema()").Scan(&schema).Error; err != nil {
		t.Fatal(err)
	}
	f.exec(strings.ReplaceAll(string(ddl), "sfd", schema))
	f.request("PATCH", base+"/listings/"+listing, `{"product_name":"Later title"}`, 200)
	for _, tc := range []struct{ order, name string }{{first, "Replacement chair"}, {second, "Renamed lamp"}} {
		detail := f.request("GET", "/api/buyer/orders/"+tc.order, "", 200)
		if detail["items"].([]any)[0].(map[string]any)["product_name"] != tc.name {
			t.Fatal("migration lost the backfill or overwrote an existing snapshot")
		}
	}
	// Canonical seed imports after migration omit this nullable column. Their
	// fallback is the immutable product name, never the current listing title.
	f.exec("UPDATE order_items SET product_name=NULL WHERE order_id=?", first)
	detail := f.request("GET", "/api/buyer/orders/"+first, "", 200)
	if detail["items"].([]any)[0].(map[string]any)["product_name"] != "Original lamp" {
		t.Fatal("legacy imported item used the mutable listing title")
	}
	f.exec("UPDATE products SET display_name=NULL WHERE product_id=(SELECT product_id FROM listings WHERE listing_id=?)", listing)
	detail = f.request("GET", "/api/buyer/orders/"+first, "", 200)
	if detail["items"].([]any)[0].(map[string]any)["product_name"] != nil {
		t.Fatal("unnamed historical product acquired a fabricated title")
	}

}
