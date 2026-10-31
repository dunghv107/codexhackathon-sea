"""Select demo identities with KBS, then verify real Go checkouts. Never alter source history."""

import asyncio
import json
import os
import sys
from typing import Any
from urllib.parse import quote
from uuid import uuid4

import httpx
from psycopg import AsyncConnection, Error
from psycopg.rows import dict_row

from app.core.kbs_config import SIM_BATCH
from app.repositories.risk_repository import RiskRepository
from app.rules.engine import RuleEngine

PHONES = {"low": "+84900000001", "medium": "+84900000002", "high": "+84900000003"}
SHOP = "live:bootstrap-shop"
BUYER = "live:bootstrap-buyer"
LISTING = "live:bootstrap-listing-1"
ADDRESS = "live:bootstrap-address"


async def choose_keys(conn: AsyncConnection[dict[str, Any]]) -> dict[str, str]:
    repository = RiskRepository(conn)
    config = await repository.configuration()
    engine = RuleEngine(config)
    cursor = await conn.execute(
        "SELECT phone_e164,phone_key FROM sfd.demo_phone_mappings WHERE batch_id=%s AND phone_e164=ANY(%s)",
        (SIM_BATCH, list(PHONES.values())),
    )
    existing = {row["phone_e164"]: row["phone_key"] for row in await cursor.fetchall()}
    # These are search hints only. The repository's point-in-time facts decide eligibility below.
    cursor = await conn.execute(
        "SELECT s.phone_key FROM sfd.checkout_identity_snapshots s "
        "JOIN sfd.orders o ON o.checkout_id=s.checkout_id AND o.batch_id=%s "
        "JOIN sfd.order_events e ON e.order_id=o.order_id AND e.batch_id=%s "
        "WHERE s.batch_id=%s AND s.phone_key IS NOT NULL AND s.available_at<now() AND e.event_type='delivery_failed' "
        "AND e.available_at<now() AND e.occurred_at<now() "
        "GROUP BY s.phone_key HAVING count(DISTINCT s.checkout_id)>=2 "
        "ORDER BY count(DISTINCT s.checkout_id),s.phone_key",
        (SIM_BATCH, SIM_BATCH, SIM_BATCH),
    )
    candidates = [row["phone_key"] for row in await cursor.fetchall()]
    scenarios = {item["scenario_id"]: item for item in json.loads(os.environ["OLIST_SCENARIOS"])}
    preferred = {}
    for level, scenario in (("low", "S01"), ("high", "S03")):
        cursor = await conn.execute(
            "SELECT phone_key FROM sfd.checkout_identity_snapshots "
            "WHERE batch_id=%s AND checkout_id=%s AND available_at<now()",
            (SIM_BATCH, scenarios[scenario]["checkout_id"]),
        )
        row = await cursor.fetchone()
        preferred[level] = row["phone_key"] if row else None

    chosen: dict[str, str] = {}
    # One ephemeral target, always rolled back; failed candidates never create live orders or assessments.
    async with conn.transaction(force_rollback=True):
        probe = f"live:{uuid4()}"
        await conn.execute(
            "INSERT INTO sfd.checkouts(checkout_id,buyer_id,placed_at,payment_method,currency,total_amount,"
            "batch_id,source_record_id) SELECT %s,%s,now(),'cod',currency,current_price,%s,%s "
            "FROM sfd.listings WHERE listing_id=%s AND shop_id=%s AND batch_id=%s AND current_price IS NOT NULL",
            (probe, BUYER, SIM_BATCH, probe, LISTING, SHOP, SIM_BATCH),
        )
        await conn.execute(
            "INSERT INTO sfd.orders(order_id,checkout_id,shop_id,batch_id,source_record_id) VALUES (%s,%s,%s,%s,%s)",
            (probe, probe, SHOP, SIM_BATCH, probe),
        )
        await conn.execute(
            "INSERT INTO sfd.order_items(item_id,order_id,shop_id,listing_id,quantity,unit_price,freight_amount,"
            "batch_id,source_record_id) SELECT %s,%s,shop_id,listing_id,1,current_price,0,batch_id,%s "
            "FROM sfd.listings WHERE listing_id=%s AND batch_id=%s",
            (probe, probe, probe, LISTING, SIM_BATCH),
        )
        await conn.execute(
            "INSERT INTO sfd.checkout_identity_snapshots(checkout_id,available_at,batch_id) VALUES (%s,now(),%s)",
            (probe, SIM_BATCH),
        )
        for level, phone in PHONES.items():
            keys = [existing[phone]] if phone in existing else [preferred.get(level), *candidates]
            for key in dict.fromkeys(keys):
                if not key or key in chosen.values():
                    continue
                await conn.execute(
                    "UPDATE sfd.checkout_identity_snapshots SET phone_key=%s WHERE checkout_id=%s AND batch_id=%s",
                    (key, probe, SIM_BATCH),
                )
                facts = (await repository.facts([probe], SHOP, config))[0]
                result = engine.evaluate(facts.features)
                if result.risk_level != level:
                    continue
                if level == "medium" and (
                    facts.features["R1"]["resolved"] != 2
                    or facts.features["R1"]["failed"] != 2
                    or {reason.rule_id for reason in result.reasons} != {"R1"}
                ):
                    continue
                chosen[level] = key
                break
            if level not in chosen:
                suffix = "Existing mapping was preserved." if phone in existing else "No mapping was written."
                raise RuntimeError(
                    f"Could not find a {level} phone at the current cutoff. {suffix} "
                    "Source history and scoring rules were not changed."
                )
    return chosen


async def save_mappings(conn: AsyncConnection[dict[str, Any]], keys: dict[str, str]) -> None:
    async with conn.transaction():
        for level, phone in PHONES.items():
            await conn.execute(
                "INSERT INTO sfd.demo_phone_mappings(batch_id,phone_e164,phone_key,scenario_label,source_reference) "
                "VALUES (%s,%s,%s,%s,%s) ON CONFLICT (batch_id,phone_e164) DO NOTHING",
                (
                    SIM_BATCH,
                    phone,
                    keys[level],
                    level,
                    f"synthetic demo mapping; Olist seed {os.environ['OLIST_SEED_SHA']}",
                ),
            )
            cursor = await conn.execute(
                "SELECT phone_key FROM sfd.demo_phone_mappings WHERE batch_id=%s AND phone_e164=%s FOR UPDATE",
                (SIM_BATCH, phone),
            )
            row = await cursor.fetchone()
            if row is None or row["phone_key"] != keys[level]:
                raise RuntimeError(f"Conflicting {level} mapping; mappings rolled back without replacing identities")


async def login(client: httpx.AsyncClient, username: str, password_variable: str) -> None:
    response = await client.post(
        "/api/auth/login", json={"username": username, "password": os.environ[password_variable]}
    )
    if response.status_code != 200:
        raise RuntimeError(f"Cannot log in as {username}. Use the existing bootstrap password and an active account.")
    client.headers["Authorization"] = f"Bearer {response.json()['data']['access_token']}"


async def verify_checkouts(
    conn: AsyncConnection[dict[str, Any]],
    keys: dict[str, str],
    buyer: httpx.AsyncClient,
    seller: httpx.AsyncClient,
) -> None:
    for level, phone in PHONES.items():
        # Recover an equivalent checkout even if a previous HTTP response was lost; never reset seller decisions.
        cursor = await conn.execute(
            "SELECT o.order_id FROM sfd.orders o "
            "JOIN sfd.checkouts c ON c.checkout_id=o.checkout_id AND c.batch_id=%s "
            "JOIN sfd.checkout_contacts p ON p.checkout_id=c.checkout_id AND p.batch_id=%s "
            "JOIN sfd.checkout_identity_snapshots s ON s.checkout_id=c.checkout_id AND s.batch_id=%s "
            "WHERE o.batch_id=%s AND o.shop_id=%s AND c.buyer_id=%s AND c.payment_method='cod' "
            "AND p.phone_e164=%s AND p.key_source='demo_mapping' AND s.phone_key=%s "
            "AND (SELECT count(*) FROM sfd.orders other WHERE other.checkout_id=c.checkout_id AND other.batch_id=%s)=1 "
            "AND (SELECT count(*) FROM sfd.order_items i WHERE i.order_id=o.order_id AND i.batch_id=%s)=1 "
            "AND EXISTS (SELECT 1 FROM sfd.order_items i WHERE i.order_id=o.order_id AND i.batch_id=%s "
            "AND i.listing_id=%s AND i.quantity=1) ORDER BY c.placed_at DESC,o.order_id LIMIT 1",
            (
                SIM_BATCH,
                SIM_BATCH,
                SIM_BATCH,
                SIM_BATCH,
                SHOP,
                BUYER,
                phone,
                keys[level],
                SIM_BATCH,
                SIM_BATCH,
                SIM_BATCH,
                LISTING,
            ),
        )
        previous = await cursor.fetchone()
        if previous:
            order = previous["order_id"]
        else:
            response = await buyer.post(
                "/api/buyer/checkouts",
                json={
                    "phone": phone,
                    "saved_address_id": ADDRESS,
                    "payment_method": "cod",
                    "items": [{"listing_id": LISTING, "quantity": 1}],
                },
            )
            response.raise_for_status()
            order = response.json()["data"]["orders"][0]["order_id"]
        path = f"/api/seller/shops/{quote(SHOP, safe='')}/orders/{quote(order, safe='')}"
        response = await seller.get(path)
        response.raise_for_status()
        detail = response.json()["data"]
        if not detail.get("latest_assessment") or (level == "medium" and not detail.get("verification")):
            response = await seller.post(path + "/score")
            response.raise_for_status()
            detail = response.json()["data"]
        assessment = detail.get("latest_assessment")
        if not assessment or assessment["risk_level"] != level:
            raise RuntimeError(f"Actual {level} checkout has a different risk level; inspect order {order}")
        if level == "medium" and (
            not detail.get("verification")
            or detail["verification"]["assessment_id"] != assessment["assessment_id"]
            or {reason["rule_id"] for reason in assessment["reasons"]} != {"R1"}
        ):
            raise RuntimeError(f"Medium checkout lacks R1-only scoring or call verification; inspect order {order}")
        cursor = await conn.execute(
            "SELECT p.key_source,s.phone_key FROM sfd.orders o "
            "JOIN sfd.checkout_contacts p ON p.checkout_id=o.checkout_id AND p.batch_id=%s "
            "JOIN sfd.checkout_identity_snapshots s ON s.checkout_id=o.checkout_id AND s.batch_id=%s "
            "WHERE o.order_id=%s AND o.batch_id=%s",
            (SIM_BATCH, SIM_BATCH, order, SIM_BATCH),
        )
        identity = await cursor.fetchone()
        if not identity or identity["key_source"] != "demo_mapping" or identity["phone_key"] != keys[level]:
            raise RuntimeError("Go did not use the selected mapping. Enable DEMO_MODE=true and recreate backend.")
        if detail.get("phone_masked") != "***" + phone[-4:]:
            raise RuntimeError("Seller phone masking did not match the demo contact")
        print(f"PASS demo {level}: {phone}, score={assessment['score']}, shop={SHOP}, order={order}", flush=True)


async def main() -> None:
    async with (
        await AsyncConnection[dict[str, Any]].connect(
            os.environ["DATABASE_URL"], autocommit=True, row_factory=dict_row
        ) as conn,
        httpx.AsyncClient(base_url="http://backend:8000", timeout=120, trust_env=False) as buyer,
        httpx.AsyncClient(base_url="http://backend:8000", timeout=120, trust_env=False) as seller,
    ):
        cursor = await conn.execute("SELECT pg_try_advisory_lock(20261031,3) AS acquired")
        lock = await cursor.fetchone()
        if not lock or not lock["acquired"]:
            raise RuntimeError("Another demo phone setup is running; retry after it finishes")
        await login(buyer, "buyer_demo", "BOOTSTRAP_BUYER_PASSWORD")
        await login(seller, "seller_demo", "BOOTSTRAP_SELLER_PASSWORD")
        keys = await choose_keys(conn)
        await save_mappings(conn, keys)
        await verify_checkouts(conn, keys, buyer, seller)


if __name__ == "__main__":
    try:
        asyncio.run(main())
    except RuntimeError as error:
        print(f"Demo phone setup failed: {error}", file=sys.stderr)
        sys.exit(1)
    except (Error, httpx.HTTPError) as error:
        print(
            f"Demo phone setup failed ({type(error).__name__}). Inspect service logs; "
            "committed checkouts are kept and reused on rerun.",
            file=sys.stderr,
        )
        sys.exit(1)
