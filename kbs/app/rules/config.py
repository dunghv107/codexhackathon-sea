from pathlib import Path
from typing import Literal, Self

from pydantic import BaseModel, ConfigDict, Field, model_validator

RuleId = Literal["R1", "R2", "R3", "R4", "R5", "R6", "R7", "R8", "R9", "R10"]


class RuleParameters(BaseModel):
    model_config = ConfigDict(extra="forbid", allow_inf_nan=False)

    window_days: float | None = Field(default=None, gt=0)
    window_hours: float | None = Field(default=None, gt=0)
    alpha: float | None = Field(default=None, gt=0)
    prior: float | None = Field(default=None, ge=0, le=1)
    tiers: list[tuple[float, int]] | None = None
    minimum: int | None = Field(default=None, ge=1)
    minimum_shops: int | None = Field(default=None, ge=1)
    minimum_distinct_checkouts: int | None = Field(default=None, ge=1)
    minimum_checkouts: int | None = Field(default=None, ge=1)
    minimum_buyers: int | None = Field(default=None, ge=1)
    minimum_resolved_checkouts: int | None = Field(default=None, ge=1)
    buyer_failure_rate_strictly_above: float | None = Field(default=None, ge=0, le=1)
    minimum_history_orders: int | None = Field(default=None, ge=1)
    ratio_strictly_above: float | None = Field(default=None, gt=0)
    age_days_strictly_below: float | None = Field(default=None, gt=0)
    requires: Literal["R3 OR R6", "R7 OR R5"] | None = None
    reason: str | None = None

    def number(self, name: str) -> float:
        value = getattr(self, name)
        if not isinstance(value, (float, int)) or isinstance(value, bool):
            raise ValueError(f"Missing numeric rule parameter: {name}")
        return float(value)


class RuleDefinition(BaseModel):
    model_config = ConfigDict(extra="forbid")

    rule_id: RuleId
    description: str
    max_points: int = Field(ge=0, le=100)
    enabled: bool
    params: RuleParameters
    authority: str

    @model_validator(mode="after")
    def validate_parameters(self) -> Self:
        required = {
            "R1": ("window_days", "alpha", "prior"),
            "R2": ("window_days", "alpha", "prior"),
            "R3": ("window_days", "minimum"),
            "R4": ("window_hours", "minimum_shops", "minimum_distinct_checkouts"),
            "R5": ("window_hours", "minimum_checkouts"),
            "R6": ("window_days", "minimum_buyers", "minimum_resolved_checkouts", "buyer_failure_rate_strictly_above"),
            "R7": ("window_days", "minimum_history_orders", "ratio_strictly_above"),
            "R8": ("age_days_strictly_below",),
            "R9": (),
            "R10": ("window_days", "minimum"),
        }
        for name in required[self.rule_id]:
            self.params.number(name)
        if self.rule_id in {"R1", "R2"}:
            tiers = self.params.tiers
            if (
                not tiers
                or tiers != sorted(tiers)
                or any(not 0 <= t <= 1 or not 0 <= p <= self.max_points for t, p in tiers)
            ):
                raise ValueError("Rate rules require sorted, bounded tiers")
        expected_dependency = {"R5": "R3 OR R6", "R8": "R7 OR R5"}.get(self.rule_id)
        if self.params.requires != expected_dependency:
            raise ValueError("Rule dependency does not match supported semantics")
        if self.rule_id == "R9" and self.enabled:
            raise ValueError("R9 cannot be enabled without a validated address feature")
        return self


class Thresholds(BaseModel):
    model_config = ConfigDict(extra="forbid")

    medium_min: int = Field(ge=1, le=100)
    high_min: int = Field(ge=1, le=100)


class RuleConfig(BaseModel):
    model_config = ConfigDict(extra="forbid")

    version: str = Field(min_length=1)
    status: str
    score_cap: int = Field(ge=1, le=100)
    thresholds: Thresholds
    missing_feature: str
    scoring: str
    dependency_order: list[RuleId]
    rules: list[RuleDefinition]

    @model_validator(mode="after")
    def validate_rules(self) -> Self:
        ids = [rule.rule_id for rule in self.rules]
        if len(ids) != 10 or len(set(ids)) != 10 or sorted(ids) != sorted(self.dependency_order):
            raise ValueError("Exactly R1-R10 must occur once in rules and dependency_order")
        if not self.thresholds.medium_min < self.thresholds.high_min <= self.score_cap:
            raise ValueError("Invalid risk thresholds")
        dependencies: dict[RuleId, tuple[RuleId, RuleId]] = {"R5": ("R3", "R6"), "R8": ("R5", "R7")}
        for rule, prerequisites in dependencies.items():
            if any(self.dependency_order.index(parent) > self.dependency_order.index(rule) for parent in prerequisites):
                raise ValueError("Dependency must precede dependent rule")
        return self

    @classmethod
    def load(cls, path: Path) -> Self:
        return cls.model_validate_json(path.read_text(encoding="utf-8"))
