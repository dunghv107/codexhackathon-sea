package integration_test

import (
	"fmt"
	"os"
	"strings"
	"testing"
)

func TestDemoShopNamesPreserveIdentityAndCustomNames(t *testing.T) {
	f := newFixture(t)
	add := func(n int, name string) string {
		id := fmt.Sprintf("sim:%032x", n)
		f.exec("INSERT INTO shops(shop_id,display_name,owner_seller_id,batch_id,source_record_id) VALUES (?,?,'seller','olist_vn_sim',?)", id, name, id)
		return id
	}
	first := add(1, fmt.Sprintf("Demo shop %032x", 1))
	second := add(2, fmt.Sprintf("Demo shop %032x", 2))
	custom := add(3, "Tiệm Mây Xanh")
	reserved := add(4, "Cửa hàng demo 12")
	ddl, err := os.ReadFile("../migrations/000010_demo_shop_names.up.sql")
	if err != nil {
		t.Fatal(err)
	}
	var schema string
	if err := f.db.Raw("SELECT current_schema()").Scan(&schema).Error; err != nil {
		t.Fatal(err)
	}
	apply := func() { f.exec(strings.ReplaceAll(string(ddl), "sfd", schema)) }
	apply()
	apply()
	third := add(5, fmt.Sprintf("Demo shop %032x", 5))
	apply()
	expected := map[string]string{
		first: "Cửa hàng demo 13", second: "Cửa hàng demo 14", third: "Cửa hàng demo 15",
		custom: "Tiệm Mây Xanh", reserved: "Cửa hàng demo 12",
	}
	for id, name := range expected {
		card := f.request("GET", "/api/catalog/shops/"+id, "", 200)
		if card["display_name"] != name || card["shop_id"] != id {
			t.Fatalf("unexpected catalog identity/name: %+v", card)
		}
	}
	rows := f.request("GET", "/api/seller/shops", "", 200)["data"].([]any)
	if len(rows) != len(expected) {
		t.Fatal("shop count changed", rows)
	}
	for _, row := range rows {
		shop := row.(map[string]any)
		if shop["display_name"] != expected[shop["shop_id"].(string)] {
			t.Fatal("seller and catalog names disagree", shop)
		}
	}
	var preserved int64
	if err := f.db.Table("shops").Where("shop_id=source_record_id AND batch_id='olist_vn_sim' AND owner_seller_id='seller'").Count(&preserved).Error; err != nil || preserved != int64(len(expected)) {
		t.Fatal("migration changed shop ownership or provenance", preserved, err)
	}
}
