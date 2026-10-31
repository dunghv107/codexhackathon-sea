package entity

import (
	"encoding/json"
	"time"
)

// RiskEvidence is normalized, append-only evidence entered by a platform reviewer.
type RiskEvidence struct {
	RecordID, Kind, SourceRef, RecordedBy                                                   string
	OccurredAt, AvailableAt                                                                 time.Time
	BuyerID, RelatedBuyerID, EdgeType, Status                                               string
	Action, AbuseCode                                                                       string
	OrderID, CaseID, CaseType, VerificationStatus, ResponsibleParty, OutcomeCode, Rationale string
	EvidenceID, ContentSHA256, Verdict                                                      string
}

type PlatformRiskObservation struct {
	AssessmentID    string          `json:"assessment_id"`
	OrderID         string          `json:"order_id"`
	ShopID          string          `json:"shop_id"`
	ShopName        string          `json:"shop_name"`
	RuleVersion     string          `json:"rule_version"`
	BaselineVersion string          `json:"baseline_version"`
	AssessedAt      time.Time       `json:"assessed_at"`
	InputCutoffAt   time.Time       `json:"input_cutoff_at"`
	Signals         json.RawMessage `json:"signals"`
}
