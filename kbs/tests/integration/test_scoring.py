from datetime import timedelta
from decimal import Decimal

import pytest
from tests.integration.conftest import CUTOFF, Dataset

from app.core.kbs_config import RULES_PATH, SIM_BATCH
from app.repositories.order_repository import OrderRepository
from app.repositories.risk_repository import RiskRepository
from app.rules.config import RuleConfig
from app.rules.engine import RuleEngine
from app.services.errors import NotFoundError
from app.services.scoring_service import ScoringService

pytestmark = pytest.mark.integration


async def test_sql_scoring_idempotency_and_atomic_policy_hold(dataset: Dataset) -> None:
    for index in range(3):
        order = (await dataset.checkout(f"hist-{index}", at=CUTOFF - timedelta(days=10)))[0]
        await dataset.outcome(order)
    target = (await dataset.checkout("target"))[0]
    service = ScoringService(RiskRepository(dataset.connection))
    assessment = (await service.score([target], "shop-a"))[0]
    assert assessment.score == 100
    assert {r.rule_id for r in assessment.reasons} == {"R1", "R2", "R3", "R6"}
    assert (await service.score([target], "shop-a"))[0].assessment_id == assessment.assessment_id
    repository = OrderRepository(dataset.connection)
    detail = await repository.detail(target, "shop-a")
    assert detail.review_status == "hold"
    assert len(detail.decisions) == 1
    for _ in range(3):
        rescored = (await service.score([target], "shop-a", force=True))[0]
        assert rescored.matches_previous is True
    assert len((await repository.detail(target, "shop-a")).decisions) == 1


async def test_future_information_and_raw_batch_cannot_change_score(dataset: Dataset) -> None:
    target = (await dataset.checkout("target"))[0]
    repository = RiskRepository(dataset.connection)
    config = RuleConfig.load(RULES_PATH)
    for index in range(3):
        await dataset.checkout(
            f"future-{index}", at=CUTOFF - timedelta(days=10), phone="unrelated", device=None, cluster=None
        )
    before = RuleEngine(config).evaluate((await repository.facts([target], "shop-a", config))[0].features)
    for index in range(3):
        order = f"future-{index}:shop-a"
        await dataset.outcome(order, available=CUTOFF + timedelta(seconds=1))
        await dataset.connection.execute(
            "UPDATE sfd.checkout_identity_snapshots SET phone_key = 'phone', available_at = %s "
            "WHERE checkout_id = %s AND batch_id = %s",
            (CUTOFF + timedelta(seconds=1), f"future-{index}", SIM_BATCH),
        )
        raw = (
            await dataset.checkout(
                f"raw-{index}",
                at=CUTOFF - timedelta(days=10),
                shops=("raw-shop",),
                batch="olist_raw",
            )
        )[0]
        await dataset.outcome(raw, batch="olist_raw")
    after = RuleEngine(config).evaluate((await repository.facts([target], "shop-a", config))[0].features)
    assert after == before


async def test_resolution_after_cutoff_and_latest_seller_verdict(dataset: Dataset) -> None:
    historical = (await dataset.checkout("historical", at=CUTOFF - timedelta(days=10)))[0]
    await dataset.outcome(historical, party="seller")
    target = (await dataset.checkout("target"))[0]
    repository = RiskRepository(dataset.connection)
    config = RuleConfig.load(RULES_PATH)
    before = (await repository.facts([target], "shop-a", config))[0]
    assert before.features["R1"] == {"resolved": 1, "failed": 0}
    await dataset.connection.execute(
        "INSERT INTO sfd.case_resolutions VALUES ('later',%s,'verified','buyer','later','fixture',%s,%s,%s,'later')",
        (historical, CUTOFF - timedelta(hours=1), CUTOFF + timedelta(seconds=1), SIM_BATCH),
    )
    assert (await repository.facts([target], "shop-a", config))[0] == before


async def test_checkout_grain_and_tenant_isolation(dataset: Dataset) -> None:
    target = (await dataset.checkout("target", shops=("shop-a", "shop-b", "shop-c")))[0]
    config = RuleConfig.load(RULES_PATH)
    repository = RiskRepository(dataset.connection)
    facts = (await repository.facts([target], "shop-a", config))[0]
    assert facts.features["R4"] == {"checkouts": 1, "shops": 3}
    assert RuleEngine(config).evaluate(facts.features).score == 0
    with pytest.raises(NotFoundError):
        await ScoringService(repository).score([target], "shop-b")
    with pytest.raises(NotFoundError):
        await OrderRepository(dataset.connection).detail(target, "shop-b")
    assert await repository.facts([target], "shop-b", config) == []


async def test_r7_uses_shop_currency_and_historical_amounts(dataset: Dataset) -> None:
    for index in range(5):
        await dataset.checkout(f"base-{index}", at=CUTOFF - timedelta(days=10), phone=None, device=None, cluster=None)
    target = (await dataset.checkout("target", age=1, amount=301, phone=None, device=None, cluster=None))[0]
    result = (await ScoringService(RiskRepository(dataset.connection)).score([target], "shop-a"))[0]
    assert result.score == 25
    assert result.risk_level == "low"
    assert {r.rule_id for r in result.reasons} == {"R7", "R8"}


async def test_r10_requires_buyer_cancellation_after_confirmation(dataset: Dataset) -> None:
    for index in range(2):
        order = (await dataset.checkout(f"history-{index}", at=CUTOFF - timedelta(days=10)))[0]
        for event, at in (("confirmed", CUTOFF - timedelta(days=9)), ("canceled", CUTOFF - timedelta(days=8))):
            await dataset.connection.execute(
                "INSERT INTO sfd.order_events VALUES (%s,%s,%s,'buyer',NULL,%s,%s,%s,%s)",
                (f"{order}:{event}", order, event, at, at, SIM_BATCH, f"{order}:{event}"),
            )
    target = (await dataset.checkout("target"))[0]
    result = (await ScoringService(RiskRepository(dataset.connection)).score([target], "shop-a"))[0]
    assert result.score == 15
    assert [r.rule_id for r in result.reasons] == ["R10"]


async def test_delivery_not_available_before_cutoff_is_unknown(dataset: Dataset) -> None:
    order = (await dataset.checkout("history", at=CUTOFF - timedelta(days=10)))[0]
    await dataset.outcome(order, available=CUTOFF)
    target = (await dataset.checkout("target"))[0]
    repository = RiskRepository(dataset.connection)
    config = RuleConfig.load(RULES_PATH)
    facts = (await repository.facts([target], "shop-a", config))[0]
    assert facts.features["R1"] == {"resolved": 0, "failed": 0}
    assert facts.features["R3"] == {"count": 0}
    assert RuleEngine(config).evaluate(facts.features).score == 0


async def test_actual_checkout_burst_triggers_dependency_rules(dataset: Dataset) -> None:
    for index in range(3):
        order = (await dataset.checkout(f"failed-{index}", at=CUTOFF - timedelta(days=10)))[0]
        await dataset.outcome(order)
    await dataset.checkout("recent-b", at=CUTOFF - timedelta(minutes=30), shops=("shop-b",))
    await dataset.checkout("recent-c", at=CUTOFF - timedelta(minutes=15), shops=("shop-c",))
    target = (await dataset.checkout("target"))[0]
    result = (await ScoringService(RiskRepository(dataset.connection)).score([target], "shop-a"))[0]
    assert {r.rule_id for r in result.reasons} == {"R1", "R2", "R3", "R4", "R5", "R6"}
    assert result.score == 100


async def test_unknown_identity_does_not_link_nulls(dataset: Dataset) -> None:
    for index in range(3):
        order = (
            await dataset.checkout(
                f"history-{index}",
                at=CUTOFF - timedelta(days=10),
                phone=None,
                device=None,
                cluster=None,
            )
        )[0]
        await dataset.outcome(order)
    target = (await dataset.checkout("target", phone=None, device=None, cluster=None))[0]
    result = (await ScoringService(RiskRepository(dataset.connection)).score([target], "shop-a"))[0]
    assert result.score == 0
    assert result.feature_snapshot["unknown_reasons"]


async def test_median_preserves_decimal_money_at_strict_threshold(dataset: Dataset) -> None:
    for index in range(6):
        await dataset.checkout(
            f"base-{index}",
            at=CUTOFF - timedelta(days=10),
            amount=Decimal("1000000000000000.01"),
            phone=None,
            device=None,
            cluster=None,
        )
    target = (await dataset.checkout("target", amount=Decimal("3000000000000000.03")))[0]
    result = (await ScoringService(RiskRepository(dataset.connection)).score([target], "shop-a"))[0]
    assert result.score == 0
    payload = result.model_dump(mode="json")
    assert Decimal(payload["feature_snapshot"]["R7"]["median"]) == Decimal("1000000000000000.01")


async def test_rule_bootstrap_is_idempotent_and_preserves_existing_version(dataset: Dataset) -> None:
    repository = RiskRepository(dataset.connection)
    before = await repository.configuration()
    await repository.initialize_rules()
    assert (await repository.configuration()).base_version == before.version
    await dataset.connection.execute("DELETE FROM sfd.rule_versions WHERE batch_id = %s", (SIM_BATCH,))
    await repository.initialize_rules()
    await repository.initialize_rules()
    assert (await repository.configuration()).base_version == RuleConfig.load(RULES_PATH).version
    cursor = await dataset.connection.execute(
        "SELECT count(*) AS count FROM sfd.rule_versions WHERE batch_id = %s", (SIM_BATCH,)
    )
    row = await cursor.fetchone()
    assert row is not None
    assert row["count"] == 2
