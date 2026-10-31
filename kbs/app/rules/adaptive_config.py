from typing import Literal, Self

from pydantic import BaseModel, ConfigDict, Field, model_validator

AdaptiveRuleId = Literal["SFD-A01", "SFD-A04", "SFD-A06", "SFD-A08", "SFD-A09", "SFD-A11", "SFD-A17", "SFD-A18"]
LegacyRuleId = Literal["R1", "R2", "R3", "R4", "R5", "R6", "R7", "R8", "R9", "R10"]
RuleId = LegacyRuleId | AdaptiveRuleId
FeatureName = Literal[
    "buyer_failure_rate", "quantity_per_listing", "buyer_order_amount", "shop_order_amount", "buyer_phone_history"
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


class AdaptiveRuleDefinition(BaseModel):
    model_config = ConfigDict(extra="forbid")

    rule_id: AdaptiveRuleId
    description: str
    enabled: bool
    mode: Literal["shadow"]
    max_points: Literal[0]
    action: str = Field(min_length=1)
    evidence_group: Literal["delivery_history", "burst", "basket", "data_quality"]
    authority: Literal["team_demo_assumption_not_calibrated"]
    params: BaselineParameters
    dependencies: Dependency | None = None

    @model_validator(mode="after")
    def combined_signals(self) -> Self:
        if self.rule_id in {"SFD-A17", "SFD-A18"} and self.dependencies is not None:
            raise ValueError("Coverage and attribution cannot depend on anomaly signals")
        if self.rule_id in {"SFD-A04", "SFD-A11"}:
            allowed = {"SFD-A08"} if self.rule_id == "SFD-A04" else {"SFD-A08", "SFD-A09"}
            if self.dependencies is None or not set(self.dependencies.rules) <= allowed:
                raise ValueError("Velocity/contact rules require a supported basket anomaly dependency")
        return self
