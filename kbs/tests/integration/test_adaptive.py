import hashlib
from datetime import timedelta

import pytest
from psycopg.types.json import Jsonb
from tests.integration.conftest import CUTOFF, Dataset

from app.core.kbs_config import ADAPTIVE_RULES_PATH, RULES_PATH, SIM_BATCH
from app.repositories.order_repository import OrderRepository
from app.repositories.risk_repository import RiskRepository
from app.rules.config import RuleConfig
from app.rules.engine import RuleEngine
from app.schemas.adaptive import AdaptiveAssessment
from app.services.errors import ConflictError
from app.services.scoring_service import ScoringService

pytestmark = pytest.mark.integration


async def install(dataset: Dataset, config: RuleConfig) -> None:
    await dataset.connection.execute(
        "INSERT INTO sfd.rule_versions VALUES (%s,%s,%s,%s,now(),'fixture')",
        (
            SIM_BATCH,
            config.version,
            Jsonb(config.model_dump()),
            hashlib.sha256(config.model_dump_json().encode()).hexdigest(),
        ),
    )


async def test_shadow_version_preserves_old_assessment_and_low_release(dataset: Dataset) -> None:
    target = (await dataset.checkout("target"))[0]
    repo = RiskRepository(dataset.connection)
    service = ScoringService(repo)
    old = (await service.score([target], "shop-a"))[0]
    await repo.initialize_rules()
    config = await repo.configuration()
    assert config == RuleConfig.load(ADAPTIVE_RULES_PATH)
    new = (await service.score([target], "shop-a"))[0]
    assert new.rule_version != old.rule_version
    assert new.assessment_id != old.assessment_id
    assert (new.score, new.risk_level, new.reasons) == (old.score, old.risk_level, old.reasons)
    adaptive = AdaptiveAssessment.model_validate(new.feature_snapshot["adaptive"])
    assert adaptive.coverage.status == "insufficient_data"
    assert not adaptive.coverage.blocking
    assert next(s for s in adaptive.signals if s.rule_id == "SFD-A17").status == "fired"
    assert (await OrderRepository(dataset.connection).detail(target, "shop-a")).review_status == "released"
    assert (await repo.previous([target], "shop-a", old.rule_version))[target] == old
    assert (await service.score([target], "shop-a"))[0].assessment_id == new.assessment_id
    # New delivery information cannot enter the checkout-time snapshot, even on force replay.
    await dataset.outcome(target, at=CUTOFF + timedelta(days=1))
    replay = (await service.score([target], "shop-a", force=True))[0]
    assert replay.matches_previous
    assert replay.feature_snapshot["adaptive"] == new.feature_snapshot["adaptive"]
    await dataset.connection.execute("UPDATE sfd.orders SET review_status='hold' WHERE order_id=%s", (target,))
    await dataset.connection.execute(
        "INSERT INTO sfd.review_decisions VALUES (%s,'manual',%s,%s,'seller','fixture','hold','fixture',now())",
        (SIM_BATCH, target, replay.assessment_id),
    )
    await service.score([target], "shop-a", force=True)
    assert (await OrderRepository(dataset.connection).detail(target, "shop-a")).review_status == "hold"


async def test_bootstrap_never_replaces_custom_policy_or_rewrites_definition(dataset: Dataset) -> None:
    repo = RiskRepository(dataset.connection)
    raw = RuleConfig.load(RULES_PATH).model_dump()
    raw["version"] = "team-custom-policy"
    custom = RuleConfig.model_validate(raw)
    await install(dataset, custom)
    await repo.initialize_rules()
    assert await repo.configuration() == custom
    await install(dataset, RuleConfig.load(ADAPTIVE_RULES_PATH))
    await dataset.connection.execute(
        "UPDATE sfd.rule_versions SET config=jsonb_set(config,'{baseline_version}',%s) WHERE rule_version=%s",
        (Jsonb("tampered-baseline"), RuleConfig.load(ADAPTIVE_RULES_PATH).version),
    )
    with pytest.raises(ConflictError, match="without a new version"):
        await repo.initialize_rules()


async def test_baseline_version_change_preserves_cached_snapshot(dataset: Dataset) -> None:
    target = (await dataset.checkout("target"))[0]
    repo = RiskRepository(dataset.connection)
    await repo.initialize_rules()
    old = (await ScoringService(repo).score([target], "shop-a"))[0]
    raw = (await repo.configuration()).model_dump()
    raw.update(version="test-shadow-0.2", baseline_version="test-baseline-0.2")
    await install(dataset, RuleConfig.model_validate(raw))
    new = (await ScoringService(repo).score([target], "shop-a"))[0]
    assert new.rule_version == "test-shadow-0.2"
    assert AdaptiveAssessment.model_validate(new.feature_snapshot["adaptive"]).baseline_version == "test-baseline-0.2"
    assert (await repo.previous([target], "shop-a", old.rule_version))[target] == old


async def test_context_is_buyer_scoped_currency_isolated_and_strictly_point_in_time(dataset: Dataset) -> None:
    earlier = CUTOFF - timedelta(days=5)
    same_buyer = await dataset.checkout(
        "split", buyer="buyer", phone="old-phone", at=earlier, shops=("shop-a", "shop-b")
    )
    await dataset.outcome(same_buyer[0], party="seller")
    await dataset.outcome(same_buyer[1], party="buyer")
    unrelated = (
        await dataset.checkout("unrelated", buyer="someone-else", phone="new-phone", at=earlier, shops=("shop-b",))
    )[0]
    await dataset.outcome(unrelated)
    delayed = (await dataset.checkout("delayed", buyer="buyer", at=earlier))[0]
    await dataset.outcome(delayed, available=CUTOFF)
    late_resolution = (await dataset.checkout("late-resolution", buyer="buyer", at=earlier))[0]
    await dataset.outcome(late_resolution)
    await dataset.connection.execute(
        "UPDATE sfd.case_resolutions SET available_at=%s WHERE case_id=%s", (CUTOFF, late_resolution)
    )
    future_event = (await dataset.checkout("future-event", buyer="buyer", at=earlier))[0]
    await dataset.outcome(future_event, at=CUTOFF)
    await dataset.checkout("other-currency", buyer="buyer", at=earlier)
    await dataset.connection.execute("UPDATE sfd.checkouts SET currency='BRL' WHERE checkout_id='other-currency'")
    await dataset.checkout("raw", buyer="buyer", batch="olist_raw", shops=("raw-shop",), at=earlier)
    await dataset.checkout("future", buyer="buyer", at=CUTOFF + timedelta(seconds=1))
    target = (
        await dataset.checkout("target", buyer="buyer", phone="new-phone", shops=("shop-a", "shop-b"), amount=200)
    )[0]
    repo = RiskRepository(dataset.connection)
    config = RuleConfig.load(ADAPTIVE_RULES_PATH)
    facts = (await repo.facts([target], "shop-a", config))[0]
    context = facts.adaptive_context
    assert context
    assert context.target.amount == 200
    history = {h.order_id: h for h in context.history}
    assert set(history) == {*same_buyer, delayed, late_resolution, future_event}
    assert history[same_buyer[0]].responsible_party == "seller"
    assert history[same_buyer[1]].responsible_party == "buyer"
    assert history[delayed].outcome is None
    assert not history[delayed].resolution_verified
    assert history[future_event].outcome is None
    assert history[late_resolution].outcome == "delivery_failed"
    assert not history[late_resolution].resolution_verified
    result = RuleEngine(config).evaluate(facts.features, context)
    assert result.adaptive
    correction = next(s for s in result.adaptive.signals if s.rule_id == "SFD-A18")
    assert correction.evidence[0].observed == 1
    snapshot = result.adaptive.model_dump_json()
    for sensitive in ("old-phone", "new-phone", same_buyer[1], unrelated):
        assert sensitive not in snapshot
    assert await repo.facts([target], "shop-b", config) == []


async def test_resolution_correction_uses_latest_known_verdict_on_same_order(dataset: Dataset) -> None:
    historical = (await dataset.checkout("history", buyer="buyer", at=CUTOFF - timedelta(days=10)))[0]
    await dataset.outcome(historical, at=CUTOFF - timedelta(days=8))
    await dataset.connection.execute(
        "INSERT INTO sfd.case_resolutions "
        "VALUES ('corrected',%s,'verified','carrier','fixture','fixture',%s,%s,%s,'corrected')",
        (historical, CUTOFF - timedelta(days=1), CUTOFF + timedelta(seconds=1), SIM_BATCH),
    )
    await dataset.connection.execute(
        "INSERT INTO sfd.cases VALUES ('refund-case',%s,'refund',%s,%s,%s,'refund-case')",
        (historical, CUTOFF - timedelta(minutes=5), CUTOFF - timedelta(minutes=5), SIM_BATCH),
    )
    await dataset.connection.execute(
        "INSERT INTO sfd.case_resolutions "
        "VALUES ('refund-verdict','refund-case','verified','seller','fixture','fixture',%s,%s,%s,'refund-verdict')",
        (CUTOFF - timedelta(minutes=2), CUTOFF - timedelta(minutes=2), SIM_BATCH),
    )
    early = (await dataset.checkout("early", buyer="buyer"))[0]
    later = (await dataset.checkout("later", buyer="buyer", at=CUTOFF + timedelta(minutes=1)))[0]
    facts = await RiskRepository(dataset.connection).facts(
        [early, later], "shop-a", RuleConfig.load(ADAPTIVE_RULES_PATH)
    )
    contexts = {f.order_id: f.adaptive_context for f in facts}
    for target, expected in ((early, "buyer"), (later, "carrier")):
        context = contexts[target]
        assert context
        row = next(h for h in context.history if h.order_id == historical)
        assert row.resolution_verified
        assert row.responsible_party == expected


async def test_delivered_item_baseline_ignores_open_late_and_other_shop_orders(dataset: Dataset) -> None:
    for index in range(1, 11):
        order = (await dataset.checkout(f"history-{index}", buyer="buyer", at=CUTOFF - timedelta(days=2)))[0]
        await dataset.connection.execute("UPDATE sfd.order_items SET quantity=%s WHERE order_id=%s", (index, order))
        await dataset.outcome(order, failed=False)
    for name in ("open", "late"):
        order = (await dataset.checkout(name, buyer="buyer", amount=999999, at=CUTOFF - timedelta(days=1)))[0]
        if name == "late":
            await dataset.outcome(order, failed=False, available=CUTOFF + timedelta(seconds=1))
    other = (
        await dataset.checkout(
            "other-shop", buyer="buyer", amount=999999, shops=("shop-b",), at=CUTOFF - timedelta(days=2)
        )
    )[0]
    await dataset.outcome(other, failed=False)
    target = (await dataset.checkout("target", buyer="buyer"))[0]
    await dataset.connection.execute("UPDATE sfd.order_items SET quantity=20 WHERE order_id=%s", (target,))
    config = RuleConfig.load(ADAPTIVE_RULES_PATH)
    facts = (await RiskRepository(dataset.connection).facts([target], "shop-a", config))[0]
    result = RuleEngine(config).evaluate(facts.features, facts.adaptive_context)
    assert result.adaptive
    quantity = next(s for s in result.adaptive.signals if s.rule_id == "SFD-A08")
    assert quantity.status == "fired"
    assert quantity.evidence[0].observed == 20
    baseline = quantity.evidence[0].baseline
    assert baseline
    assert baseline.sample_count == 10
    assert str(baseline.threshold) == "9.1"
    amount = next(s for s in result.adaptive.signals if s.rule_id == "SFD-A09")
    assert amount.status == "fired"
    assert amount.evidence[0].comparison_threshold == 910
