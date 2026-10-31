from decimal import Decimal

from app.rules.adaptive import AdaptiveEvaluator
from app.rules.adaptive_config import AdaptiveRuleDefinition
from app.rules.config import RuleConfig, RuleDefinition
from app.schemas.adaptive import AdaptiveContext, SignalStatus
from app.schemas.risk import Features, Reason, ScoreResult


class RuleEngine:
    """Evaluate observed facts without database, clock, or model-provider access."""

    def __init__(self, config: RuleConfig) -> None:
        self.config = config
        self.rules = {rule.rule_id: rule for rule in config.rules if isinstance(rule, RuleDefinition)}

    def evaluate(self, features: Features, adaptive_context: AdaptiveContext | None = None) -> ScoreResult:
        snapshot = {key: dict(values) for key, values in features.items()}
        reasons: list[Reason] = []
        unknown: dict[str, str] = {}
        fired: set[str] = set()
        for rule_id in self.config.dependency_order:
            rule = self.rules.get(rule_id)
            if rule is None or not rule.enabled:
                continue
            facts = snapshot.setdefault(rule_id, {})
            try:
                points = self._points(rule, facts)
            except LookupError as error:
                unknown[rule_id] = str(error)
                continue
            dependency = rule.dependency
            if dependency:
                matches = [parent in fired for parent in dependency.rules]
                if not (any(matches) if dependency.mode == "any" else all(matches)):
                    points = 0
            if points:
                fired.add(rule_id)
                reasons.append(Reason(rule_id=rule_id, contribution=points, description=rule.description))
        score = min(self.config.score_cap, sum(reason.points for reason in reasons))
        level = "high" if score >= self.config.thresholds.high_min else "medium"
        if score < self.config.thresholds.medium_min:
            level = "low"
        adaptive = None
        if any(isinstance(rule, AdaptiveRuleDefinition) and rule.enabled for rule in self.config.rules):
            if adaptive_context is None:
                raise ValueError("Adaptive rules require their point-in-time context")
            statuses: dict[str, SignalStatus] = {
                rule_id: "fired" if rule_id in fired else ("unknown" if rule_id in unknown else "clear")
                for rule_id in self.rules
            }
            adaptive = AdaptiveEvaluator(self.config, adaptive_context, statuses).evaluate()
        return ScoreResult(
            score=score,
            risk_level=level,
            reasons=reasons,
            feature_snapshot=snapshot,
            unknown_reasons=unknown,
            adaptive=adaptive,
        )

    @staticmethod
    def _value(facts: dict[str, Decimal | None], key: str) -> Decimal:
        value = facts.get(key)
        if value is None:
            raise LookupError(f"Unknown {key}")
        return value

    def _points(self, rule: RuleDefinition, facts: dict[str, Decimal | None]) -> int:
        def value(key: str) -> Decimal:
            return self._value(facts, key)

        def param(key: str) -> Decimal:
            return Decimal(str(rule.params.number(key)))

        match rule.rule_id:
            case "R1" | "R2":
                resolved = value("resolved")
                if resolved == 0:
                    raise LookupError("No resolved history")
                rate = (value("failed") + param("alpha") * param("prior")) / (resolved + param("alpha"))
                facts["smoothed_rate"] = rate
                return max(
                    (points for threshold, points in rule.params.tiers or [] if rate >= Decimal(str(threshold))),
                    default=0,
                )
            case "R3" | "R10":
                matches = value("count") >= param("minimum")
            case "R4":
                matches = value("checkouts") >= param("minimum_distinct_checkouts") and value("shops") >= param(
                    "minimum_shops"
                )
            case "R5":
                matches = value("checkouts") >= param("minimum_checkouts")
            case "R6":
                resolved = value("resolved")
                if resolved == 0:
                    raise LookupError("No resolved history")
                facts["failure_rate"] = value("failed") / resolved
                matches = (
                    value("buyers") >= param("minimum_buyers")
                    and resolved >= param("minimum_resolved_checkouts")
                    and facts["failure_rate"] > param("buyer_failure_rate_strictly_above")
                )
            case "R7":
                if value("cod") == 0:
                    return 0
                if value("history_orders") < param("minimum_history_orders"):
                    raise LookupError("Insufficient shop history")
                median = value("median")
                if median <= 0:
                    raise LookupError("Non-positive shop median")
                facts["ratio"] = value("amount") / median
                matches = facts["ratio"] > param("ratio_strictly_above")
            case "R8":
                age = value("age_days")
                if age < 0:
                    raise LookupError("Account created after checkout")
                matches = age < param("age_days_strictly_below")
            case _:
                raise ValueError(f"No scoring handler for {rule.rule_id}")
        return rule.max_points if matches else 0
