from typing import Literal, Self

from pydantic import BaseModel, ConfigDict, Field, model_validator

AdaptiveRuleId = Literal[
    "SFD-A01",
    "SFD-A02",
    "SFD-A03",
    "SFD-A04",
    "SFD-A05",
    "SFD-A06",
    "SFD-A07",
    "SFD-A08",
    "SFD-A09",
    "SFD-A10",
    "SFD-A11",
    "SFD-A12",
    "SFD-A13",
    "SFD-A14",
    "SFD-A15",
    "SFD-A16",
    "SFD-A17",
    "SFD-A18",
]
EXTENDED_RULES = frozenset(
    {"SFD-A02", "SFD-A03", "SFD-A05", "SFD-A07", "SFD-A10", "SFD-A12", "SFD-A13", "SFD-A14", "SFD-A15", "SFD-A16"}
)
EdgeType = Literal["verified_document", "verified_payment", "verified_account"]
LegacyRuleId = Literal["R1", "R2", "R3", "R4", "R5", "R6", "R7", "R8", "R9", "R10"]
RuleId = LegacyRuleId | AdaptiveRuleId
FeatureName = Literal[
    "buyer_failure_rate",
    "quantity_per_listing",
    "buyer_order_amount",
    "shop_order_amount",
    "buyer_phone_history",
    "buyer_cancellation_rate",
    "buyer_outcome_change",
    "cod_exposure",
    "new_categories",
    "after_sale_rate",
    "evidence_reuse",
]


class Dependency(BaseModel):
    model_config = ConfigDict(extra="forbid")

    mode: Literal["any", "all"]
    rules: list[RuleId] = Field(min_length=1)

    @model_validator(mode="after")
    def unique_rules(self) -> Self:
        if len(self.rules) != len(set(self.rules)):
            raise ValueError("Duplicate dependency")
        return self


class BaselineParameters(BaseModel):
    model_config = ConfigDict(extra="forbid", allow_inf_nan=False)

    lookback_days: int = Field(ge=1, le=730)
    min_samples: int = Field(ge=3)
    max_age_days: int = Field(ge=1)
    quantile: float = Field(gt=0, lt=1)
    window_hours: float = Field(gt=0, le=168)
    min_resolved: int = Field(ge=1)
    alpha: float = Field(gt=0)
    minimum_effect: float = Field(ge=0)
    quantity_cohorts: list[Literal["buyer_listing", "shop_listing"]]
    required_features: list[FeatureName]

    @model_validator(mode="after")
    def validate_support(self) -> Self:
        if self.max_age_days > self.lookback_days:
            raise ValueError("Baseline freshness must fit its lookback")
        if not self.quantity_cohorts or len(set(self.quantity_cohorts)) != len(self.quantity_cohorts):
            raise ValueError("Quantity cohorts must be nonempty and unique")
        if not self.required_features or len(set(self.required_features)) != len(self.required_features):
            raise ValueError("Required decision features must be nonempty and unique")
        return self


class ExtendedParameters(BaseModel):
    model_config = ConfigDict(extra="forbid")

    recent_days: int = Field(ge=1)
    maturity_days: int = Field(ge=0)
    min_link_types: int = Field(ge=2, le=3)
    min_link_sources: int = Field(ge=2)
    min_bad_peers: int = Field(ge=1)
    min_synchronized_peers: int = Field(ge=1)
    min_distinct_checkouts: int = Field(ge=2)
    allowed_edge_types: list[EdgeType] = Field(min_length=2)
    max_link_age_days: int = Field(ge=1)
    exposure_policy: Literal["until_success_cancel_or_return_v1"]

    @model_validator(mode="after")
    def unique_types(self) -> Self:
        if len(set(self.allowed_edge_types)) != len(self.allowed_edge_types) or self.min_link_types > len(
            self.allowed_edge_types
        ):
            raise ValueError("Identity policy requires enough distinct allowed evidence types")
        return self


class AdaptiveRuleDefinition(BaseModel):
    model_config = ConfigDict(extra="forbid")

    rule_id: AdaptiveRuleId
    description: str
    enabled: bool
    mode: Literal["shadow"]
    max_points: Literal[0]
    action: str = Field(min_length=1)
    evidence_group: Literal["delivery_history", "burst", "basket", "data_quality", "after_sale", "platform"]
    authority: Literal["team_demo_assumption_not_calibrated"]
    params: BaselineParameters
    dependencies: Dependency | None = None
    policy: ExtendedParameters | None = None
    audience: Literal["seller", "platform"] = "seller"

    @model_validator(mode="after")
    def combined_signals(self) -> Self:
        if self.rule_id in EXTENDED_RULES and (
            self.policy is None or self.policy.recent_days + self.policy.maturity_days >= self.params.lookback_days
        ):
            raise ValueError("Extended rules require a policy with separate recent and reference windows")
        if (self.rule_id in {"SFD-A12", "SFD-A13", "SFD-A16"}) != (self.audience == "platform"):
            raise ValueError("Identity enforcement and seller monitoring are platform-only")
        if self.rule_id in {"SFD-A17", "SFD-A18"} and self.dependencies is not None:
            raise ValueError("Coverage and attribution cannot depend on anomaly signals")
        if self.rule_id in {"SFD-A04", "SFD-A11"}:
            allowed = {"SFD-A05", "SFD-A08"} if self.rule_id == "SFD-A04" else {"SFD-A05", "SFD-A08", "SFD-A09"}
            if self.dependencies is None or not set(self.dependencies.rules) <= allowed:
                raise ValueError("Velocity/contact rules require a supported basket anomaly dependency")
        if self.rule_id == "SFD-A07" and self.dependencies != Dependency(mode="all", rules=["SFD-A05"]):
            raise ValueError("Shop expansion requires abnormal COD exposure")
        if self.rule_id == "SFD-A10" and (
            self.dependencies is None or not set(self.dependencies.rules) <= {"SFD-A08", "SFD-A09"}
        ):
            raise ValueError("New category requires a basket anomaly")
        return self
