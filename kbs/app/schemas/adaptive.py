from datetime import datetime
from decimal import Decimal
from typing import Literal

from pydantic import BaseModel, Field

from app.rules.adaptive_config import AdaptiveRuleId, FeatureName

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


class AdaptiveContext(BaseModel):
    target: HistoricalOrder
    history: list[HistoricalOrder]


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
