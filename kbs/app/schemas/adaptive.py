from datetime import datetime
from decimal import Decimal
from typing import Literal

from pydantic import BaseModel, Field

from app.rules.adaptive_config import AdaptiveRuleId, EdgeType, FeatureName

SignalStatus = Literal["fired", "clear", "unknown"]


class HistoricalOrder(BaseModel):
    order_id: str
    checkout_id: str
    buyer_id: str
    shop_id: str
    placed_at: datetime
    currency: str
    amount: Decimal
    quantities: dict[str, int]
    phone_key: str | None = None
    address_key: str | None = None
    terminal_at: datetime | None = None
    outcome: Literal["delivery_succeeded", "delivery_failed"] | None = None
    outcome_available_at: datetime | None = None
    responsible_party: str | None = None
    resolution_verified: bool = False
    resolution_available_at: datetime | None = None
    payment_method: str = "cod"
    categories: set[str] | None = None
    account_created_at: datetime | None = None
    tracking_from: datetime | None = None
    cod_closed_at: datetime | None = None
    confirmed_at: datetime | None = None
    cancellation_at: datetime | None = None
    cancellation_party: str | None = None
    cancellation_verified: bool = False
    cancellation_known_at: datetime | None = None
    after_sale_present: bool = False
    after_sale_verified: bool = False
    after_sale_party: str | None = None
    after_sale_code: str | None = None
    after_sale_known_at: datetime | None = None
    seller_case: bool = False
    seller_case_known_at: datetime | None = None


class IdentityLink(BaseModel):
    related_buyer_id: str
    edge_type: EdgeType
    occurred_at: datetime
    available_at: datetime
    source_ref: str


class Enforcement(BaseModel):
    buyer_id: str
    abuse_code: Literal["delivery", "cancellation", "after_sale"]
    action: Literal["restricted", "reinstated"]
    occurred_at: datetime
    available_at: datetime
    record_id: str


class EvidenceReuse(BaseModel):
    evidence_id: str
    independent_cases: int
    verdict: Literal["contradiction_verified", "no_contradiction", "unresolved"]
    available_at: datetime


class AdaptiveContext(BaseModel):
    target: HistoricalOrder
    history: list[HistoricalOrder]
    checkout_shops: set[str] = Field(default_factory=set)
    identity_links: list[IdentityLink] = Field(default_factory=list)
    enforcements: list[Enforcement] = Field(default_factory=list)
    evidence_reviews: list[EvidenceReuse] = Field(default_factory=list)
    evidence_count: int = 0


class BaselineSnapshot(BaseModel):
    version: str
    cohort: str
    sample_count: int
    quantile: Decimal | None = None
    threshold: Decimal | None = None
    window_start: datetime
    window_end: datetime
    available_at: datetime | None = None
    selection_policy: str
    source_ref: str
    missing_reason: str | None = None


class AdaptiveEvidence(BaseModel):
    feature: str
    label: str
    observed: Decimal | None = None
    comparison_threshold: Decimal | None = None
    observed_sample_count: int | None = None
    baseline: BaselineSnapshot | None = None
    missing_reason: str | None = None


class ShadowSignal(BaseModel):
    rule_id: AdaptiveRuleId
    mode: Literal["shadow"] = "shadow"
    status: SignalStatus
    description: str
    action: str
    evidence_group: str
    contribution: Literal[0] = 0
    evidence: list[AdaptiveEvidence] = Field(default_factory=list)
    missing_reason: str | None = None
    audience: Literal["seller", "platform"] = "seller"


class Coverage(BaseModel):
    status: Literal["complete", "insufficient_data"]
    required_features: list[FeatureName]
    known_features: list[FeatureName]
    missing_features: dict[FeatureName, str]
    blocking: Literal[False] = False


class AdaptiveAssessment(BaseModel):
    mode: Literal["shadow"] = "shadow"
    calibration: Literal["demo_not_calibrated"] = "demo_not_calibrated"
    baseline_version: str
    policy_version: str
    input_cutoff_at: datetime
    signals: list[ShadowSignal]
    coverage: Coverage
    evidence_groups: dict[str, list[AdaptiveRuleId]]
