package serviceimpl

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"slices"
	"strings"
	"time"

	"github.com/tiloneeee/ai-fraud-detection/backend/internal/domain/entity"
	"github.com/tiloneeee/ai-fraud-detection/backend/internal/domain/repository"
	"github.com/tiloneeee/ai-fraud-detection/backend/internal/usecase/service"
	apperror "github.com/tiloneeee/ai-fraud-detection/backend/pkg/error"
)

type riskDataService struct{ repo repository.RiskDataRepository }

func NewRiskDataService(repo repository.RiskDataRepository) service.RiskDataService {
	return &riskDataService{repo: repo}
}

func strictRiskData(raw json.RawMessage, target any) error {
	d := json.NewDecoder(bytes.NewReader(raw))
	d.DisallowUnknownFields()
	if err := d.Decode(target); err != nil {
		return apperror.ErrValidation.WithMessage("Dữ liệu bằng chứng không hợp lệ")
	}
	if err := d.Decode(new(any)); err != io.EOF {
		return apperror.ErrValidation.WithMessage("Chỉ chấp nhận một bản ghi JSON")
	}
	return nil
}

func riskText(value string) bool { return strings.TrimSpace(value) != "" && len(value) <= 500 }

func (s *riskDataService) Record(ctx context.Context, actorID uint, input service.RiskEvidenceInput) error {
	if actorID == 0 {
		return apperror.ErrUnauthorized
	}
	now := time.Now().UTC()
	if !riskText(input.RecordID) || len(input.RecordID) > 128 || !riskText(input.SourceRef) || input.OccurredAt.IsZero() || input.OccurredAt.After(now) {
		return apperror.ErrValidation.WithMessage("Cần mã bản ghi, nguồn kiểm chứng và thời điểm hợp lệ")
	}
	r := &entity.RiskEvidence{RecordID: input.RecordID, Kind: input.Kind, SourceRef: strings.TrimSpace(input.SourceRef),
		OccurredAt: input.OccurredAt, AvailableAt: now, RecordedBy: fmt.Sprintf("admin:%d", actorID)}
	switch input.Kind {
	case "identity_link":
		var data struct {
			BuyerID        string `json:"buyer_id"`
			RelatedBuyerID string `json:"related_buyer_id"`
			EdgeType       string `json:"edge_type"`
			Status         string `json:"status"`
		}
		if err := strictRiskData(input.Data, &data); err != nil {
			return err
		}
		if !riskText(data.BuyerID) || !riskText(data.RelatedBuyerID) || data.BuyerID == data.RelatedBuyerID ||
			!slices.Contains([]string{"verified_document", "verified_payment", "verified_account"}, data.EdgeType) ||
			!slices.Contains([]string{"verified", "revoked"}, data.Status) {
			return apperror.ErrValidation.WithMessage("Liên kết cần hai người mua khác nhau và nguồn xác minh được hỗ trợ")
		}
		r.BuyerID, r.RelatedBuyerID, r.EdgeType, r.Status = data.BuyerID, data.RelatedBuyerID, data.EdgeType, data.Status
	case "enforcement":
		var data struct {
			BuyerID   string `json:"buyer_id"`
			Action    string `json:"action"`
			AbuseCode string `json:"abuse_code"`
		}
		if err := strictRiskData(input.Data, &data); err != nil {
			return err
		}
		if !riskText(data.BuyerID) || !slices.Contains([]string{"restricted", "reinstated"}, data.Action) ||
			!slices.Contains([]string{"delivery", "cancellation", "after_sale"}, data.AbuseCode) {
			return apperror.ErrValidation.WithMessage("Quyết định xử lý hoặc loại hành vi không hợp lệ")
		}
		r.BuyerID, r.Action, r.AbuseCode = data.BuyerID, data.Action, data.AbuseCode
	case "case_verdict":
		var data struct {
			OrderID            string `json:"order_id"`
			CaseID             string `json:"case_id"`
			CaseType           string `json:"case_type"`
			VerificationStatus string `json:"verification_status"`
			ResponsibleParty   string `json:"responsible_party"`
			OutcomeCode        string `json:"outcome_code"`
			Rationale          string `json:"rationale"`
		}
		if err := strictRiskData(input.Data, &data); err != nil {
			return err
		}
		outcomes := map[string]string{"delivery": "delivery_failure", "cancellation": "cancellation", "return": "return_abuse", "refund": "refund_abuse"}
		code, supported := outcomes[data.CaseType]
		if !riskText(data.OrderID) || !riskText(data.Rationale) || !supported || (data.OutcomeCode != code && data.OutcomeCode != "legitimate") ||
			!slices.Contains([]string{"unresolved", "insufficient_evidence", "verified"}, data.VerificationStatus) ||
			!slices.Contains([]string{"buyer", "seller", "carrier", "shared", "unknown"}, data.ResponsibleParty) ||
			(strings.HasSuffix(data.OutcomeCode, "_abuse") && (data.VerificationStatus != "verified" || data.ResponsibleParty != "buyer")) {
			return apperror.ErrValidation.WithMessage("Kết luận phải phù hợp loại hồ sơ; nhãn lạm dụng cần xác minh trách nhiệm người mua")
		}
		r.OrderID, r.CaseID, r.CaseType = data.OrderID, data.CaseID, data.CaseType
		r.VerificationStatus, r.ResponsibleParty, r.OutcomeCode, r.Rationale = data.VerificationStatus, data.ResponsibleParty, data.OutcomeCode, data.Rationale
	case "evidence_review":
		var data struct {
			EvidenceID    string `json:"evidence_id"`
			ContentBase64 string `json:"content_base64"`
			Verdict       string `json:"verdict"`
		}
		if err := strictRiskData(input.Data, &data); err != nil {
			return err
		}
		content, err := base64.StdEncoding.DecodeString(data.ContentBase64)
		if err != nil || len(content) == 0 || len(content) > 600_000 || !riskText(data.EvidenceID) ||
			!slices.Contains([]string{"contradiction_verified", "no_contradiction", "unresolved"}, data.Verdict) {
			return apperror.ErrValidation.WithMessage("Cần nội dung bằng chứng tối đa 600 KB và kết luận đối chiếu hợp lệ")
		}
		digest := sha256.Sum256(content)
		r.EvidenceID, r.ContentSHA256, r.Verdict = data.EvidenceID, hex.EncodeToString(digest[:]), data.Verdict
	default:
		return apperror.ErrValidation.WithMessage("Loại bằng chứng không được hỗ trợ")
	}
	return s.repo.Record(ctx, r)
}

func (s *riskDataService) ListPlatform(ctx context.Context, status string, offset, limit int) ([]entity.PlatformRiskObservation, int64, error) {
	if !slices.Contains([]string{"all", "fired", "unknown", "clear"}, status) || offset < 0 || limit < 1 || limit > 100 {
		return nil, 0, apperror.ErrValidation.WithMessage("Bộ lọc cảnh báo không hợp lệ")
	}
	return s.repo.ListPlatform(ctx, status, offset, limit)
}
