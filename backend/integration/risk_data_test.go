package integration_test

import (
	"encoding/json"
	"fmt"
	"testing"
	"time"
)

func TestRiskEvidenceAdminAPIAndPurchaseSnapshots(t *testing.T) {
	f := newFixture(t)
	shop := f.request("POST", "/api/seller/shops", `{"display_name":"Risk shop"}`, 201)["shop_id"].(string)
	f.exec(`INSERT INTO products(product_id,category_code,category_name_en,batch_id,source_record_id) VALUES ('book','books','Books','olist_vn_sim','book')`)
	listing := f.request("POST", "/api/seller/shops/"+shop+"/listings", `{"product_name":"Book","category_code":"books","current_price":"12"}`, 201)["listing_id"].(string)
	address := f.request("POST", "/api/buyer/addresses", `{"label":"Home","address_raw":"12 Main Street","region":"City","country_code":"VN","is_default":true}`, 201)["saved_address_id"].(string)
	checkout := f.request("POST", "/api/buyer/checkouts", fmt.Sprintf(`{"phone":"0900000001","saved_address_id":%q,"payment_method":"cod","items":[{"listing_id":%q,"quantity":1}]}`, address, listing), 201)
	order := checkout["orders"].([]any)[0].(map[string]any)["order_id"].(string)
	var snapshots int64
	if err := f.db.Table("order_item_risk_snapshots s").Joins("JOIN order_items i USING(item_id,batch_id)").Where("i.order_id=? AND s.category_code='books'", order).Count(&snapshots).Error; err != nil || snapshots != 1 {
		t.Fatal("checkout category snapshot", snapshots, err)
	}
	if err := f.db.Table("order_risk_tracking").Where("order_id=?", order).Count(&snapshots).Error; err != nil || snapshots != 1 {
		t.Fatal("checkout state coverage", snapshots, err)
	}
	f.exec(`INSERT INTO buyers(buyer_id,batch_id,source_record_id) VALUES ('peer','olist_vn_sim','peer')`)
	post := func(id, kind string, data map[string]any, status int) string {
		t.Helper()
		body, err := json.Marshal(map[string]any{"record_id": id, "kind": kind, "source_ref": "ticket:1", "occurred_at": time.Now().UTC(), "data": data})
		if err != nil {
			t.Fatal(err)
		}
		f.request("POST", "/api/admin/risk/evidence", string(body), status)
		return string(body)
	}
	identity := map[string]any{"buyer_id": "peer", "related_buyer_id": "buyer", "edge_type": "verified_document", "status": "verified"}
	body := post("link", "identity_link", identity, 201)
	f.request("POST", "/api/admin/risk/evidence", body, 409)
	identity["buyer_id"] = "missing"
	post("missing", "identity_link", identity, 404)
	post("restriction", "enforcement", map[string]any{"buyer_id": "peer", "action": "restricted", "abuse_code": "delivery"}, 201)
	post("refund", "case_verdict", map[string]any{"order_id": order, "case_type": "refund", "verification_status": "verified", "responsible_party": "buyer", "outcome_code": "refund_abuse", "rationale": "Confirmed contradiction"}, 201)
	f.exec(`INSERT INTO case_evidence VALUES ('proof','refund:case','buyer','image','private-content',now(),'olist_vn_sim','proof')`)
	post("review", "evidence_review", map[string]any{"evidence_id": "proof", "content_base64": "aGVsbG8=", "verdict": "contradiction_verified"}, 201)
	var stored struct {
		BuyerID, RelatedBuyerID, RecordedBy string
		AvailableAt, OccurredAt             time.Time
	}
	if err := f.db.Table("risk_identity_assertions").Where("record_id='link'").Scan(&stored).Error; err != nil {
		t.Fatal(err)
	}
	if stored.BuyerID != "buyer" || stored.RelatedBuyerID != "peer" || stored.RecordedBy == "" || stored.AvailableAt.Before(stored.OccurredAt) {
		t.Fatal(stored)
	}
	f.request("POST", "/api/admin/risk/evidence", `{"available_at":"2020-01-01T00:00:00Z"}`, 400)
	f.request("GET", "/api/admin/risk/observations?status=all", "", 200)
	f.request("GET", "/api/admin/risk/observations?page=0", "", 400)
	f.request("GET", "/api/admin/risk/observations?status=invalid", "", 400)
	f.exec(`INSERT INTO rule_versions(rule_version,config,config_sha256,activated_at,approved_by,batch_id) VALUES ('v','{}',repeat('a',64),now(),'test','olist_vn_sim')`)
	for _, item := range []struct{ id, at, status string }{{"old", "2026-01-01", "fired"}, {"new", "2026-01-02", "unknown"}} {
		f.exec(`INSERT INTO risk_assessments(assessment_id,order_id,assessed_at,input_cutoff_at,rule_version,score,risk_level,feature_snapshot,reasons,batch_id) VALUES (?,?,?,now(),'v',0,'low','{"adaptive":{"baseline_version":"demo-v2"}}','[]','olist_vn_sim')`, item.id, order, item.at)
		f.exec(`INSERT INTO risk_platform_observations VALUES (?,'olist_vn_sim',?::jsonb)`, item.id, fmt.Sprintf(`[{"rule_id":"SFD-A12","status":%q}]`, item.status))
	}
	observations := f.request("GET", "/api/admin/risk/observations?status=all", "", 200)
	items := observations["items"].([]any)
	if len(items) != 1 || items[0].(map[string]any)["assessment_id"] != "new" || items[0].(map[string]any)["baseline_version"] != "demo-v2" {
		t.Fatal(observations)
	}
	if result := f.request("GET", "/api/admin/risk/observations?status=fired", "", 200); result["total"].(float64) != 0 {
		t.Fatal(result)
	}
	f.exec("UPDATE users SET role='USER' WHERE username='admin'")
	f.request("GET", "/api/admin/risk/observations", "", 403)
	f.request("POST", "/api/admin/risk/evidence", body, 403)
	f.token = ""
	f.request("GET", "/api/admin/risk/observations", "", 401)
}
