import hashlib
import os
from collections.abc import AsyncIterator
from datetime import UTC, datetime, timedelta
from decimal import Decimal
from typing import Any

import pytest
from psycopg import AsyncConnection
from psycopg.rows import dict_row
from psycopg.types.json import Jsonb

from app.core.kbs_config import RULES_PATH, SIM_BATCH
from app.rules.config import RuleConfig

CUTOFF = datetime(2026, 10, 25, 10, tzinfo=UTC)


class Dataset:
    def __init__(self, connection: AsyncConnection[dict[str, Any]]) -> None:
        self.connection = connection

    async def checkout(
        self,
        name: str,
        *,
        phone: str | None = "phone",
        device: str | None = "device",
        cluster: str | None = "cluster",
        buyer: str | None = None,
        age: int = 100,
        at: datetime = CUTOFF,
        shops: tuple[str, ...] = ("shop-a",),
        amount: int | Decimal = 100,
        batch: str = SIM_BATCH,
    ) -> list[str]:
        buyer = buyer or name
        conn = self.connection
        await conn.execute(
            "INSERT INTO sfd.buyers(buyer_id,account_created_at,batch_id,source_record_id) "
            "VALUES (%s,%s,%s,%s) ON CONFLICT DO NOTHING",
            (buyer, at - timedelta(days=age), batch, buyer),
        )
        await conn.execute(
            "INSERT INTO sfd.checkouts VALUES (%s,%s,%s,'cod','VND',%s,%s,%s)",
            (name, buyer, at, amount * len(shops), batch, name),
        )
        await conn.execute(
            "INSERT INTO sfd.checkout_identity_snapshots VALUES (%s,%s,%s,%s,%s,%s)",
            (name, phone, device, cluster, at, batch),
        )
        orders = []
        for shop in shops:
            order = f"{name}:{shop}"
            await conn.execute(
                "INSERT INTO sfd.orders(order_id,checkout_id,shop_id,batch_id,source_record_id) "
                "VALUES (%s,%s,%s,%s,%s)",
                (order, name, shop, batch, order),
            )
            await conn.execute(
                "INSERT INTO sfd.order_items VALUES (%s,%s,%s,%s,1,%s,0,%s,%s)",
                (order, order, shop, f"listing:{shop}", amount, batch, order),
            )
            orders.append(order)
        return orders

    async def outcome(
        self,
        order: str,
        *,
        party: str = "buyer",
        failed: bool = True,
        at: datetime = CUTOFF - timedelta(days=1),
        available: datetime | None = None,
        batch: str = SIM_BATCH,
    ) -> None:
        available = available or at
        await self.connection.execute(
            "INSERT INTO sfd.order_events VALUES (%s,%s,%s,'carrier',NULL,%s,%s,%s,%s)",
            (
                f"event:{order}",
                order,
                "delivery_failed" if failed else "delivery_succeeded",
                at,
                available,
                batch,
                order,
            ),
        )
        if failed:
            await self.connection.execute(
                "INSERT INTO sfd.cases VALUES (%s,%s,'delivery',%s,%s,%s,%s)",
                (order, order, at, available, batch, order),
            )
            await self.connection.execute(
                "INSERT INTO sfd.case_resolutions VALUES (%s,%s,'verified',%s,'fixture','fixture',%s,%s,%s,%s)",
                (order, order, party, at, available, batch, order),
            )


@pytest.fixture
async def dataset() -> AsyncIterator[Dataset]:
    url = os.getenv("KBS_TEST_DATABASE_URL")
    if not url:
        pytest.skip("Set KBS_TEST_DATABASE_URL to an isolated database with the sfd schema")
    async with (
        await AsyncConnection[dict[str, Any]].connect(url, row_factory=dict_row, autocommit=True) as connection,
        connection.transaction(force_rollback=True),
    ):
        await connection.execute("SET LOCAL statement_timeout = '15s'")
        for batch in (SIM_BATCH, "olist_raw"):
            await connection.execute(
                "INSERT INTO sfd.data_batches VALUES (%s,'fixture',true,'fixture',NULL,%s)",
                (batch, CUTOFF),
            )
        for shop, batch in (
            ("shop-a", SIM_BATCH),
            ("shop-b", SIM_BATCH),
            ("shop-c", SIM_BATCH),
            ("raw-shop", "olist_raw"),
        ):
            await connection.execute(
                "INSERT INTO sfd.shops VALUES (%s,%s,NULL,%s,%s)",
                (shop, shop, batch, shop),
            )
            await connection.execute(
                "INSERT INTO sfd.products(product_id,batch_id,source_record_id) VALUES (%s,%s,%s)",
                (shop, batch, shop),
            )
            await connection.execute(
                "INSERT INTO sfd.listings VALUES (%s,%s,%s,100,'VND',NULL,%s,%s)",
                (f"listing:{shop}", shop, shop, batch, shop),
            )
        config = RuleConfig.load(RULES_PATH)
        await connection.execute(
            "INSERT INTO sfd.rule_versions VALUES (%s,%s,%s,%s,%s,'fixture')",
            (
                SIM_BATCH,
                config.version,
                Jsonb(config.model_dump()),
                hashlib.sha256(RULES_PATH.read_bytes()).hexdigest(),
                CUTOFF - timedelta(days=365),
            ),
        )
        yield Dataset(connection)
