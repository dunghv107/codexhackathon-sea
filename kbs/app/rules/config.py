from pathlib import Path
from typing import Annotated, Literal, Self

from pydantic import BaseModel, ConfigDict, Field, model_validator

from app.rules.adaptive_config import AdaptiveRuleDefinition, Dependency, LegacyRuleId, RuleId


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

    rule_id: LegacyRuleId
    description: str
    max_points: int = Field(ge=0, le=100)
    enabled: bool
    params: RuleParameters
    authority: str
    dependencies: Dependency | None = None

    @property
    def dependency(self) -> Dependency | None:
        if self.dependencies is not None:
            return self.dependencies
        if self.params.requires == "R3 OR R6":
            return Dependency(mode="any", rules=["R3", "R6"])
        if self.params.requires == "R7 OR R5":
            return Dependency(mode="any", rules=["R7", "R5"])
        return None

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
        if self.dependencies is None and self.params.requires != expected_dependency:
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
    rules: list[Annotated[RuleDefinition | AdaptiveRuleDefinition, Field(discriminator="rule_id")]]
    baseline_version: str | None = Field(default=None, min_length=1)
    base_version: str | None = Field(default=None, min_length=1)

    @model_validator(mode="after")
    def validate_rules(self) -> Self:
        ids = [rule.rule_id for rule in self.rules]
        if not ids or len(set(ids)) != len(ids) or sorted(ids) != sorted(self.dependency_order):
            raise ValueError("Supported rules must occur once in rules and dependency_order")
        if not self.thresholds.medium_min < self.thresholds.high_min <= self.score_cap:
            raise ValueError("Invalid risk thresholds")
        rules = {rule.rule_id: rule for rule in self.rules}
        if "SFD-A17" in ids and any(
            isinstance(rule, AdaptiveRuleDefinition)
            and self.dependency_order.index(rule.rule_id) > self.dependency_order.index("SFD-A17")
            for rule in self.rules
        ):
            raise ValueError("Coverage must follow adaptive feature evaluation")
        for rule in self.rules:
            adaptive = isinstance(rule, AdaptiveRuleDefinition)
            if adaptive and (not self.baseline_version or not self.base_version):
                raise ValueError("Adaptive rules require immutable baseline and base version references")
            dependency = rule.dependencies if adaptive else rule.dependency
            if dependency is None:
                continue
            for parent in dependency.rules:
                if parent not in ids or self.dependency_order.index(parent) >= self.dependency_order.index(
                    rule.rule_id
                ):
                    raise ValueError("Dependency must precede dependent rule; missing or cyclic dependency")
                if rule.enabled and not rules[parent].enabled:
                    raise ValueError("Enabled rule depends on a disabled rule")
                if not adaptive and isinstance(rules[parent], AdaptiveRuleDefinition):
                    raise ValueError("Shadow rules cannot affect scoring rule dependencies")
        return self

    @classmethod
    def load(cls, path: Path) -> Self:
        return cls.model_validate_json(path.read_text(encoding="utf-8"))
