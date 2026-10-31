"""Score only the synthetic sale-burst fixture and verify its rule contract."""

import asyncio
import os
from collections import Counter

from app.db.postgres import Database
from app.repositories.risk_repository import RiskRepository
from app.services.scoring_service import ScoringService


async def main() -> None:
    if os.environ.get("ALLOW_DEMO_SCORE") != "true":
        raise SystemExit("Refusing demo scoring: set ALLOW_DEMO_SCORE=true explicitly")
    test_database_url = os.environ.get("BOOTSTRAP_TEST_DATABASE_URL")
    database_url = test_database_url or os.environ["DATABASE_URL"]
    database = Database(database_url)
    await database.open()
    try:
        async with database.pool.connection() as connection:
            repository = RiskRepository(connection)
            await connection.execute("SET LOCAL lock_timeout = '10s'")
            await connection.execute("SELECT pg_advisory_xact_lock(20261031, 2)")
            cursor = await connection.execute(
                "SELECT o.order_id FROM sfd.orders o "
                "JOIN sfd.data_batches batch ON batch.batch_id=o.batch_id AND batch.is_synthetic "
                "JOIN sfd.shops shop ON shop.shop_id=o.shop_id AND shop.batch_id=o.batch_id "
                "JOIN sfd.sellers seller ON seller.seller_id=shop.owner_seller_id "
                "AND seller.batch_id=shop.batch_id AND seller.status='active' "
                "JOIN public.users u ON seller.auth_subject='local:' || u.id "
                "AND u.username='seller_demo' AND u.status='ACTIVE' AND u.deleted_at IS NULL "
                "WHERE o.batch_id='olist_vn_sim' AND o.shop_id='live:bootstrap-shop' "
                "AND o.source_record_id LIKE 'sale-burst:v1:order:%' ORDER BY o.order_id"
            )
            rows = await cursor.fetchall()
            expected_ids = {f"live:sale-burst:order:{number:03d}" for number in range(1, 301)}
            if len(rows) != 300 or {row["order_id"] for row in rows} != expected_ids:
                raise RuntimeError("Expected exactly 300 synthetic sale orders owned by seller_demo")
            # CI provisions an empty database; deployed KBS owns active-rule initialization.
            if test_database_url:
                await repository.initialize_rules()
            assessments = await ScoringService(repository).score(
                [row["order_id"] for row in rows], "live:bootstrap-shop"
            )

            # Keep checks inside the transaction so any contract mismatch rolls back scoring.
            if Counter(item.risk_level for item in assessments) != {"low": 235, "medium": 5, "high": 60}:
                raise RuntimeError("Unexpected risk totals; scoring transaction rolled back")
            for item in assessments:
                number = int(item.order_id.rsplit(":", 1)[1])
                buyer_number = (number - 1) % 20 + 1
                buyer_order_number = (number - 1) // 20 + 1
                expected = "low"
                if buyer_number <= 5 and buyer_order_number == 3:
                    expected = "medium"
                elif buyer_number <= 5 and buyer_order_number >= 4:
                    expected = "high"
                if item.risk_level != expected:
                    raise RuntimeError(f"Unexpected risk level for {item.order_id}; transaction rolled back")

                if buyer_number <= 5 and buyer_order_number >= 11:
                    signals = {
                        signal["rule_id"]: signal["status"]
                        for signal in item.feature_snapshot.get("adaptive", {}).get("signals", [])
                    }
                    if signals.get("SFD-A08") != "fired" or signals.get("SFD-A04") != "fired":
                        raise RuntimeError(f"Missing A08/A04 for {item.order_id}; transaction rolled back")
    finally:
        await database.close()

    print("PASS: sale burst scored 235 Low / 5 Medium / 60 High with A08+A04 evidence")


if __name__ == "__main__":
    asyncio.run(main())
