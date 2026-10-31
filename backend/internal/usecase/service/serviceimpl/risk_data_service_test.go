package serviceimpl

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/tiloneeee/ai-fraud-detection/backend/internal/domain/entity"
	"github.com/tiloneeee/ai-fraud-detection/backend/internal/usecase/service"
)

type riskEvidenceRepo struct{ record *entity.RiskEvidence }

func (r *riskEvidenceRepo) Record(_ context.Context, e *entity.RiskEvidence) error {
	r.record = e
	return nil
}
func (*riskEvidenceRepo) ListPlatform(context.Context, string, int, int) ([]entity.PlatformRiskObservation, int64, error) {
	return nil, 0, nil
}

func TestRiskEvidenceValidatesProvenanceAndHashesContent(t *testing.T) {
	repo := &riskEvidenceRepo{}
	s := NewRiskDataService(repo)
	input := service.RiskEvidenceInput{RecordID: "review:1", Kind: "evidence_review", SourceRef: "review-ticket:1", OccurredAt: time.Now().Add(-time.Hour), Data: json.RawMessage(`{"evidence_id":"e1","content_base64":"aGVsbG8=","verdict":"no_contradiction"}`)}
	before := time.Now()
	if err := s.Record(context.Background(), 7, input); err != nil {
		t.Fatal(err)
	}
	if repo.record.ContentSHA256 != "2cf24dba5fb0a30e26e83b2ac5b9e29e1b161e5c1fa7425e73043362938b9824" || repo.record.RecordedBy != "admin:7" || repo.record.AvailableAt.Before(before) {
		t.Fatal(repo.record)
	}
	for name, alter := range map[string]func(*service.RiskEvidenceInput){
		"future":    func(v *service.RiskEvidenceInput) { v.OccurredAt = time.Now().Add(time.Hour) },
		"no source": func(v *service.RiskEvidenceInput) { v.SourceRef = " " },
		"bad kind":  func(v *service.RiskEvidenceInput) { v.Kind = "phone" },
		"malformed": func(v *service.RiskEvidenceInput) { v.Data = json.RawMessage(`{`) },
		"forged hash": func(v *service.RiskEvidenceInput) {
			v.Data = json.RawMessage(`{"evidence_id":"e1","content_sha256":"trusted","verdict":"no_contradiction"}`)
		},
		"multiple json": func(v *service.RiskEvidenceInput) { v.Data = append(v.Data, []byte(` {}`)...) },
		"bad base64": func(v *service.RiskEvidenceInput) {
			v.Data = json.RawMessage(`{"evidence_id":"e1","content_base64":"!","verdict":"no_contradiction"}`)
		},
		"oversize": func(v *service.RiskEvidenceInput) {
			v.Data = json.RawMessage(`{"evidence_id":"e1","content_base64":"` + strings.Repeat("YWFh", 200001) + `","verdict":"unresolved"}`)
		},
		"weak edge": func(v *service.RiskEvidenceInput) {
			v.Kind = "identity_link"
			v.Data = json.RawMessage(`{"buyer_id":"a","related_buyer_id":"b","edge_type":"shared_phone","status":"verified"}`)
		},
		"same buyer": func(v *service.RiskEvidenceInput) {
			v.Kind = "identity_link"
			v.Data = json.RawMessage(`{"buyer_id":"a","related_buyer_id":"a","edge_type":"verified_document","status":"verified"}`)
		},
		"unverified abuse": func(v *service.RiskEvidenceInput) {
			v.Kind = "case_verdict"
			v.Data = json.RawMessage(`{"order_id":"o","case_type":"refund","verification_status":"unresolved","responsible_party":"buyer","outcome_code":"refund_abuse","rationale":"ticket"}`)
		},
		"wrong case outcome": func(v *service.RiskEvidenceInput) {
			v.Kind = "case_verdict"
			v.Data = json.RawMessage(`{"order_id":"o","case_type":"return","verification_status":"verified","responsible_party":"buyer","outcome_code":"refund_abuse","rationale":"ticket"}`)
		},
	} {
		t.Run(name, func(t *testing.T) {
			candidate := input
			alter(&candidate)
			repo.record = nil
			if s.Record(context.Background(), 7, candidate) == nil || repo.record != nil {
				t.Fatal("invalid evidence persisted")
			}
		})
	}
	if s.Record(context.Background(), 0, input) == nil {
		t.Fatal("missing actor accepted")
	}
}
