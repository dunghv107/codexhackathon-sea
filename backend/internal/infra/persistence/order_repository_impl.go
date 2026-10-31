package persistence

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"github.com/tiloneeee/ai-fraud-detection/backend/internal/domain/entity"
	"github.com/tiloneeee/ai-fraud-detection/backend/internal/domain/repository"
	"github.com/tiloneeee/ai-fraud-detection/backend/internal/usecase/service"
	apperror "github.com/tiloneeee/ai-fraud-detection/backend/pkg/error"
)

// latestAssessmentJoin picks one assessment per order: newest assessed_at, ties broken by id.
const latestAssessmentJoin = `
LEFT JOIN LATERAL (
    SELECT risk_level, score FROM risk_assessments
    WHERE order_id = o.order_id
    ORDER BY assessed_at DESC, assessment_id DESC LIMIT 1
) ra ON true`

const orderFrom = `
FROM orders o
JOIN checkouts c ON c.checkout_id = o.checkout_id
JOIN buyers bu ON bu.buyer_id = c.buyer_id AND bu.batch_id = c.batch_id
JOIN shops s ON s.shop_id = o.shop_id
JOIN data_batches b ON b.batch_id = o.batch_id` + latestAssessmentJoin

// Totals are summed in numeric (exact decimal) and read as text; payment shows "succeeded" if any line succeeded.
const orderSummarySelect = `
SELECT o.batch_id, b.is_synthetic, b.source_system, o.order_id, o.checkout_id,
       c.buyer_id, COALESCE(NULLIF(bu.display_name, ''), c.buyer_id) AS buyer_display_name,
       c.placed_at, o.shop_id, s.display_name AS shop_name,
       c.payment_method,
       COALESCE((SELECT p.status FROM payments p WHERE p.checkout_id = c.checkout_id
                 ORDER BY (p.status = 'succeeded') DESC, p.payment_id LIMIT 1), 'unknown') AS payment_status,
       it.item_count, it.order_total, c.currency, o.review_status, o.fulfillment_status,
       ra.risk_level, ra.score` + orderFrom + `
LEFT JOIN LATERAL (
    SELECT COALESCE(sum(quantity), 0) AS item_count,
           COALESCE(sum(quantity * unit_price + freight_amount), 0)::numeric(18,2)::text AS order_total
    FROM order_items WHERE order_id = o.order_id
) it ON true`

type orderRepositoryImpl struct {
	db *gorm.DB
}

// NewOrderRepository creates a new order repository
func NewOrderRepository(db *gorm.DB) repository.OrderRepository {
	return &orderRepositoryImpl{db: db}
}

func (r *orderRepositoryImpl) CreateCheckout(ctx context.Context, checkout *entity.Checkout, addr *entity.CheckoutAddress, payment *entity.Payment, contact *entity.CheckoutContact) error {
	db := dbFrom(ctx, r.db)
	var buyerCount int64
	if err := db.Model(&entity.Buyer{}).Where("buyer_id = ? AND batch_id = ?", checkout.BuyerID, checkout.BatchID).Count(&buyerCount).Error; err != nil {
		return err
	}
	if buyerCount != 1 {
		return service.ErrRiskBatch
	}
	for _, row := range []any{checkout, addr, payment} {
		if err := db.Create(row).Error; err != nil {
			return wrapCreateError(err, "checkout")
		}
	}
	// Contact values must never be interpolated into SQL logs.
	if err := db.Session(&gorm.Session{Logger: logger.Default.LogMode(logger.Silent)}).Create(contact).Error; err != nil {
		return wrapCreateError(err, "contact")
	}
	if err := db.Session(&gorm.Session{Logger: logger.Default.LogMode(logger.Silent)}).Exec(`INSERT INTO checkout_identity_snapshots(checkout_id,available_at,batch_id,phone_key) VALUES (?,?,?,?)`,
		checkout.CheckoutID, checkout.PlacedAt, checkout.BatchID, contact.PhoneKey).Error; err != nil {
		return wrapCreateError(err, "snapshot định danh")
	}
	return nil
}

func (r *orderRepositoryImpl) CreateOrder(ctx context.Context, order *entity.Order, items []entity.OrderItem) error {
	db := dbFrom(ctx, r.db)
	if err := db.Create(order).Error; err != nil {
		return wrapCreateError(err, "đơn hàng")
	}
	if err := db.Create(&items).Error; err != nil {
		return wrapCreateError(err, "dòng hàng")
	}
	return nil
}

func (r *orderRepositoryImpl) CreateEvent(ctx context.Context, event *entity.OrderEvent) error {
	if err := dbFrom(ctx, r.db).Create(event).Error; err != nil {
		return wrapCreateError(err, "sự kiện đơn")
	}
	return nil
}

func (r *orderRepositoryImpl) CreateDecision(ctx context.Context, decision *entity.ReviewDecision) error {
	if err := dbFrom(ctx, r.db).Create(decision).Error; err != nil {
		return wrapCreateError(err, "quyết định duyệt")
	}
	return nil
}

func (r *orderRepositoryImpl) UpdateOrder(ctx context.Context, order *entity.Order, columns ...string) error {
	if err := dbFrom(ctx, r.db).Model(order).Select(columns).Updates(order).Error; err != nil {
		return wrapUpdateError(err, "đơn hàng")
	}
	return nil
}

// orderScope builds the WHERE clause; it refuses to run unscoped so one tenant never reads another's orders.
func orderScope(f entity.OrderFilter) (string, []any, error) {
	var conds []string
	var args []any
	add := func(cond string, v any) { conds, args = append(conds, cond), append(args, v) }

	if f.BuyerID == "" && f.ShopID == "" {
		return "", nil, apperror.ErrInternalServerError.WithMessage("Truy vấn đơn hàng thiếu phạm vi")
	}
	if f.BuyerID != "" {
		add("c.buyer_id = ?", f.BuyerID)
	}
	if f.ShopID != "" {
		add("o.shop_id = ?", f.ShopID)
	}
	if f.OrderID != "" {
		add("o.order_id = ?", f.OrderID)
	}
	if f.BatchID != "" {
		add("o.batch_id = ?", f.BatchID)
	}
	if f.DisplayStatus != "" {
		add("(CASE WHEN o.fulfillment_status='unfulfilled' THEN CASE WHEN o.review_status='released' THEN 'confirmed' ELSE 'confirming' END ELSE o.fulfillment_status END) = ?", f.DisplayStatus)
	}
	if f.ReviewStatus != "" {
		add("o.review_status = ?", f.ReviewStatus)
	}
	if f.FulfillmentStatus != "" {
		add("o.fulfillment_status = ?", f.FulfillmentStatus)
	}
	if f.RiskLevel == entity.RiskUnassessed {
		conds = append(conds, "ra.risk_level IS NULL")
	} else if f.RiskLevel != "" {
		add("ra.risk_level = ?", f.RiskLevel)
	}
	return "\nWHERE " + strings.Join(conds, " AND "), args, nil
}

func (r *orderRepositoryImpl) ListOrders(ctx context.Context, f entity.OrderFilter, offset, limit int) ([]entity.OrderSummary, int64, error) {
	where, args, err := orderScope(f)
	if err != nil {
		return nil, 0, err
	}
	var total int64
	if err := dbFrom(ctx, r.db).Raw("SELECT count(*)"+orderFrom+where, args...).Scan(&total).Error; err != nil {
		return nil, 0, wrapListError(err, "đơn hàng")
	}
	var orders []entity.OrderSummary
	if err := dbFrom(ctx, r.db).Raw(orderSummarySelect+where+"\nORDER BY c.placed_at DESC, o.order_id LIMIT ? OFFSET ?", append(args, limit, offset)...).
		Scan(&orders).Error; err != nil {
		return nil, 0, wrapListError(err, "đơn hàng")
	}
	return orders, total, nil
}

func (r *orderRepositoryImpl) LockOrder(ctx context.Context, f entity.OrderFilter) (*entity.Order, error) {
	where, args, err := orderScope(entity.OrderFilter{BuyerID: f.BuyerID, ShopID: f.ShopID, OrderID: f.OrderID})
	if err != nil {
		return nil, err
	}
	var order entity.Order
	res := dbFrom(ctx, r.db).Raw(`
SELECT o.* FROM orders o JOIN checkouts c ON c.checkout_id = o.checkout_id`+where+"\nFOR UPDATE OF o", args...).Scan(&order)
	if res.Error != nil {
		return nil, wrapFindError(res.Error, "đơn hàng")
	}
	if res.RowsAffected == 0 {
		return nil, apperror.ErrNotFound.WithMessage("Không tìm thấy đơn hàng")
	}
	return &order, nil
}

func (r *orderRepositoryImpl) ListItems(ctx context.Context, orderID string) ([]entity.OrderItemView, error) {
	var items []entity.OrderItemView
	if err := dbFrom(ctx, r.db).Raw(`
SELECT i.item_id, i.listing_id, COALESCE(i.product_name, p.display_name) AS product_name, p.category_name_en, i.quantity,
       i.unit_price::text AS unit_price, i.freight_amount::text AS freight_amount
FROM order_items i
JOIN listings l ON l.listing_id = i.listing_id
JOIN products p ON p.product_id = l.product_id
WHERE i.order_id = ? ORDER BY i.item_id`, orderID).Scan(&items).Error; err != nil {
		return nil, wrapListError(err, "dòng hàng")
	}
	return items, nil
}

// FindCheckoutAddress returns an empty snapshot when the checkout has none (some Olist rows).
func (r *orderRepositoryImpl) FindCheckoutAddress(ctx context.Context, checkoutID string) (*entity.CheckoutAddress, error) {
	var addr entity.CheckoutAddress
	if err := dbFrom(ctx, r.db).Where("checkout_id = ?", checkoutID).Limit(1).Find(&addr).Error; err != nil {
		return nil, wrapFindError(err, "địa chỉ giao hàng")
	}
	return &addr, nil
}

func (r *orderRepositoryImpl) ListEvents(ctx context.Context, orderID string) ([]entity.OrderEvent, error) {
	var events []entity.OrderEvent
	if err := dbFrom(ctx, r.db).Where("order_id = ?", orderID).Order("occurred_at, event_id").Find(&events).Error; err != nil {
		return nil, wrapListError(err, "sự kiện đơn")
	}
	return events, nil
}

func (r *orderRepositoryImpl) FindLatestAssessment(ctx context.Context, orderID string) (*entity.RiskAssessment, error) {
	var row struct {
		BatchID         string
		RuleConfig      string
		AssessmentID    string
		OrderID         string
		AssessedAt      time.Time
		InputCutoffAt   *time.Time
		RuleVersion     string
		Score           int
		RiskLevel       string
		FeatureSnapshot string
		Reasons         string
	}
	res := dbFrom(ctx, r.db).Raw(`
SELECT a.batch_id, v.config::text AS rule_config, a.assessment_id, a.order_id, a.assessed_at, a.input_cutoff_at, a.rule_version, a.score, a.risk_level,
       a.feature_snapshot::text AS feature_snapshot, a.reasons::text AS reasons
FROM risk_assessments a JOIN rule_versions v ON v.rule_version=a.rule_version AND v.batch_id=a.batch_id
WHERE a.order_id = ?
ORDER BY assessed_at DESC, assessment_id DESC LIMIT 1`, orderID).Scan(&row)
	if res.Error != nil {
		return nil, wrapFindError(res.Error, "đánh giá rủi ro")
	}
	if res.RowsAffected == 0 {
		return nil, nil
	}
	return &entity.RiskAssessment{
		BatchID:         row.BatchID,
		RuleConfig:      json.RawMessage(row.RuleConfig),
		AssessmentID:    row.AssessmentID,
		OrderID:         row.OrderID,
		AssessedAt:      row.AssessedAt,
		InputCutoffAt:   row.InputCutoffAt,
		RuleVersion:     row.RuleVersion,
		Score:           row.Score,
		RiskLevel:       row.RiskLevel,
		FeatureSnapshot: json.RawMessage(row.FeatureSnapshot),
		Reasons:         json.RawMessage(row.Reasons),
	}, nil
}

func (r *orderRepositoryImpl) ListDecisions(ctx context.Context, orderID string) ([]entity.ReviewDecision, error) {
	var decisions []entity.ReviewDecision
	if err := dbFrom(ctx, r.db).Where("order_id = ?", orderID).Order("created_at, decision_id").Find(&decisions).Error; err != nil {
		return nil, wrapListError(err, "quyết định duyệt")
	}
	return decisions, nil
}

func (r *orderRepositoryImpl) PaymentReady(ctx context.Context, checkoutID string) (bool, error) {
	var ready bool
	if err := dbFrom(ctx, r.db).Raw(`
SELECT c.payment_method = 'cod'
    OR EXISTS (SELECT 1 FROM payments p WHERE p.checkout_id = c.checkout_id AND p.status = 'succeeded')
FROM checkouts c WHERE c.checkout_id = ?`, checkoutID).Scan(&ready).Error; err != nil {
		return false, wrapFindError(err, "thanh toán")
	}
	return ready, nil
}

func (r *orderRepositoryImpl) CountByStatus(ctx context.Context, shopID string) ([]entity.StatusCount, error) {
	var counts []entity.StatusCount
	if err := dbFrom(ctx, r.db).Raw(`
SELECT o.review_status, o.fulfillment_status, ra.risk_level, count(*) AS count
FROM orders o`+latestAssessmentJoin+`
WHERE o.shop_id = ?
GROUP BY 1, 2, 3`, shopID).Scan(&counts).Error; err != nil {
		return nil, wrapListError(err, "thống kê đơn")
	}
	return counts, nil
}

func (r *orderRepositoryImpl) ListCases(ctx context.Context, orderID string) (json.RawMessage, error) {
	var result string
	err := dbFrom(ctx, r.db).Raw(`
SELECT COALESCE(jsonb_agg(jsonb_build_object(
    'case_id', c.case_id, 'case_type', c.case_type, 'opened_at', c.opened_at,
    'available_at', c.available_at,
    'messages', (SELECT COALESCE(jsonb_agg(jsonb_build_object('speaker_role',m.speaker_role,'body',m.body,'occurred_at',m.occurred_at,'available_at',m.available_at) ORDER BY m.occurred_at,m.message_id),'[]'::jsonb) FROM case_messages m WHERE m.case_id=c.case_id),
    'evidence', (SELECT COALESCE(jsonb_agg(jsonb_build_object('media_type',e.media_type,'content_ref',e.content_ref,'submitted_by',e.submitted_by,'available_at',e.available_at) ORDER BY e.available_at,e.evidence_id),'[]'::jsonb) FROM case_evidence e WHERE e.case_id=c.case_id),
    'resolutions', (SELECT COALESCE(jsonb_agg(jsonb_build_object('verification_status',r.verification_status,'responsible_party',r.responsible_party,'rationale',r.rationale,'decided_at',r.decided_at,'available_at',r.available_at) ORDER BY r.decided_at,r.resolution_id),'[]'::jsonb) FROM case_resolutions r WHERE r.case_id=c.case_id)
) ORDER BY c.opened_at,c.case_id),'[]'::jsonb)::text
FROM cases c WHERE c.order_id=?`, orderID).Scan(&result).Error
	if err != nil {
		return nil, wrapListError(err, "hồ sơ tranh chấp")
	}
	return json.RawMessage(result), nil
}
