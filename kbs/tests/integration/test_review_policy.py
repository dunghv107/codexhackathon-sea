from datetime import timedelta
from pathlib import Path

import pytest
from tests.integration.conftest import CUTOFF, Dataset

from app.core.kbs_config import RULES_PATH, SIM_BATCH
from app.repositories.order_repository import OrderRepository
from app.repositories.risk_repository import RiskRepository
from app.rules.config import RuleConfig
from app.schemas.risk import Assessment, OrderFacts, RiskLevel, ScoreResult
from app.services.scoring_service import ScoringService

pytestmark = pytest.mark.integration

MIGRATION = Path(__file__).resolve().parents[3] / "backend/migrations/000011_low_risk_release.up.sql"


async def legacy_assessment(dataset: Dataset, order: str, risk: RiskLevel = "low") -> Assessment:
    cursor = await dataset.connection.execute(
        "INSERT INTO sfd.risk_assessments "
        "(batch_id,assessment_id,order_id,assessed_at,rule_version,score,risk_level,"
        "feature_snapshot,reasons,input_cutoff_at) VALUES (%s,%s,%s,%s,%s,%s,%s,'{}','[]',%s) RETURNING *",
        (
            SIM_BATCH,
            f"legacy:{order}",
            order,
            CUTOFF + timedelta(minutes=1),
            RuleConfig.load(RULES_PATH).version,
            {"low": 0, "medium": 30, "high": 60}[risk],
            risk,
            CUTOFF,
        ),
    )
    return Assessment.model_validate(await cursor.fetchone())


async def migrate(dataset: Dataset) -> None:
    # Keep the migration inside the fixture's rollback transaction.
    await dataset.connection.execute(MIGRATION.read_bytes().replace(b"BEGIN;", b"").replace(b"COMMIT;", b""))


@pytest.mark.parametrize("operation", ["cached", "force", "migration"])
@pytest.mark.parametrize(
    ("review", "fulfillment", "actor", "action", "expected"),
    [
        ("pending", "unfulfilled", None, None, "released"),
        ("hold", "unfulfilled", "policy", "hold", "released"),
        ("hold", "unfulfilled", "seller", "hold", "hold"),
        ("hold", "unfulfilled", "seller", "keep_hold", "hold"),
        ("hold", "unfulfilled", None, None, "hold"),
        ("released", "unfulfilled", "seller", "release", "released"),
        ("released", "packing", None, None, "released"),
        ("released", "shipped", "policy", "release", "released"),
        ("released", "delivered", None, None, "released"),
        ("pending", "canceled", None, None, "pending"),
        ("released", "returned", "policy", "release", "released"),
    ],
)
async def test_low_release_preserves_manual_holds_and_fulfillment(
    dataset: Dataset,
    operation: str,
    review: str,
    fulfillment: str,
    actor: str | None,
    action: str | None,
    expected: str,
) -> None:
    target = (await dataset.checkout("target"))[0]
    assessment = await legacy_assessment(dataset, target)
    await dataset.connection.execute(
        "UPDATE sfd.orders SET review_status=%s, fulfillment_status=%s WHERE order_id=%s",
        (review, fulfillment, target),
    )
    if actor:
        await dataset.connection.execute(
            "INSERT INTO sfd.review_decisions VALUES (%s,'decision',%s,%s,%s,'fixture',%s,'fixture',%s)",
            (SIM_BATCH, target, assessment.assessment_id, actor, action, CUTOFF + timedelta(minutes=2)),
        )
    for _ in range(2):
        if operation == "migration":
            await migrate(dataset)
        else:
            scored = (
                await ScoringService(RiskRepository(dataset.connection)).score(
                    [target], "shop-a", force=operation == "force"
                )
            )[0]
            if operation == "cached":
                assert scored.assessment_id == assessment.assessment_id
        detail = await OrderRepository(dataset.connection).detail(target, "shop-a")
        assert detail.review_status == expected
        assert detail.fulfillment_status == fulfillment
        changed = expected != review
        assert len(detail.decisions) == int(actor is not None) + int(changed)
        if changed:
            decision = detail.decisions[-1]
            assert (decision.actor_type, decision.action, decision.reason) == ("policy", "release", "Low risk score")
            assert decision.created_at > assessment.assessed_at


@pytest.mark.parametrize(
    ("risk", "score", "expected"), [("low", 29, "released"), ("medium", 30, "hold"), ("high", 60, "hold")]
)
async def test_new_assessment_sets_gate_and_audits_once(
    dataset: Dataset, risk: RiskLevel, score: int, expected: str
) -> None:
    target = (await dataset.checkout("target"))[0]
    repo = RiskRepository(dataset.connection)
    assessment = await repo.save(
        OrderFacts(order_id=target, shop_id="shop-a", input_cutoff_at=CUTOFF, features={}),
        RuleConfig.load(RULES_PATH),
        ScoreResult(score=score, risk_level=risk, reasons=[], feature_snapshot={}, unknown_reasons={}),
    )
    await repo.apply_review_policy(assessment, "shop-a")
    detail = await OrderRepository(dataset.connection).detail(target, "shop-a")
    assert detail.review_status == expected
    assert len(detail.decisions) == 1
    assert detail.decisions[0].action == ("release" if risk == "low" else "hold")
    assert detail.decisions[0].actor_type == "policy"


async def test_new_medium_reholds_auto_release_and_stale_low_cannot_release_it(dataset: Dataset) -> None:
    target = (await dataset.checkout("target"))[0]
    repo = RiskRepository(dataset.connection)
    service = ScoringService(repo)
    low = (await service.score([target], "shop-a"))[0]
    assert (await OrderRepository(dataset.connection).detail(target, "shop-a")).review_status == "released"
    await repo.save(
        OrderFacts(order_id=target, shop_id="shop-a", input_cutoff_at=CUTOFF, features={}),
        RuleConfig.load(RULES_PATH),
        ScoreResult(score=30, risk_level="medium", reasons=[], feature_snapshot={}, unknown_reasons={}),
    )
    await repo.apply_review_policy(low, "shop-a")
    await migrate(dataset)
    detail = await OrderRepository(dataset.connection).detail(target, "shop-a")
    assert detail.review_status == "hold"
    assert [decision.action for decision in detail.decisions] == ["release", "hold"]


@pytest.mark.parametrize("risk", ["medium", "high"])
async def test_migration_and_cached_scores_do_not_release_non_low_orders(dataset: Dataset, risk: RiskLevel) -> None:
    target = (await dataset.checkout("target"))[0]
    await legacy_assessment(dataset, target, risk)
    unassessed = (await dataset.checkout("unassessed"))[0]
    await migrate(dataset)
    await ScoringService(RiskRepository(dataset.connection)).score([target], "shop-a")
    for order in (target, unassessed):
        detail = await OrderRepository(dataset.connection).detail(order, "shop-a")
        assert detail.review_status == "pending"
        assert detail.decisions == []


async def test_release_and_assessment_roll_back_together(dataset: Dataset) -> None:
    target = (await dataset.checkout("target"))[0]
    async with dataset.connection.transaction(force_rollback=True):
        await ScoringService(RiskRepository(dataset.connection)).score([target], "shop-a")
        assert (await OrderRepository(dataset.connection).detail(target, "shop-a")).review_status == "released"
    detail = await OrderRepository(dataset.connection).detail(target, "shop-a")
    assert detail.review_status == "pending"
    assert detail.assessment is None
    assert detail.decisions == []


@pytest.mark.parametrize("fulfillment", ["packing", "shipped", "delivered", "returned", "canceled"])
async def test_high_assessment_preserves_fulfillment_in_progress(dataset: Dataset, fulfillment: str) -> None:
    target = (await dataset.checkout("target"))[0]
    await dataset.connection.execute(
        "UPDATE sfd.orders SET review_status='released', fulfillment_status=%s WHERE order_id=%s",
        (fulfillment, target),
    )
    await RiskRepository(dataset.connection).save(
        OrderFacts(order_id=target, shop_id="shop-a", input_cutoff_at=CUTOFF, features={}),
        RuleConfig.load(RULES_PATH),
        ScoreResult(score=60, risk_level="high", reasons=[], feature_snapshot={}, unknown_reasons={}),
    )
    detail = await OrderRepository(dataset.connection).detail(target, "shop-a")
    assert (detail.review_status, detail.fulfillment_status) == ("released", fulfillment)
    assert detail.decisions == []
