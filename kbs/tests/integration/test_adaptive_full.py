from datetime import datetime, timedelta

import pytest
from tests.integration.conftest import CUTOFF, Dataset

from app.core.kbs_config import FULL_ADAPTIVE_RULES_PATH, SIM_BATCH
from app.repositories.order_repository import OrderRepository
from app.repositories.risk_repository import RiskRepository
from app.rules.config import RuleConfig
from app.rules.engine import RuleEngine
from app.schemas.adaptive import AdaptiveAssessment, AdaptiveContext
from app.services.errors import ConflictError
from app.services.scoring_service import ScoringService

pytestmark = pytest.mark.integration
CONFIG = RuleConfig.load(FULL_ADAPTIVE_RULES_PATH)
PAST = CUTOFF - timedelta(days=2)


async def context(dataset: Dataset, target: str) -> AdaptiveContext:
    facts = (await RiskRepository(dataset.connection).facts([target], "shop-a", CONFIG))[0]
    assert facts.adaptive_context is not None
    return facts.adaptive_context


async def event(
    dataset: Dataset, order: str, kind: str, at: datetime = PAST, available: datetime | None = None
) -> None:
    await dataset.connection.execute(
        "INSERT INTO sfd.order_events VALUES (%s,%s,%s,'platform',NULL,%s,%s,%s,%s)",
        (f"{order}:{kind}", order, kind, at, available or at, SIM_BATCH, f"{order}:{kind}"),
    )


async def verdict(
    dataset: Dataset,
    order: str,
    kind: str,
    *,
    party: str = "buyer",
    code: str = "legitimate",
    available: datetime = PAST,
) -> str:
    name = f"{order}:{kind}"
    await dataset.connection.execute(
        "INSERT INTO sfd.cases VALUES (%s,%s,%s,%s,%s,%s,%s)",
        (name, order, kind, PAST, PAST, SIM_BATCH, name),
    )
    await dataset.connection.execute(
        "INSERT INTO sfd.case_resolutions VALUES (%s,%s,'verified',%s,'fixture','fixture',%s,%s,%s,%s)",
        (name, name, party, PAST, PAST, SIM_BATCH, name),
    )
    await dataset.connection.execute(
        "INSERT INTO sfd.risk_case_outcomes VALUES (%s,%s,%s,%s,'fixture','admin:test')",
        (name, SIM_BATCH, code, available),
    )
    return name


async def test_all_rules_persist_private_signals_separately_and_replay(dataset: Dataset) -> None:
    target = (await dataset.checkout("target"))[0]
    repo = RiskRepository(dataset.connection)
    service = ScoringService(repo)
    legacy = (await service.score([target], "shop-a"))[0]
    await repo.initialize_rules()
    scored = (await service.score([target], "shop-a"))[0]
    assert scored.rule_version == CONFIG.version
    assert (scored.score, scored.risk_level, scored.reasons) == (legacy.score, legacy.risk_level, legacy.reasons)
    public = AdaptiveAssessment.model_validate(scored.feature_snapshot["adaptive"]).signals
    assert len(public) == 15
    assert {s.rule_id for s in scored.platform_signals} == {"SFD-A12", "SFD-A13", "SFD-A16"}
    for private_id in ("SFD-A12", "SFD-A13", "SFD-A16"):
        assert private_id not in scored.model_dump_json()
    cursor = await dataset.connection.execute("SELECT signals FROM sfd.risk_platform_observations")
    stored = await cursor.fetchone()
    assert stored is not None
    assert len(stored["signals"]) == 3
    replay = (await service.score([target], "shop-a", force=True))[0]
    assert replay.matches_previous
    assert (await OrderRepository(dataset.connection).detail(target, "shop-a")).review_status == "released"
    await dataset.connection.execute("UPDATE sfd.orders SET review_status='hold' WHERE order_id=%s", (target,))
    await dataset.connection.execute(
        "INSERT INTO sfd.review_decisions VALUES (%s,'manual',%s,%s,'seller','fixture','hold','fixture',now())",
        (SIM_BATCH, target, replay.assessment_id),
    )
    await service.score([target], "shop-a", force=True)
    assert (await OrderRepository(dataset.connection).detail(target, "shop-a")).review_status == "hold"


async def test_full_bootstrap_requires_migration_12(dataset: Dataset) -> None:
    await dataset.connection.execute("DROP TABLE sfd.risk_platform_observations")
    with pytest.raises(ConflictError, match="Migration 12"):
        await RiskRepository(dataset.connection).initialize_rules()


async def test_purchase_snapshots_do_not_use_mutable_categories_or_late_backfill(dataset: Dataset) -> None:
    old = (await dataset.checkout("old", buyer="buyer", at=PAST))[0]
    late = (await dataset.checkout("late", buyer="buyer", at=PAST))[0]
    target = (await dataset.checkout("target", buyer="buyer", shops=("shop-a", "shop-b")))[0]
    for order, at in ((old, PAST), (late, PAST + timedelta(seconds=1)), (target, CUTOFF)):
        await dataset.connection.execute(
            "INSERT INTO sfd.order_item_risk_snapshots VALUES (%s,%s,'books',%s)", (order, SIM_BATCH, at)
        )
        await dataset.connection.execute(
            "INSERT INTO sfd.order_risk_tracking VALUES (%s,%s,%s)", (order, SIM_BATCH, at)
        )
    await dataset.connection.execute("UPDATE sfd.products SET category_code=NULL")
    ctx = await context(dataset, target)
    assert ctx.target.categories == {"books"}
    assert ctx.checkout_shops == {"shop-a", "shop-b"}
    rows = {r.order_id: r for r in ctx.history}
    assert rows[old].categories == {"books"}
    assert rows[late].categories is None
    assert rows[old].tracking_from == PAST


async def test_cancellation_attribution_and_after_sale_are_known_before_cutoff(dataset: Dataset) -> None:
    cancelled = (await dataset.checkout("cancelled", buyer="buyer", at=PAST - timedelta(days=3)))[0]
    await event(dataset, cancelled, "confirmed", at=PAST - timedelta(days=2))
    await event(dataset, cancelled, "canceled", at=PAST - timedelta(days=1))
    await verdict(dataset, cancelled, "cancellation", code="cancellation", available=CUTOFF)
    delivered = (await dataset.checkout("delivered", buyer="buyer", at=PAST - timedelta(days=10)))[0]
    await dataset.outcome(delivered, failed=False, at=PAST - timedelta(days=8))
    await verdict(dataset, delivered, "refund", code="refund_abuse")
    case = await verdict(dataset, delivered, "return", code="legitimate", party="seller")
    # A late seller correction must not refresh or replace the known buyer verdict.
    await dataset.connection.execute(
        "INSERT INTO sfd.case_resolutions VALUES ('late',%s,'verified','buyer','fixture','fixture',%s,%s,%s,'late')",
        (case, PAST + timedelta(days=1), CUTOFF, SIM_BATCH),
    )
    first = (await dataset.checkout("first", buyer="buyer"))[0]
    second = (await dataset.checkout("second", buyer="buyer", at=CUTOFF + timedelta(seconds=1)))[0]
    rows = {r.order_id: r for r in (await context(dataset, first)).history}
    confirmed, canceled = rows[cancelled].confirmed_at, rows[cancelled].cancellation_at
    assert confirmed is not None
    assert canceled is not None
    assert confirmed < canceled
    assert not rows[cancelled].cancellation_verified
    assert rows[delivered].after_sale_verified
    assert rows[delivered].after_sale_code == "return_abuse"
    assert rows[delivered].seller_case
    later = {r.order_id: r for r in (await context(dataset, second)).history}
    assert later[cancelled].cancellation_verified
    assert not later[delivered].seller_case
    assert not later[delivered].after_sale_verified  # New verdict lacks an outcome annotation.


async def test_cod_open_history_survives_lookback_and_failed_delivery(dataset: Dataset) -> None:
    old = (await dataset.checkout("old-open", buyer="buyer", at=CUTOFF - timedelta(days=180)))[0]
    failed = (await dataset.checkout("failed", buyer="buyer", at=PAST - timedelta(days=1)))[0]
    returned = (await dataset.checkout("returned", buyer="buyer", at=PAST - timedelta(days=1)))[0]
    await dataset.outcome(failed, at=PAST)
    await event(dataset, returned, "returned", available=CUTOFF)
    other = (await dataset.checkout("brl", buyer="buyer", at=PAST))[0]
    await dataset.connection.execute("UPDATE sfd.checkouts SET currency='BRL' WHERE checkout_id='brl'")
    target = (await dataset.checkout("target", buyer="buyer"))[0]
    later = (await dataset.checkout("later", buyer="buyer", at=CUTOFF + timedelta(seconds=1)))[0]
    rows = {r.order_id: r for r in (await context(dataset, target)).history}
    assert old in rows
    assert other not in rows
    assert rows[failed].cod_closed_at is None
    assert rows[returned].cod_closed_at is None
    assert next(r for r in (await context(dataset, later)).history if r.order_id == returned).cod_closed_at == CUTOFF


async def test_identity_revocation_and_enforcement_are_point_in_time(dataset: Dataset) -> None:
    await dataset.checkout("a", buyer="a", at=PAST, shops=("shop-b",))
    await dataset.checkout("z", buyer="z", at=PAST)
    for edge, source in (("verified_document", "document:1"), ("verified_payment", "payment:1")):
        await dataset.connection.execute(
            "INSERT INTO sfd.risk_identity_assertions VALUES (%s,%s,'a','z',%s,'verified',%s,%s,%s,'admin:test')",
            (edge, SIM_BATCH, edge, PAST, PAST, source),
        )
    await dataset.connection.execute(
        "INSERT INTO sfd.risk_identity_assertions "
        "VALUES ('revoked',%s,'a','z','verified_document','revoked',%s,%s,'document:1','admin:test')",
        (SIM_BATCH, PAST + timedelta(hours=1), CUTOFF),
    )
    for record, action, available in (("restricted", "restricted", PAST), ("reinstated", "reinstated", CUTOFF)):
        await dataset.connection.execute(
            "INSERT INTO sfd.risk_enforcement_events VALUES (%s,%s,'a',%s,'delivery',%s,%s,'audit','admin:test')",
            (record, SIM_BATCH, action, PAST if action == "restricted" else PAST + timedelta(hours=1), available),
        )
    target = (await dataset.checkout("target", buyer="z"))[0]
    later = (await dataset.checkout("later", buyer="z", at=CUTOFF + timedelta(seconds=1)))[0]
    early = await context(dataset, target)
    assert len(early.identity_links) == 2
    assert early.enforcements[0].action == "restricted"
    assert any(r.buyer_id == "a" for r in early.history)
    late = await context(dataset, later)
    assert len(late.identity_links) == 1
    assert late.enforcements[0].action == "reinstated"


async def test_evidence_requires_independent_orders_and_verified_contradiction(dataset: Dataset) -> None:
    first = (await dataset.checkout("first", buyer="buyer", at=PAST - timedelta(days=1)))[0]
    second = (await dataset.checkout("second", buyer="peer", at=PAST - timedelta(days=1), shops=("shop-b",)))[0]
    for index, order in enumerate((first, second)):
        case = await verdict(dataset, order, "refund")
        await dataset.connection.execute(
            "INSERT INTO sfd.case_evidence VALUES (%s,%s,'buyer','image','private-ref',%s,%s,%s)",
            (f"proof{index}", case, PAST, SIM_BATCH, f"proof{index}"),
        )
        await dataset.connection.execute(
            "INSERT INTO sfd.risk_evidence_reviews VALUES (%s,%s,%s,%s,'no_contradiction',%s,%s,'audit','admin:test')",
            (f"hash{index}", SIM_BATCH, f"proof{index}", "a" * 64, PAST, PAST),
        )
    await dataset.connection.execute(
        "INSERT INTO sfd.risk_evidence_reviews "
        "VALUES ('verified',%s,'proof0',%s,'contradiction_verified',%s,%s,'audit','admin:test')",
        (SIM_BATCH, "a" * 64, PAST + timedelta(hours=1), CUTOFF),
    )
    target = (await dataset.checkout("target", buyer="buyer"))[0]
    later = (await dataset.checkout("later", buyer="buyer", at=CUTOFF + timedelta(seconds=1)))[0]
    for order, expected in ((target, "clear"), (later, "fired")):
        ctx = await context(dataset, order)
        assert ctx.evidence_count == 1
        assert ctx.evidence_reviews[0].independent_cases == 1
        facts = (await RiskRepository(dataset.connection).facts([order], "shop-a", CONFIG))[0]
        result = RuleEngine(CONFIG).evaluate(facts.features, ctx)
        assert result.adaptive
        assert next(s for s in result.adaptive.signals if s.rule_id == "SFD-A15").status == expected
        assert "private-ref" not in result.adaptive.model_dump_json()
