from datetime import timedelta
from decimal import Decimal

import pytest
from pydantic import ValidationError
from tests.unit.test_adaptive import CUTOFF, delivered, order, signal

from app.core.kbs_config import FULL_ADAPTIVE_RULES_PATH, RULES_PATH
from app.rules.adaptive import AdaptiveEvaluator
from app.rules.adaptive_config import AdaptiveRuleDefinition
from app.rules.config import RuleConfig
from app.rules.engine import RuleEngine
from app.schemas.adaptive import AdaptiveAssessment, AdaptiveContext, HistoricalOrder

pytestmark = pytest.mark.unit


def configuration() -> RuleConfig:
    raw = RuleConfig.load(FULL_ADAPTIVE_RULES_PATH).model_dump()
    for rule in raw["rules"]:
        if rule["rule_id"].startswith("SFD-"):
            rule["params"].update(min_samples=3, min_resolved=2, quantile=0.75, max_age_days=30)
            if rule["policy"]:
                rule["policy"]["maturity_days"] = 0
    return RuleConfig.model_validate(raw)


def assess(
    history: list[HistoricalOrder], target: HistoricalOrder | None = None, **extra: object
) -> AdaptiveAssessment:
    context = AdaptiveContext.model_validate({"target": target or order(), "history": history, **extra})
    return AdaptiveEvaluator(configuration(), context, {}).evaluate()


def linked() -> list[dict[str, object]]:
    return [
        {
            "related_buyer_id": peer,
            "edge_type": kind,
            "occurred_at": CUTOFF - timedelta(days=2),
            "available_at": CUTOFF - timedelta(days=1),
            "source_ref": f"{peer}:{kind}",
        }
        for peer in ("peer1", "peer2")
        for kind in ("verified_document", "verified_payment")
    ]


def rates_history(*, after_sale: bool = False) -> list[HistoricalOrder]:
    history = []
    for buyer, bad_count, size in (("peer0", 0, 2), ("peer1", 1, 2), ("peer2", 2, 2), ("buyer", 4, 4)):
        for index in range(size):
            bad = index < bad_count
            history.append(
                delivered(
                    f"{buyer}-{index}",
                    buyer_id=buyer,
                    categories={"old"},
                    confirmed_at=CUTOFF - timedelta(days=2),
                    outcome="delivery_succeeded" if after_sale or not bad else None,
                    cancellation_at=CUTOFF - timedelta(days=1) if bad and not after_sale else None,
                    cancellation_verified=bad,
                    cancellation_party="buyer",
                    cancellation_known_at=CUTOFF - timedelta(days=1),
                    after_sale_present=bad and after_sale,
                    after_sale_verified=bad and after_sale,
                    after_sale_party="buyer",
                    after_sale_code="refund_abuse" if bad else "legitimate",
                    after_sale_known_at=CUTOFF - timedelta(days=1),
                )
            )
    return history


def test_a02_uses_confirmed_resolved_denominator_and_verified_responsibility() -> None:
    history = rates_history()
    fact = signal(assess(history), "SFD-A02")
    assert fact.status == "fired"
    assert fact.evidence[0].observed == Decimal(5) / 6
    assert fact.evidence[0].comparison_threshold == Decimal(".725")
    for row in history:
        if row.buyer_id == "buyer":
            row.cancellation_party = "seller"
    assert signal(assess(history), "SFD-A02").status == "clear"
    for row in history:
        if row.buyer_id == "buyer":
            row.cancellation_verified = False
    assert signal(assess(history), "SFD-A02").status == "unknown"


def test_a03_compares_supported_nonoverlapping_windows_without_double_points() -> None:
    reference = [
        delivered(f"old-{i}", placed_at=CUTOFF - timedelta(days=10), outcome_available_at=CUTOFF - timedelta(days=9))
        for i in range(3)
    ]
    recent = [
        delivered(f"new-{i}", outcome="delivery_failed", resolution_verified=True, responsible_party="buyer")
        for i in range(3)
    ]
    result = signal(assess(reference + recent), "SFD-A03")
    assert result.status == "fired"
    assert result.evidence[0].observed == Decimal(".6")
    assert result.evidence[0].comparison_threshold == Decimal(".1")
    assert result.contribution == 0
    recent[0].responsible_party = recent[1].responsible_party = recent[2].responsible_party = "carrier"
    assert signal(assess(reference + recent), "SFD-A03").status == "clear"
    assert signal(assess(reference + recent[:1]), "SFD-A03").status == "unknown"
    for row in reference:
        row.placed_at = CUTOFF - timedelta(days=50)
        row.outcome_available_at = CUTOFF - timedelta(days=40)
    assert "quá cũ" in (signal(assess(reference + recent), "SFD-A03").missing_reason or "")


def test_new_after_sale_verdict_does_not_refresh_stale_cancellation_baseline() -> None:
    history = rates_history()
    for row in history:
        row.placed_at = CUTOFF - timedelta(days=60)
        row.confirmed_at = CUTOFF - timedelta(days=59)
        row.outcome_available_at = CUTOFF - timedelta(days=50)
        row.cancellation_known_at = CUTOFF - timedelta(days=50)
        row.after_sale_known_at = CUTOFF - timedelta(hours=1)
    result = signal(assess(history), "SFD-A02")
    assert result.status == "unknown"
    assert result.evidence[0].baseline is not None
    assert result.evidence[0].baseline.available_at == CUTOFF - timedelta(days=50)


def cod_history() -> list[HistoricalOrder]:
    return [
        order(
            str(i), amount=(4 - i) * 100, placed_at=CUTOFF - timedelta(days=i), tracking_from=CUTOFF - timedelta(days=i)
        )
        for i in (3, 2, 1)
    ]


def test_a05_uses_open_exposure_at_checkout_anchors_including_failed_delivery() -> None:
    history = cod_history()
    history[-1].terminal_at = CUTOFF - timedelta(hours=2)  # Failed delivery does not end COD liability.
    target = order(tracking_from=CUTOFF)
    result = signal(assess(history, target), "SFD-A05")
    assert result.status == "fired"
    assert result.evidence[0].observed == 700
    assert result.evidence[0].comparison_threshold == 450
    for row in history:
        row.cod_closed_at = CUTOFF - timedelta(hours=1)
    assert signal(assess(history, target), "SFD-A05").status == "clear"
    history[0].tracking_from = None
    assert signal(assess(history, target), "SFD-A05").status == "unknown"
    assert signal(assess(history, order(payment_method="credit_card")), "SFD-A05").status == "clear"


def test_a07_does_not_penalize_one_multishop_checkout_and_requires_exposure() -> None:
    history = cod_history()
    for row in history:
        row.cod_closed_at = row.placed_at + timedelta(hours=1)
    target = order(amount=10000, tracking_from=CUTOFF)
    assert signal(assess(history, target, checkout_shops={"shop", "shop-b", "shop-c"}), "SFD-A07").status == "clear"
    history += [order(f"burst-{i}", shop_id=f"shop-{i}", placed_at=CUTOFF - timedelta(minutes=i * 10)) for i in (1, 2)]
    assert signal(assess(history, target), "SFD-A07").status == "fired"
    target.amount = Decimal(1)
    assert signal(assess(history, target), "SFD-A07").status == "clear"


def test_a10_requires_snapshot_support_and_a_basket_anomaly() -> None:
    history = [delivered(str(i), categories={"old"}, quantities={"sku": i}, amount=i * 100) for i in (1, 2, 3)]
    target = order(categories={"new"}, quantities={"sku": 10})
    assert signal(assess(history, target), "SFD-A10").status == "fired"
    assert signal(assess(history, order(categories={"new"})), "SFD-A10").status == "clear"
    history[0].categories = None
    assert signal(assess(history, target), "SFD-A10").status == "unknown"
    assert signal(assess(history, order(categories=None, quantities={"sku": 10})), "SFD-A10").status == "unknown"


def test_a12_needs_independent_identity_sources_adverse_outcomes_and_synchrony() -> None:
    history = [
        delivered(
            f"bad-{peer}", buyer_id=peer, outcome="delivery_failed", resolution_verified=True, responsible_party="buyer"
        )
        for peer in ("peer1", "peer2")
    ]
    history += [
        order(f"recent-{peer}", buyer_id=peer, placed_at=CUTOFF - timedelta(minutes=10)) for peer in ("peer1", "peer2")
    ]
    result = signal(assess(history, identity_links=linked()), "SFD-A12")
    assert result.status == "fired"
    assert result.audience == "platform"
    assert signal(assess(history), "SFD-A12").status == "unknown"
    same_source = linked()
    for link in same_source:
        link["source_ref"] = "one-document-reused"
    assert signal(assess(history, identity_links=same_source), "SFD-A12").status == "unknown"
    assert signal(assess(history[:2], identity_links=linked()), "SFD-A12").status == "clear"
    history[0].responsible_party = "seller"
    assert signal(assess(history, identity_links=linked()), "SFD-A12").status == "clear"
    old_links = linked()
    for link in old_links:
        link["occurred_at"] = CUTOFF - timedelta(days=100)
    assert signal(assess(history, identity_links=old_links), "SFD-A12").status == "unknown"


def test_a13_matches_new_account_time_active_enforcement_and_repeated_verified_behavior() -> None:
    target = order(account_created_at=CUTOFF - timedelta(days=4))
    history = [delivered("repeat", outcome="delivery_failed", resolution_verified=True, responsible_party="buyer")]
    enforcement = {
        "buyer_id": "peer1",
        "abuse_code": "delivery",
        "action": "restricted",
        "record_id": "e",
        "occurred_at": CUTOFF - timedelta(days=5),
        "available_at": CUTOFF - timedelta(days=5),
    }
    result = signal(assess(history, target, identity_links=linked(), enforcements=[enforcement]), "SFD-A13")
    assert result.status == "fired"
    enforcement["action"] = "reinstated"
    assert (
        signal(assess(history, target, identity_links=linked(), enforcements=[enforcement]), "SFD-A13").status
        == "clear"
    )
    enforcement["action"] = "restricted"
    target.account_created_at = CUTOFF - timedelta(days=6)
    assert (
        signal(assess(history, target, identity_links=linked(), enforcements=[enforcement]), "SFD-A13").status
        == "clear"
    )
    target.account_created_at = None
    assert (
        signal(assess(history, target, identity_links=linked(), enforcements=[enforcement]), "SFD-A13").status
        == "unknown"
    )


def test_a14_distinguishes_verified_abuse_from_normal_refunds_and_uses_category_cohort() -> None:
    history = rates_history(after_sale=True)
    result = signal(assess(history, order(categories={"old"})), "SFD-A14")
    assert result.status == "fired"
    assert result.evidence[0].observed == Decimal(5) / 6
    assert result.evidence[0].baseline
    assert result.evidence[0].baseline.cohort == "shop_category_currency_resolved_buyers"
    fallback = signal(assess(history, order(categories={"new"})), "SFD-A14").evidence[0].baseline
    assert fallback
    assert fallback.cohort == "shop_currency_resolved_buyers"
    for row in history:
        if row.buyer_id == "buyer":
            row.after_sale_code = "legitimate"
    assert signal(assess(history), "SFD-A14").status == "clear"
    for row in history:
        if row.buyer_id == "buyer":
            row.after_sale_verified = False
    assert signal(assess(history), "SFD-A14").status == "unknown"


@pytest.mark.parametrize(
    ("verdict", "cases", "status"),
    [
        ("contradiction_verified", 1, "fired"),
        ("contradiction_verified", 0, "clear"),
        ("no_contradiction", 1, "clear"),
        ("unresolved", 1, "unknown"),
    ],
)
def test_a15_requires_independent_case_and_verified_contradiction(verdict: str, cases: int, status: str) -> None:
    review = {
        "evidence_id": "e",
        "verdict": verdict,
        "independent_cases": cases,
        "available_at": CUTOFF - timedelta(days=1),
    }
    result = signal(assess([], evidence_reviews=[review], evidence_count=1), "SFD-A15")
    assert result.status == status
    assert "evidence_id" not in result.model_dump_json()
    assert signal(assess([], evidence_count=2), "SFD-A15").status == "unknown"


def test_a16_is_platform_only_and_all_eighteen_rules_remain_shadow() -> None:
    reference = [
        delivered(
            f"old-{i}",
            buyer_id=f"peer{i}",
            placed_at=CUTOFF - timedelta(days=10),
            outcome_available_at=CUTOFF - timedelta(days=9),
        )
        for i in range(3)
    ]
    recent = [
        delivered(f"new-{i}", buyer_id=f"peer{i}", seller_case=True, seller_case_known_at=CUTOFF - timedelta(days=1))
        for i in range(3)
    ]
    context = AdaptiveContext(target=order(), history=reference + recent)
    result = RuleEngine(configuration()).evaluate({}, context)
    legacy = RuleEngine(RuleConfig.load(RULES_PATH)).evaluate({})
    assert (result.score, result.risk_level, result.reasons) == (legacy.score, legacy.risk_level, legacy.reasons)
    assert result.adaptive
    assert len(result.adaptive.signals) == 18
    assert {s.rule_id for s in result.adaptive.signals if s.audience == "platform"} == {"SFD-A12", "SFD-A13", "SFD-A16"}
    assert signal(result.adaptive, "SFD-A16").status == "fired"
    assert all(s.mode == "shadow" and s.contribution == 0 for s in result.adaptive.signals)
    assert not result.adaptive.coverage.blocking


@pytest.mark.parametrize(
    "case", ["missing_policy", "overlap", "weak_identity", "seller_leak", "ungated_expansion", "ungated_category"]
)
def test_full_policy_rejects_invalid_or_unsafe_configuration(case: str) -> None:
    raw = configuration().model_dump()
    rules = {r["rule_id"]: r for r in raw["rules"]}
    if case == "missing_policy":
        rules["SFD-A03"]["policy"] = None
    elif case == "overlap":
        rules["SFD-A03"]["policy"]["recent_days"] = 90
    elif case == "weak_identity":
        rules["SFD-A12"]["policy"]["allowed_edge_types"] = ["verified_document", "verified_document"]
    elif case == "seller_leak":
        rules["SFD-A16"]["audience"] = "seller"
    elif case == "ungated_expansion":
        rules["SFD-A07"]["dependencies"] = None
    else:
        rules["SFD-A10"]["dependencies"] = None
    with pytest.raises(ValidationError):
        RuleConfig.model_validate(raw)


def test_all_new_handlers_have_explicit_policies() -> None:
    config = RuleConfig.load(FULL_ADAPTIVE_RULES_PATH)
    rules = [r for r in config.rules if isinstance(r, AdaptiveRuleDefinition)]
    assert len(rules) == 18
    assert config.base_version == "draft-vn-olist-adaptive-shadow-0.1"
    assert config.baseline_version == "demo-adaptive-baseline-0.2"
