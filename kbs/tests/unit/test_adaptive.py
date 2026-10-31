from datetime import UTC, datetime, timedelta
from decimal import Decimal

import pytest
from pydantic import ValidationError

from app.core.kbs_config import ADAPTIVE_RULES_PATH, RULES_PATH
from app.rules.adaptive import AdaptiveEvaluator
from app.rules.adaptive_config import AdaptiveRuleDefinition
from app.rules.config import RuleConfig
from app.rules.engine import RuleEngine
from app.schemas.adaptive import AdaptiveAssessment, AdaptiveContext, HistoricalOrder, ShadowSignal
from app.schemas.risk import Features

pytestmark = pytest.mark.unit
CUTOFF = datetime(2026, 10, 25, 10, tzinfo=UTC)


def configuration() -> RuleConfig:
    raw = RuleConfig.load(ADAPTIVE_RULES_PATH).model_dump()
    for rule in raw["rules"]:
        if rule["rule_id"].startswith("SFD-"):
            rule["params"].update(min_samples=3, min_resolved=2, quantile=0.75, max_age_days=10, lookback_days=30)
    return RuleConfig.model_validate(raw)


def order(name: str = "target", **changes: object) -> HistoricalOrder:
    data: dict[str, object] = {
        "order_id": name,
        "checkout_id": name,
        "buyer_id": "buyer",
        "shop_id": "shop",
        "placed_at": CUTOFF,
        "currency": "VND",
        "amount": 100,
        "quantities": {"sku": 1},
        "phone_key": "phone",
        "address_key": "address",
    }
    data.update(changes)
    return HistoricalOrder.model_validate(data)


def delivered(name: str, **changes: object) -> HistoricalOrder:
    data: dict[str, object] = {
        "placed_at": CUTOFF - timedelta(days=2),
        "outcome": "delivery_succeeded",
        "outcome_available_at": CUTOFF - timedelta(days=1),
    }
    data.update(changes)
    return order(name, **data)


def assess(
    history: list[HistoricalOrder], target: HistoricalOrder | None = None, config: RuleConfig | None = None
) -> AdaptiveAssessment:
    return AdaptiveEvaluator(
        config or configuration(), AdaptiveContext(target=target or order(), history=history), {}
    ).evaluate()


def signal(result: AdaptiveAssessment, name: str) -> ShadowSignal:
    return next(s for s in result.signals if s.rule_id == name)


def test_quantity_uses_each_listing_distribution_and_buyer_history_for_wholesale() -> None:
    history = [delivered(str(i), quantities={"retail": i, "wholesale": i * 100}) for i in (1, 2, 3)]
    result = assess(history, order(quantities={"retail": 20, "wholesale": 20}))
    facts = signal(result, "SFD-A08").evidence
    assert [(f.observed, f.comparison_threshold) for f in facts] == [(20, Decimal("2.5")), (20, 250)]
    assert all(f.baseline and f.baseline.cohort == "buyer_listing" for f in facts)
    assert signal(assess(history, order(quantities={"wholesale": 200})), "SFD-A08").status == "clear"


def test_quantity_falls_back_to_shop_but_not_an_unrelated_listing() -> None:
    history = [delivered(str(i), buyer_id="peer", quantities={"sku": i}) for i in (1, 2, 3)]
    result = signal(assess(history, order(quantities={"sku": 9})), "SFD-A08")
    assert result.status == "fired"
    assert result.evidence[0].baseline
    assert result.evidence[0].baseline.cohort == "shop_listing"
    assert signal(assess(history, order(quantities={"other": 9})), "SFD-A08").status == "unknown"
    assert signal(assess(history, order(quantities={})), "SFD-A08").status == "unknown"


@pytest.mark.parametrize(
    ("values", "days", "message"),
    [
        ([1, 2], 1, "Cần 3 mẫu"),
        ([2, 2, 2], 1, "không có biến thiên"),
        ([1, 2, 3], 11, "quá cũ"),
        ([1, 2, 3], 40, "Cần 3 mẫu"),
    ],
)
def test_invalid_baseline_is_unknown(values: list[int], days: int, message: str) -> None:
    history = [
        delivered(
            str(i),
            quantities={"sku": value},
            placed_at=CUTOFF - timedelta(days=days + 1),
            outcome_available_at=CUTOFF - timedelta(days=days),
        )
        for i, value in enumerate(values)
    ]
    result = signal(assess(history, order(quantities={"sku": 99})), "SFD-A08")
    assert result.status == "unknown"
    assert result.evidence[0].missing_reason
    assert message in result.evidence[0].missing_reason
    assert result.evidence[0].comparison_threshold is None


def test_amount_needs_both_buyer_and_shop_anomaly_and_preserves_decimal() -> None:
    history = [delivered(str(i), amount=Decimal(i) + Decimal("0.01")) for i in (100, 200, 300)]
    # A high-value buyer remains ordinary compared with this shop's distribution.
    history += [delivered(f"peer-{i}", buyer_id="peer", amount=i) for i in (1000, 2000, 3000)]
    result = signal(assess(history, order(amount=500)), "SFD-A09")
    assert result.status == "clear"
    assert result.evidence[0].comparison_threshold == Decimal("250.01")
    assert result.evidence[1].comparison_threshold == 1750
    assert signal(assess(history, order(amount=3001)), "SFD-A09").status == "fired"
    assert signal(assess(history, order(buyer_id="new", amount=3001)), "SFD-A09").status == "unknown"


def burst_history() -> list[HistoricalOrder]:
    return [order(str(i), placed_at=CUTOFF - timedelta(minutes=30 - i * 10)) for i in range(3)]


def test_velocity_counts_checkout_anchors_and_requires_quantity_anomaly() -> None:
    history = burst_history()
    # The other shop order in one checkout is not another purchase at this shop.
    history.append(order("split", checkout_id="0", shop_id="other", placed_at=history[0].placed_at))
    history += [delivered(f"base-{i}", buyer_id="peer", quantities={"sku": i}) for i in (1, 2, 3)]
    result = signal(assess(history, order(quantities={"sku": 10})), "SFD-A04")
    assert result.status == "fired"
    fact = result.evidence[0]
    assert fact.observed == 4
    assert fact.comparison_threshold == Decimal("2.5")
    assert fact.baseline
    assert fact.baseline.sample_count == 3
    assert fact.baseline.selection_policy == "checkout_anchored_windows_including_anchor"
    assert signal(assess(history), "SFD-A04").status == "clear"
    assert signal(assess(burst_history()), "SFD-A04").status == "unknown"


def test_duplicates_use_matching_basket_address_and_asof_closed_state() -> None:
    history = burst_history()
    assert signal(assess(history), "SFD-A06").status == "fired"
    history[0].terminal_at = CUTOFF - timedelta(minutes=1)
    history[1].terminal_at = CUTOFF - timedelta(minutes=1)
    result = signal(assess(history), "SFD-A06")
    assert result.status == "clear"
    assert result.evidence[0].observed == 2
    # These orders were still open at the historical anchors.
    assert result.evidence[0].comparison_threshold == Decimal("2.5")
    assert signal(assess(history, order(address_key="different")), "SFD-A06").status == "clear"
    assert signal(assess(history, order(quantities={"sku": 2})), "SFD-A06").status == "clear"
    assert signal(assess(history, order(address_key=None)), "SFD-A06").status == "unknown"


def test_contact_requires_history_and_a_basket_signal_and_supports_all_dependencies() -> None:
    history = [delivered(str(i), quantities={"sku": i}, amount=i * 100) for i in (1, 2, 3)]
    result = assess(history, order(phone_key="new", quantities={"sku": 10}))
    assert signal(result, "SFD-A11").status == "fired"
    assert signal(assess(history, order(phone_key="new")), "SFD-A11").status == "clear"
    assert signal(assess(history, order(quantities={"sku": 10})), "SFD-A11").status == "clear"
    assert signal(assess(history, order(phone_key=None, quantities={"sku": 10})), "SFD-A11").status == "unknown"
    config = configuration()
    rule = next(r for r in config.rules if r.rule_id == "SFD-A11")
    assert rule.dependencies
    rule.dependencies.mode = "all"
    assert signal(assess(history, order(phone_key="new", quantities={"sku": 10}), config), "SFD-A11").status == "clear"
    old = [delivered(str(i), placed_at=CUTOFF - timedelta(days=12), quantities={"sku": i}) for i in (1, 2, 3)]
    assert "quá cũ" in (signal(assess(old, order(quantities={"sku": 10})), "SFD-A11").missing_reason or "")


def test_failure_rate_uses_empirical_prior_successes_and_verified_attribution() -> None:
    peers = []
    for peer, failed_count in enumerate((0, 1, 2)):
        peers.extend(
            delivered(
                f"peer-{peer}-{index}",
                buyer_id=f"peer-{peer}",
                outcome="delivery_failed" if index < failed_count else "delivery_succeeded",
                resolution_verified=True,
                responsible_party="buyer",
            )
            for index in range(2)
        )
    buyer = [
        delivered(
            f"buyer-{i}",
            outcome="delivery_failed",
            responsible_party="buyer",
            resolution_verified=True,
            phone_key="old",
        )
        for i in range(4)
    ]
    result = signal(assess(peers + buyer, order(phone_key="new")), "SFD-A01")
    # Cohort prior=3/6; peer smoothed rates=.25,.5,.75; Q(.75)=.625; effect=.1.
    assert result.status == "fired"
    assert result.evidence[0].observed == Decimal(5) / 6
    assert result.evidence[0].comparison_threshold == Decimal(".725")
    buyer[0].responsible_party = "seller"
    buyer[1].responsible_party = "carrier"
    buyer[2].resolution_verified = False
    result = assess(peers + buyer)
    assert signal(result, "SFD-A01").evidence[0].observed == Decimal(2) / 5
    correction = signal(result, "SFD-A18")
    assert correction.status == "fired"
    assert correction.evidence[0].observed == 2
    assert signal(assess(buyer), "SFD-A01").status == "unknown"


def test_coverage_is_required_features_only_and_shadow_never_changes_score() -> None:
    config = configuration()
    rule = next(r for r in config.rules if isinstance(r, AdaptiveRuleDefinition) and r.rule_id == "SFD-A17")
    rule.params.required_features = ["quantity_per_listing"]
    history = [delivered(str(i), quantities={"sku": i}) for i in (1, 2, 3)]
    features: Features = {"R3": {"count": Decimal(3)}, "R5": {"checkouts": Decimal(3)}}
    original = RuleEngine(RuleConfig.load(RULES_PATH)).evaluate(features)
    result = RuleEngine(config).evaluate(
        features, AdaptiveContext(target=order(quantities={"sku": 20}), history=history)
    )
    assert (result.score, result.risk_level, result.reasons) == (original.score, original.risk_level, original.reasons)
    assert result.adaptive
    assert result.adaptive.coverage.status == "complete"
    assert result.adaptive.coverage.known_features == ["quantity_per_listing"]
    assert not result.adaptive.coverage.blocking
    assert signal(result.adaptive, "SFD-A17").status == "clear"
    assert all(s.contribution == 0 for s in result.adaptive.signals)
    assert "SFD-A08" in result.adaptive.evidence_groups["basket"]
    missing = assess([])
    assert signal(missing, "SFD-A17").status == "fired"
    assert "device" not in missing.coverage.missing_features
    assert "phone_key" not in result.adaptive.model_dump_json()
    assert '"buyer_id"' not in result.adaptive.model_dump_json()
    with pytest.raises(ValueError, match="point-in-time context"):
        RuleEngine(config).evaluate(features)


def test_recent_verdict_does_not_refresh_an_old_successful_delivery_baseline() -> None:
    history = [
        delivered(
            f"peer-{peer}-{index}",
            buyer_id=f"peer-{peer}",
            placed_at=CUTOFF - timedelta(days=12),
            outcome="delivery_failed" if index < peer else "delivery_succeeded",
            outcome_available_at=CUTOFF - timedelta(days=11),
            responsible_party="buyer",
            resolution_verified=True,
            resolution_available_at=CUTOFF - timedelta(days=11 if index < peer else 1),
        )
        for peer in range(3)
        for index in range(2)
    ]
    result = signal(assess(history), "SFD-A01")
    baseline = result.evidence[0].baseline
    assert baseline
    assert baseline.available_at == CUTOFF - timedelta(days=11)
    assert baseline.missing_reason == "Baseline quá cũ"


@pytest.mark.parametrize(
    ("change", "value"),
    [
        ("mode", "active"),
        ("max_points", 10),
        ("rule_id", "SFD-A99"),
        ("authority", "calibrated"),
        ("dependencies", {"mode": "any", "rules": ["missing"]}),
    ],
)
def test_config_rejects_unsupported_or_scoring_adaptive_rules(change: str, value: object) -> None:
    raw = configuration().model_dump()
    next(r for r in raw["rules"] if r["rule_id"] == "SFD-A08")[change] = value
    with pytest.raises(ValidationError):
        RuleConfig.model_validate(raw)


@pytest.mark.parametrize(
    ("parameter", "value"),
    [
        ("quantile", 1),
        ("min_samples", 1),
        ("alpha", 0),
        ("lookback_days", 0),
        ("max_age_days", 31),
        ("required_features", []),
        ("required_features", ["device"]),
        ("quantity_cohorts", []),
        ("quantity_cohorts", ["shop_listing", "shop_listing"]),
        ("minimum_effect", -1),
    ],
)
def test_config_rejects_invalid_baseline_parameters(parameter: str, value: object) -> None:
    raw = configuration().model_dump()
    next(r for r in raw["rules"] if r["rule_id"] == "SFD-A08")["params"][parameter] = value
    with pytest.raises(ValidationError):
        RuleConfig.model_validate(raw)


@pytest.mark.parametrize(
    "case",
    [
        "cycle",
        "missing",
        "disabled",
        "shadow_parent",
        "duplicate",
        "coverage_order",
        "baseline",
        "ungated",
        "coverage_dependency",
    ],
)
def test_config_validates_dependencies(case: str) -> None:
    raw = configuration().model_dump()
    rules = {r["rule_id"]: r for r in raw["rules"]}
    if case == "cycle":
        rules["SFD-A08"]["dependencies"] = {"mode": "any", "rules": ["SFD-A04"]}
    elif case == "missing":
        raw["rules"].remove(rules["R3"])
        raw["dependency_order"].remove("R3")
    elif case == "disabled":
        rules["SFD-A08"]["enabled"] = False
    elif case == "shadow_parent":
        rules["R10"]["dependencies"] = {"mode": "any", "rules": ["SFD-A08"]}
        raw["dependency_order"].remove("R10")
        raw["dependency_order"].append("R10")
    elif case == "duplicate":
        rules["SFD-A11"]["dependencies"]["rules"] = ["SFD-A08", "SFD-A08"]
    elif case == "coverage_order":
        raw["dependency_order"].remove("SFD-A17")
        raw["dependency_order"].insert(0, "SFD-A17")
    elif case == "baseline":
        raw["baseline_version"] = None
    elif case == "ungated":
        rules["SFD-A04"]["dependencies"] = None
    else:
        rules["SFD-A17"]["dependencies"] = {"mode": "any", "rules": ["SFD-A08"]}
    with pytest.raises(ValidationError):
        RuleConfig.model_validate(raw)


def test_legacy_subset_and_typed_all_dependency_remain_supported() -> None:
    raw = RuleConfig.load(RULES_PATH).model_dump()
    raw["rules"] = [r for r in raw["rules"] if r["rule_id"] in {"R3", "R6", "R5"}]
    raw["dependency_order"] = ["R3", "R6", "R5"]
    next(r for r in raw["rules"] if r["rule_id"] == "R5")["dependencies"] = {
        "mode": "all",
        "rules": ["R3", "R6"],
    }
    config = RuleConfig.model_validate(raw)
    result = RuleEngine(config).evaluate({"R3": {"count": Decimal(3)}, "R5": {"checkouts": Decimal(99)}})
    assert [r.rule_id for r in result.reasons] == ["R3"]
