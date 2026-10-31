package persistence

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/tiloneeee/ai-fraud-detection/backend/internal/domain/entity"
	"github.com/tiloneeee/ai-fraud-detection/backend/internal/domain/repository"
	apperror "github.com/tiloneeee/ai-fraud-detection/backend/pkg/error"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

type riskDataRepository struct{ db *gorm.DB }

func NewRiskDataRepository(db *gorm.DB) repository.RiskDataRepository {
	return &riskDataRepository{db: db}
}

func (r *riskDataRepository) Record(ctx context.Context, e *entity.RiskEvidence) error {
	// Reviewer source references and uploaded content must not enter SQL logs.
	return r.db.WithContext(ctx).Session(&gorm.Session{Logger: logger.Default.LogMode(logger.Silent)}).Transaction(func(tx *gorm.DB) error {
		var result *gorm.DB
		switch e.Kind {
		case "identity_link":
			result = tx.Exec(`INSERT INTO risk_identity_assertions(record_id,batch_id,buyer_id,related_buyer_id,edge_type,status,occurred_at,available_at,source_ref,recorded_by)
VALUES (?,?,least(?::text,?::text),greatest(?::text,?::text),?,?,?,?,?,?)`, e.RecordID, entity.DemoBatchID, e.BuyerID, e.RelatedBuyerID, e.BuyerID, e.RelatedBuyerID, e.EdgeType, e.Status, e.OccurredAt, e.AvailableAt, e.SourceRef, e.RecordedBy)
		case "enforcement":
			result = tx.Exec(`INSERT INTO risk_enforcement_events(record_id,batch_id,buyer_id,action,abuse_code,occurred_at,available_at,source_ref,recorded_by)
VALUES (?,?,?,?,?,?,?,?,?)`, e.RecordID, entity.DemoBatchID, e.BuyerID, e.Action, e.AbuseCode, e.OccurredAt, e.AvailableAt, e.SourceRef, e.RecordedBy)
		case "case_verdict":
			var count int64
			if err := tx.Table("orders o").Joins("JOIN checkouts c ON c.checkout_id=o.checkout_id AND c.batch_id=o.batch_id").Where("o.order_id=? AND o.batch_id=? AND c.placed_at<=?", e.OrderID, entity.DemoBatchID, e.OccurredAt).Count(&count).Error; err != nil {
				return err
			}
			if count != 1 {
				return apperror.ErrNotFound.WithMessage("Không tìm thấy đơn phù hợp thời điểm kết luận")
			}
			caseID := e.CaseID
			if caseID == "" {
				caseID = e.RecordID + ":case"
				if err := tx.Exec(`INSERT INTO cases(case_id,order_id,case_type,opened_at,available_at,batch_id,source_record_id) VALUES (?,?,?,?,?,?,?)`, caseID, e.OrderID, e.CaseType, e.OccurredAt, e.AvailableAt, entity.DemoBatchID, caseID).Error; err != nil {
					return wrapCreateError(err, "hồ sơ")
				}
			} else {
				if err := tx.Table("cases").Where("case_id=? AND order_id=? AND batch_id=? AND case_type=? AND opened_at<=?", caseID, e.OrderID, entity.DemoBatchID, e.CaseType, e.OccurredAt).Count(&count).Error; err != nil {
					return err
				}
				if count != 1 {
					return apperror.ErrNotFound.WithMessage("Hồ sơ không thuộc đơn hoặc loại yêu cầu")
				}
			}
			if err := tx.Exec(`INSERT INTO case_resolutions(resolution_id,case_id,verification_status,responsible_party,rationale,decided_by,decided_at,available_at,batch_id,source_record_id)
VALUES (?,?,?,?,?,?,?,?,?,?)`, e.RecordID, caseID, e.VerificationStatus, e.ResponsibleParty, e.Rationale, e.RecordedBy, e.OccurredAt, e.AvailableAt, entity.DemoBatchID, e.RecordID).Error; err != nil {
				return wrapCreateError(err, "kết luận")
			}
			result = tx.Exec(`INSERT INTO risk_case_outcomes(resolution_id,batch_id,outcome_code,available_at,source_ref,recorded_by) VALUES (?,?,?,?,?,?)`, e.RecordID, entity.DemoBatchID, e.OutcomeCode, e.AvailableAt, e.SourceRef, e.RecordedBy)
		case "evidence_review":
			result = tx.Exec(`INSERT INTO risk_evidence_reviews(record_id,batch_id,evidence_id,content_sha256,verdict,occurred_at,available_at,source_ref,recorded_by) VALUES (?,?,?,?,?,?,?,?,?)`, e.RecordID, entity.DemoBatchID, e.EvidenceID, e.ContentSHA256, e.Verdict, e.OccurredAt, e.AvailableAt, e.SourceRef, e.RecordedBy)
		default:
			return apperror.ErrValidation.WithMessage("Loại bằng chứng không được hỗ trợ")
		}
		if result.Error != nil {
			var pgErr *pgconn.PgError
			if errors.As(result.Error, &pgErr) && pgErr.Code == "23503" {
				return apperror.ErrNotFound.WithMessage("Không tìm thấy người mua hoặc bằng chứng trong bộ dữ liệu hiện tại")
			}
			return wrapCreateError(result.Error, "bằng chứng rủi ro")
		}
		return nil
	})
}

const platformRiskFrom = ` FROM risk_platform_observations p
JOIN risk_assessments a ON a.assessment_id=p.assessment_id AND a.batch_id=p.batch_id
JOIN orders o ON o.order_id=a.order_id AND o.batch_id=a.batch_id
JOIN shops s ON s.shop_id=o.shop_id AND s.batch_id=o.batch_id
CROSS JOIN LATERAL (SELECT jsonb_agg(signal) AS signals FROM jsonb_array_elements(p.signals) signal
 WHERE (?='all' OR signal->>'status'=?)) filtered
WHERE p.batch_id=? AND filtered.signals IS NOT NULL
AND a.assessment_id=(SELECT latest.assessment_id FROM risk_assessments latest
 WHERE latest.order_id=o.order_id AND latest.batch_id=o.batch_id ORDER BY latest.assessed_at DESC,latest.assessment_id DESC LIMIT 1)`

func (r *riskDataRepository) ListPlatform(ctx context.Context, status string, offset, limit int) ([]entity.PlatformRiskObservation, int64, error) {
	db := r.db.WithContext(ctx)
	args := []any{status, status, entity.DemoBatchID}
	var total int64
	if err := db.Raw("SELECT count(*)"+platformRiskFrom, args...).Scan(&total).Error; err != nil {
		return nil, 0, wrapListError(err, "cảnh báo sàn")
	}
	var rows []struct {
		entity.PlatformRiskObservation
		SignalJSON string
	}
	if err := db.Raw(`SELECT a.assessment_id,a.order_id,o.shop_id,s.display_name AS shop_name,a.rule_version,a.feature_snapshot->'adaptive'->>'baseline_version' AS baseline_version,a.assessed_at,a.input_cutoff_at,filtered.signals::text AS signal_json`+platformRiskFrom+` ORDER BY a.assessed_at DESC,a.assessment_id DESC LIMIT ? OFFSET ?`, append(args, limit, offset)...).Scan(&rows).Error; err != nil {
		return nil, 0, wrapListError(err, "cảnh báo sàn")
	}
	items := make([]entity.PlatformRiskObservation, 0, len(rows))
	for _, row := range rows {
		row.Signals = json.RawMessage(row.SignalJSON)
		items = append(items, row.PlatformRiskObservation)
	}
	return items, total, nil
}
