"""Run with DATABASE_URL configured: uv run python -m app.services.backfill."""

import asyncio
from collections import Counter

from app.core.kbs_config import SIM_BATCH, get_kbs_settings
from app.db.postgres import Database
from app.repositories.risk_repository import RiskRepository
from app.services.scoring_service import ScoringService


async def main() -> None:
    url = get_kbs_settings().database_url
    if url is None:
        raise ValueError("DATABASE_URL is required")
    database = Database(url.get_secret_value())
    await database.open()
    counts: Counter[str] = Counter()
    try:
        after = ""
        while True:
            async with database.pool.connection() as connection:
                cursor = await connection.execute(
                    "SELECT order_id, shop_id FROM sfd.orders WHERE batch_id = %s AND order_id > %s "
                    "ORDER BY order_id LIMIT 1000",
                    (SIM_BATCH, after),
                )
                rows = await cursor.fetchall()
                if not rows:
                    break
                groups: dict[str, list[str]] = {}
                for row in rows:
                    groups.setdefault(row["shop_id"], []).append(row["order_id"])
                service = ScoringService(RiskRepository(connection))
                for shop_id, order_ids in groups.items():
                    counts.update(result.risk_level for result in await service.score(order_ids, shop_id))
                after = rows[-1]["order_id"]
            print(dict(counts), flush=True)
    finally:
        await database.close()


if __name__ == "__main__":
    asyncio.run(main())
