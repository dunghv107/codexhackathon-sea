"""Local HTTP smoke using a new disposable database and authored synthetic history.

Run with the KBS Python environment; build reports/backend-phone-demo for this OS first.
No dataset seed, CSV, existing records, or LLM are used. See docs/PHONE_NETWORK_CALL.md.
"""

import argparse
import base64
import hashlib
import hmac
import json
import os
import secrets
import socket
import subprocess
import sys
import time
from datetime import UTC, datetime, timedelta
from pathlib import Path
from typing import Any, LiteralString, cast

import httpx
import psycopg
from dotenv import dotenv_values
from psycopg import sql
from psycopg.conninfo import conninfo_to_dict, make_conninfo

# ruff: noqa: S101 -- assertions are the smoke-test oracle.

ROOT = Path(__file__).resolve().parents[1]
BATCH = "olist_vn_sim"


def free_port() -> int:
    with socket.socket() as sock:
        sock.bind(("127.0.0.1", 0))
        return sock.getsockname()[1]


def seed(conn: psycopg.Connection) -> int:
    """Six historical deliveries: low 0/1 failed, medium 2/2, high 3/3."""
    now = datetime.now(UTC)
    conn.execute("SET search_path TO sfd,public")
    conn.execute(
        "UPDATE sfd.data_batches SET source_system='authored_phone_smoke', source_reference='Isolated "
        "synthetic HTTP smoke' WHERE batch_id=%s",
        (BATCH,),
    )
    conn.execute(
        "INSERT INTO "
        "users(username,email,password,role,status,refresh_token_hash,refresh_token_expires_at) VALUES "
        "('phone-demo','phone-demo@demo.invalid','unusable','USER','ACTIVE','smoke',%s)",
        (now + timedelta(hours=1),),
    )
    user_row = conn.execute("SELECT id FROM users WHERE username='phone-demo'").fetchone()
    assert user_row is not None
    user_id = user_row[0]
    subject = f"local:{user_id}"
    conn.execute(
        "INSERT INTO buyers(buyer_id,auth_subject,account_created_at,batch_id,source_record_id) VALUES "
        "('live:demo:buyer',%s,%s,%s,'live:demo:buyer')",
        (subject, now - timedelta(days=100), BATCH),
    )
    conn.execute(
        "INSERT INTO sellers(seller_id,auth_subject,display_name,batch_id,source_record_id) VALUES "
        "('live:demo:seller',%s,'Demo seller',%s,'live:demo:seller')",
        (subject, BATCH),
    )
    for name in ("shop", "foreign"):
        shop = f"live:demo:{name}"
        conn.execute(
            "INSERT INTO shops(shop_id,display_name,owner_seller_id,batch_id,source_record_id) VALUES "
            "(%s,'Synthetic demo',%s,%s,%s)",
            (shop, "live:demo:seller" if name == "shop" else None, BATCH, shop),
        )
        conn.execute(
            "INSERT INTO products(product_id,batch_id,source_record_id) VALUES (%s,%s,%s)", (shop, BATCH, shop)
        )
        conn.execute(
            "INSERT INTO "
            "listings(listing_id,shop_id,product_id,current_price,currency,batch_id,source_record_id) "
            "VALUES (%s,%s,%s,100,'VND',%s,%s)",
            (shop, shop, shop, BATCH, shop),
        )
    for level, count, phone in (("low", 1, "+84900000001"), ("medium", 2, "+84900000002"), ("high", 3, "+84900000003")):
        key = f"live:demo:phone:{level}"
        for i in range(count):
            record = f"live:demo:history:{level}:{i}"
            placed = now - timedelta(days=12 + i)
            outcome = placed + timedelta(days=1)
            conn.execute(
                "INSERT INTO checkouts VALUES (%s,'live:demo:buyer',%s,'cod','VND',100,%s,%s)",
                (record, placed, BATCH, record),
            )
            conn.execute(
                "INSERT INTO checkout_identity_snapshots(checkout_id,phone_key,available_at,batch_id) "
                "VALUES (%s,%s,%s,%s)",
                (record, key, placed, BATCH),
            )
            conn.execute(
                "INSERT INTO orders(order_id,checkout_id,shop_id,batch_id,source_record_id) VALUES "
                "(%s,%s,'live:demo:foreign',%s,%s)",
                (record, record, BATCH, record),
            )
            conn.execute(
                "INSERT INTO order_items VALUES (%s,%s,'live:demo:foreign','live:demo:foreign',1,100,0,%s,%s)",
                (record, record, BATCH, record),
            )
            event = "delivery_succeeded" if level == "low" else "delivery_failed"
            conn.execute(
                "INSERT INTO order_events VALUES (%s,%s,%s,'carrier',NULL,%s,%s,%s,%s)",
                (record, record, event, outcome, outcome, BATCH, record),
            )
            if level != "low":
                conn.execute(
                    "INSERT INTO cases VALUES (%s,%s,'delivery',%s,%s,%s,%s)",
                    (record, record, outcome, outcome, BATCH, record),
                )
                conn.execute(
                    "INSERT INTO case_resolutions VALUES (%s,%s,'verified','buyer','Authored smoke "
                    "fixture','fixture',%s,%s,%s,%s)",
                    (record, record, outcome, outcome, BATCH, record),
                )
        conn.execute(
            "INSERT INTO demo_phone_mappings VALUES (%s,%s,%s,%s,'authored_phone_smoke; not S01/S03')",
            (BATCH, phone, key, level),
        )
    conn.commit()
    return user_id


def token(secret: str, user_id: int) -> str:
    def encode(value: dict[str, object]) -> bytes:
        return base64.urlsafe_b64encode(json.dumps(value).encode()).rstrip(b"=")

    body = (
        encode({"alg": "HS256", "typ": "JWT"})
        + b"."
        + encode({"user_id": user_id, "username": "phone-demo", "role": "user", "exp": int(time.time()) + 3600})
    )
    return (
        body + b"." + base64.urlsafe_b64encode(hmac.new(secret.encode(), body, hashlib.sha256).digest()).rstrip(b"=")
    ).decode()


def wait_ready(url: str, process: subprocess.Popen) -> None:
    for _ in range(120):
        if process.poll() is not None:
            raise RuntimeError("Service exited; inspect reports/phone-demo.log locally")
        try:
            if httpx.get(url, timeout=1).status_code == 200:
                return
        except httpx.HTTPError:
            pass
        time.sleep(0.25)
    raise RuntimeError("Service startup timed out")


def check_http(base: str, auth: str, conn: psycopg.Connection) -> None:
    with httpx.Client(base_url=base, headers={"Authorization": f"Bearer {auth}"}, timeout=15) as client:

        def call(
            path: str, method: str = "GET", body: dict[str, Any] | None = None, status: int = 200
        ) -> dict[str, Any]:
            result = client.request(method, path, json=body)
            assert result.status_code == status, (path, result.status_code, result.text[:300])
            return result.json().get("data")

        address = call(
            "/api/buyer/addresses",
            "POST",
            {"label": "Demo", "address_raw": "12 Demo", "region": "Demo", "country_code": "VN", "is_default": True},
            201,
        )

        def checkout(phone: str, method: str = "cod") -> tuple[str, str, dict[str, Any]]:
            result = call(
                "/api/buyer/checkouts",
                "POST",
                {
                    "phone": phone,
                    "saved_address_id": address["saved_address_id"],
                    "payment_method": method,
                    "items": [{"listing_id": "live:demo:shop", "quantity": 1}],
                },
                201,
            )
            assert not any(word in json.dumps(result) for word in ("risk_level", "phone_key", "verification"))
            order = result["orders"][0]["order_id"]
            path = f"/api/seller/shops/live:demo:shop/orders/{order}"
            return order, path, call(path)

        paths = {}
        for level, phone, score in (("low", "0900000001", 0), ("medium", "0900000002", 30), ("high", "0900000003", 70)):
            order, path, detail = checkout(phone)
            assessment = detail["latest_assessment"]
            assert assessment
            assert (assessment["risk_level"], assessment["score"]) == (level, score), detail
            assert detail["phone_masked"] == "***" + phone[-4:]
            assert (detail["verification"] is not None) == (level == "medium")
            assert detail["review_status"] == ("hold" if level == "high" else "pending")
            network = call(path + "/network")
            assert len(network["nodes"]) <= 200
            assert "live:demo:history" not in json.dumps(network)
            assert "live:demo:phone" not in json.dumps(network)
            assert any(n["kind"] == "aggregate" for n in network["nodes"])
            call(path.replace("live:demo:shop", "live:demo:foreign") + "/network", status=404)
            assert httpx.get(base + path + "/network").status_code == 401
            paths[level] = (order, path, detail)
            print(f"{level}: {score}, network redacted, verification eligibility OK", flush=True)
        # Validate no writes on malformed phone.
        count = conn.execute("SELECT count(*) FROM checkouts").fetchone()
        call(
            "/api/buyer/checkouts",
            "POST",
            {
                "phone": "invalid",
                "saved_address_id": address["saved_address_id"],
                "payment_method": "cod",
                "items": [{"listing_id": "live:demo:shop", "quantity": 1}],
            },
            400,
        )
        assert conn.execute("SELECT count(*) FROM checkouts").fetchone() == count
        # Seller retry recovers missing verification after a hypothetical post-score failure.
        order, path, detail = paths["medium"]
        conn.execute("DELETE FROM order_verifications WHERE order_id=%s", (order,))
        conn.commit()
        detail = call(path + "/score", "POST")
        assert detail["verification"]
        conn.execute("UPDATE sellers SET status='suspended' WHERE seller_id='live:demo:seller'")
        conn.commit()
        call(
            path + "/verification/respond",
            "POST",
            {"verification_id": detail["verification"]["verification_id"], "key": "1"},
            403,
        )
        call(path + "/verification")
        conn.execute("UPDATE sellers SET status='active' WHERE seller_id='live:demo:seller'")
        conn.commit()
        for i in range(2):
            if i:
                order, path, detail = checkout("0900000002")
            before = detail["latest_assessment"]
            body = {"verification_id": detail["verification"]["verification_id"], "key": "1"}
            assert call(path + "/verification/respond", "POST", body)["status"] == "confirmed"
            call(path + "/verification/respond", "POST", body)
            call(path + "/verification/respond", "POST", {**body, "key": "2"}, 409)
            after = call(path)
            assert after["latest_assessment"] == before
            assert after["review_status"] == "pending"
            buyer = call(f"/api/buyer/orders/{order}")
            assert buyer["display_status"] == "confirming"
            assert "verification" not in json.dumps(buyer)
            assert conn.execute(
                "SELECT count(*) FROM order_events WHERE order_id=%s AND event_type='confirmed'", (order,)
            ).fetchone() == (1,)
            call(
                f"/api/buyer/orders/{order}/cancel",
                "POST",
                {"reason": "Synthetic buyer cancellation after confirmation"},
            )
        _, _, later = checkout("0900000002")
        fired = {
            r["rule_id"]: r["contribution"] for r in later["latest_assessment"]["reasons"] if r["contribution"] > 0
        }
        assert fired.get("R10") == 15, fired
        print("confirm -> buyer cancel twice -> later R10 +15; existing assessment unchanged", flush=True)
        _, _, prepaid = checkout("0900000002", "credit_card")
        assert prepaid["verification"] is None
        _, _, unknown = checkout("0900000009")
        assert unknown["latest_assessment"]["risk_level"] == "low"
        phone_row = conn.execute(
            "SELECT phone_key FROM checkout_identity_snapshots WHERE checkout_id=%s", (unknown["checkout_id"],)
        ).fetchone()
        assert phone_row is not None
        assert phone_row[0].startswith("hmac:v1:")
        print("prepaid excluded, unknown phone HMAC, malformed phone rollback: OK", flush=True)


def main() -> None:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--env-file", default="kbs/.env.local-smoke.local")
    args = parser.parse_args()
    values = dotenv_values(ROOT / args.env_file)
    source = values.get("DATABASE_URL")
    if not source:
        raise SystemExit("Private env file must contain local DATABASE_URL with CREATEDB rights")
    config = conninfo_to_dict(source)
    if config.get("host") not in ("localhost", "127.0.0.1", "::1"):
        raise SystemExit("Only loopback PostgreSQL is supported")
    name = "phone_smoke_" + secrets.token_hex(6)
    admin = psycopg.connect(source, autocommit=True)
    admin.execute(sql.SQL("CREATE DATABASE {}").format(sql.Identifier(name)))
    processes = []
    conn = None
    log_path = ROOT / "reports/phone-demo.log"
    log_path.parent.mkdir(exist_ok=True)
    try:
        dsn = make_conninfo(source, dbname=name)
        conn = psycopg.connect(dsn)
        for file in (
            "olist_cod/olist_cod/schema.sql",
            "backend/migrations/000001_create_users.up.sql",
            "backend/migrations/000004_align_olist_cod.up.sql",
            "backend/migrations/000005_demo_scoring_batch.up.sql",
            "backend/migrations/000006_phone_verifications.up.sql",
        ):
            # Fixed repository DDL paths, not user-supplied SQL.
            conn.execute(sql.SQL(cast(LiteralString, (ROOT / file).read_text(encoding="utf-8"))))
        conn.commit()
        user_id = seed(conn)
        kbs_port, go_port = free_port(), free_port()
        jwt_secret = secrets.token_hex(32)
        env: dict[str, str] = {
            **os.environ,
            "ENV_FILE": "/dev/null",
            "DATABASE_URL": dsn,
            "KBS_EXPLAIN_ENABLED": "false",
            "KBS_NEO4J_ENABLED": "false",
            "SERVER_PORT": str(kbs_port),
            "PORT": str(go_port),
            "ENV": "production",
            "DB_HOST": str(config.get("host") or "127.0.0.1"),
            "DB_PORT": str(config.get("port") or "5432"),
            "DB_USER": str(config.get("user") or "postgres"),
            "DB_PASSWORD": str(config.get("password") or ""),
            "DB_NAME": name,
            "DB_SSLMODE": "disable",
            "JWT_SECRET": jwt_secret,
            "PHONE_HMAC_SECRET": secrets.token_hex(32),
            "DEMO_MODE": "true",
            "RATE_LIMIT_ENABLED": "false",
            "KBS_URL": f"http://127.0.0.1:{kbs_port}",
            "LOG_FILE_PATH": str(ROOT / "reports/phone-demo-go.log"),
        }
        with log_path.open("w", encoding="utf-8") as log:
            kbs = subprocess.Popen(  # noqa: S603 -- fixed local executables and argument arrays, no shell.
                [
                    sys.executable,
                    "-m",
                    "uvicorn",
                    "app.api.app:create_app",
                    "--factory",
                    "--host",
                    "127.0.0.1",
                    "--port",
                    str(kbs_port),
                ],
                cwd=ROOT / "kbs",
                env=env,
                stdout=log,
                stderr=log,
            )
            processes.append(kbs)
            wait_ready(f"http://127.0.0.1:{kbs_port}/api/v1/health", kbs)
            backend = subprocess.Popen(  # noqa: S603 -- fixed local executables and argument arrays, no shell.
                [str(ROOT / "reports/backend-phone-demo")], cwd=ROOT / "backend", env=env, stdout=log, stderr=log
            )
            processes.append(backend)
            wait_ready(f"http://127.0.0.1:{go_port}/health", backend)
            check_http(f"http://127.0.0.1:{go_port}", token(jwt_secret, user_id), conn)
        logs = log_path.read_text(encoding="utf-8")
        assert not any(
            phone in logs
            for phone in ("0900000001", "+84900000001", "0900000002", "+84900000002", "live:demo:phone:", "hmac:v1:")
        ), "Phone identity leaked into logs"
        print("HTTP smoke PASS; disposing only this run's database", flush=True)
    finally:
        for process in reversed(processes):
            process.terminate()
            try:
                process.wait(timeout=10)
            except subprocess.TimeoutExpired:
                process.kill()
                process.wait()
        if conn:
            conn.close()
        admin.execute(sql.SQL("DROP DATABASE {} WITH (FORCE)").format(sql.Identifier(name)))
        admin.close()


if __name__ == "__main__":
    main()
