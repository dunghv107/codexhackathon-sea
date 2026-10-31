package persistence_test

import (
	"context"
	"github.com/tiloneeee/ai-fraud-detection/backend/internal/infra/persistence"
	"github.com/tiloneeee/ai-fraud-detection/backend/internal/usecase/service/serviceimpl"
	"gorm.io/gorm"
	"sync"
	"testing"
	"time"
)

// Called in both the canonical and migrated schema lanes, after checkout has committed.
func checkVerification(t *testing.T, db *gorm.DB, orderID, checkoutID string) {
	t.Helper()
	ctx := context.Background()
	exec := func(sql string, args ...any) {
		t.Helper()
		if err := db.Exec(sql, args...).Error; err != nil {
			t.Fatal(err)
		}
	}
	repo := persistence.NewVerificationRepository(db)
	orders := persistence.NewOrderRepository(db)
	tx := persistence.NewTransactor(db)
	svc := serviceimpl.NewVerificationService(repo, orders, tx, true)
	disabled := serviceimpl.NewVerificationService(repo, orders, tx, false)
	exec(`INSERT INTO risk_assessments(assessment_id,order_id,assessed_at,input_cutoff_at,rule_version,score,risk_level,feature_snapshot,reasons,batch_id) VALUES ('medium',?,now(),now(),'v',30,'medium','{}','[]','olist_vn_sim')`, orderID)
	if err := disabled.EnsureForOrder(ctx, "s", orderID); err != nil {
		t.Fatal(err)
	}
	if row, _ := repo.Find(ctx, orderID); row != nil {
		t.Fatal("disabled created verification")
	}
	if err := svc.EnsureForOrder(ctx, "s", orderID); err != nil {
		t.Fatal(err)
	}
	row, err := repo.Find(ctx, orderID)
	if err != nil || row == nil {
		t.Fatalf("missing verification: %v", err)
	}
	if _, err := svc.Respond(ctx, "other", orderID, "seller", row.VerificationID, "1"); err == nil {
		t.Fatal("foreign shop confirmed")
	}
	if _, err := disabled.Respond(ctx, "s", orderID, "seller", row.VerificationID, "1"); err == nil {
		t.Fatal("disabled confirmed")
	}
	exec(`INSERT INTO risk_assessments(assessment_id,order_id,assessed_at,input_cutoff_at,rule_version,score,risk_level,feature_snapshot,reasons,batch_id)
 SELECT 'new-medium',order_id,assessed_at+interval '1 second',input_cutoff_at,rule_version,score,risk_level,feature_snapshot,reasons,batch_id FROM risk_assessments WHERE assessment_id='medium'`)
	if _, err := svc.Respond(ctx, "s", orderID, "seller", row.VerificationID, "1"); err == nil {
		t.Fatal("stale assessment confirmed")
	}
	if err := svc.EnsureForOrder(ctx, "s", orderID); err != nil {
		t.Fatal(err)
	}
	for _, change := range []struct{ sql, reset string }{
		{`UPDATE orders SET review_status='released' WHERE order_id=?`, `UPDATE orders SET review_status='pending' WHERE order_id=?`},
		{`UPDATE orders SET fulfillment_status='canceled' WHERE order_id=?`, `UPDATE orders SET fulfillment_status='unfulfilled' WHERE order_id=?`},
		{`UPDATE risk_assessments SET risk_level='high' WHERE order_id=?`, `UPDATE risk_assessments SET risk_level='medium' WHERE order_id=?`},
	} {
		exec(change.sql, orderID)
		if _, err := svc.Respond(ctx, "s", orderID, "seller", row.VerificationID, "1"); err == nil {
			t.Fatal("ineligible order confirmed")
		}
		exec(change.reset, orderID)
	}
	exec(`UPDATE checkouts SET payment_method='credit_card' WHERE checkout_id=?`, checkoutID)
	if _, err := svc.Respond(ctx, "s", orderID, "seller", row.VerificationID, "1"); err == nil {
		t.Fatal("prepaid confirmed")
	}
	exec(`UPDATE checkouts SET payment_method='cod' WHERE checkout_id=?`, checkoutID)

	// A concurrent release holds the same order lock; the answer must recheck after it commits.
	locked := db.Begin()
	if locked.Error != nil {
		t.Fatal(locked.Error)
	}
	if err := locked.Exec(`UPDATE orders SET review_status='released' WHERE order_id=?`, orderID).Error; err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { _, err := svc.Respond(ctx, "s", orderID, "seller", row.VerificationID, "1"); done <- err }()
	select {
	case <-done:
		locked.Rollback()
		t.Fatal("answer bypassed order lock")
	case <-time.After(50 * time.Millisecond):
	}
	if err := locked.Commit().Error; err != nil {
		t.Fatal(err)
	}
	if err := <-done; err == nil {
		t.Fatal("answer ignored committed release")
	}
	exec(`UPDATE orders SET review_status='pending' WHERE order_id=?`, orderID)

	var wg sync.WaitGroup
	errors := make(chan error, 4)
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := svc.Respond(ctx, "s", orderID, "seller", row.VerificationID, "1")
			errors <- err
		}()
	}
	wg.Wait()
	close(errors)
	for err := range errors {
		if err != nil {
			t.Fatal(err)
		}
	}
	if _, err := svc.Respond(ctx, "s", orderID, "seller", row.VerificationID, "2"); err == nil {
		t.Fatal("contradictory terminal answer accepted")
	}
	var count int64
	if err := db.Table("order_events").Where("order_id=? AND event_type='confirmed'", orderID).Count(&count).Error; err != nil || count != 1 {
		t.Fatalf("duplicate/missing confirmation: %d %v", count, err)
	}
	var state struct {
		ReviewStatus      string
		FulfillmentStatus string
	}
	if err := db.Table("orders").Where("order_id=?", orderID).Take(&state).Error; err != nil || state.ReviewStatus != "pending" || state.FulfillmentStatus != "unfulfilled" {
		t.Fatal("verification changed order status")
	}
	latest, err := orders.FindLatestAssessment(ctx, orderID)
	if err != nil || latest.Score != 30 || latest.AssessmentID != "new-medium" {
		t.Fatal("verification changed assessment")
	}
	var event struct {
		OccurredAt, AvailableAt time.Time
		ActorType               string
	}
	if err := db.Table("order_events").Where("order_id=? AND event_type='confirmed'", orderID).Take(&event).Error; err != nil || !event.OccurredAt.Equal(event.AvailableAt) || event.ActorType != "platform" {
		t.Fatal("confirmation provenance incorrect")
	}

	for _, key := range []string{"2", "0"} {
		id := "answer-" + key
		exec(`INSERT INTO checkouts SELECT ?,buyer_id,placed_at,payment_method,currency,total_amount,batch_id,? FROM checkouts WHERE checkout_id=?`, id, id, checkoutID)
		exec(`INSERT INTO orders(order_id,checkout_id,shop_id,batch_id,source_record_id) VALUES (?,?,'s','olist_vn_sim',?)`, id, id, id)
		exec(`INSERT INTO risk_assessments(assessment_id,order_id,assessed_at,input_cutoff_at,rule_version,score,risk_level,feature_snapshot,reasons,batch_id) VALUES (?,?,now(),now(),'v',30,'medium','{}','[]','olist_vn_sim')`, id, id)
		if err := svc.EnsureForOrder(ctx, "s", id); err != nil {
			t.Fatal(err)
		}
		v, _ := repo.Find(ctx, id)
		if _, err := svc.Respond(ctx, "s", id, "seller", v.VerificationID, key); err != nil {
			t.Fatal(err)
		}
		if err := db.Table("order_events").Where("order_id=?", id).Count(&count).Error; err != nil || count != 0 {
			t.Fatal("negative answer created event")
		}
	}
}
