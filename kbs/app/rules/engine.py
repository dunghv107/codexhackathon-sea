from decimal import Decimal

from app.rules.config import RuleConfig, RuleDefinition
from app.schemas.risk import Features, Reason, ScoreResult


class RuleEngine:
    """Evaluate observed facts without database, clock, or model-provider access."""

    def __init__(self, config: RuleConfig) -> None:
        self.config = config
        self.rules = {rule.rule_id: rule for rule in config.rules}

    def evaluate(self, features: Features) -> ScoreResult:
        snapshot = {key: dict(values) for key, values in features.items()}
        reasons: list[Reason] = []
        unknown: dict[str, str] = {}
        fired: set[str] = set()
        for rule_id in self.config.dependency_order:
            rule = self.rules[rule_id]
            if not rule.enabled:
                continue
            facts = snapshot.setdefault(rule_id, {})
            try:
                points = self._points(rule, facts, fired)
            except LookupError as error:
                unknown[rule_id] = str(error)
                continue
            if points:
                fired.add(rule_id)
                reasons.append(Reason(rule_id=rule_id, points=points, description=rule.description))
        score = min(self.config.score_cap, sum(reason.points for reason in reasons))
        level = "high" if score >= self.config.thresholds.high_min else "medium"
        if score < self.config.thresholds.medium_min:
            level = "low"
        return ScoreResult(
            score=score, risk_level=level, reasons=reasons, feature_snapshot=snapshot, unknown_reasons=unknown
        )

    @staticmethod
    def _value(facts: dict[str, Decimal | None], key: str) -> Decimal:
        value = facts.get(key)
        if value is None:
            raise LookupError(f"Unknown {key}")
        return value

    def _points(self, rule: RuleDefinition, facts: dict[str, Decimal | None], fired: set[str]) -> int:
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
                matches = value("checkouts") >= param("minimum_checkouts") and bool(fired & {"R3", "R6"})
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
                matches = age < param("age_days_strictly_below") and bool(fired & {"R7", "R5"})
            case _:
                return 0
        return rule.max_points if matches else 0
