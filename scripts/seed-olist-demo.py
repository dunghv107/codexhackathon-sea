"""Container-side helper for seed-olist-demo.sh; secrets come only from the environment."""

import json
import os
import sys
from datetime import datetime
from urllib.parse import quote

import httpx
import psycopg
from psycopg import sql


def import_dataset(connection: psycopg.Connection) -> None:
    manifest = json.loads(os.environ["OLIST_MANIFEST"])
    digest = os.environ["OLIST_SEED_SHA"]
    with connection.transaction():
        connection.execute("SET LOCAL lock_timeout = '10s'")
        connection.execute("SELECT pg_advisory_xact_lock(20261031, 2)")
        previous = connection.execute("SELECT seed_sha256 FROM public.olist_seed_imports").fetchall()
        if previous:
            if previous != [(digest,)]:
                raise RuntimeError("A different seed was imported; refusing to replace existing history")
            print("This seed was already imported; preserving all existing rows.", flush=True)
            return
        with psycopg.connect(os.environ["DATABASE_URL"], dbname=os.environ["OLIST_STAGE_DB"]) as source:
            source.execute("SET TRANSACTION READ ONLY")
            for table, count in manifest["tables"].items():
                relation = sql.Identifier("sfd", table)
                actual = source.execute(sql.SQL("SELECT count(*) FROM {}").format(relation)).fetchall()[0][0]
                if actual != count:
                    raise RuntimeError(f"Dataset count mismatch: {table}: {actual} != {count}")
                cursor = source.execute(sql.SQL("SELECT * FROM {} LIMIT 0").format(relation))
                if cursor.description is None:
                    raise RuntimeError(f"No source columns found for {table}")
                columns = sql.SQL(", ").join(sql.Identifier(column.name) for column in cursor.description)
                target = relation
                if table in ("data_batches", "rule_versions"):
                    target = sql.Identifier("seed_shared")
                    connection.execute(
                        sql.SQL("CREATE TEMP TABLE {} AS SELECT {} FROM {} WITH NO DATA").format(
                            target, columns, relation
                        )
                    )
                with (
                    source.cursor().copy(sql.SQL("COPY {} ({}) TO STDOUT").format(relation, columns)) as reader,
                    connection.cursor().copy(sql.SQL("COPY {} ({}) FROM STDIN").format(target, columns)) as writer,
                ):
                    for block in reader:
                        writer.write(block)
                if table in ("data_batches", "rule_versions"):
                    # Migration 5 / KBS startup can already own these keys; never replace their metadata.
                    key = "batch_id" if table == "data_batches" else "rule_version"
                    comparison = (
                        "t.is_synthetic IS DISTINCT FROM s.is_synthetic"
                        if table == "data_batches"
                        else (
                            "t.batch_id IS DISTINCT FROM s.batch_id OR t.config IS DISTINCT FROM s.config "
                            "OR t.config_sha256 IS DISTINCT FROM s.config_sha256"
                        )
                    )
                    conflict = connection.execute(
                        sql.SQL("SELECT 1 FROM {} t JOIN seed_shared s USING ({}) WHERE {} LIMIT 1").format(
                            relation, sql.Identifier(key), sql.SQL(comparison)
                        )
                    ).fetchone()
                    if conflict:
                        raise RuntimeError(f"Conflicting {table}; import rolled back")
                    connection.execute(
                        sql.SQL("INSERT INTO {} ({}) SELECT {} FROM seed_shared ON CONFLICT ({}) DO NOTHING").format(
                            relation, columns, columns, sql.Identifier(key)
                        )
                    )
                    connection.execute("DROP TABLE seed_shared")
                print(f"Imported {table}: {count}", flush=True)
        connection.execute(
            "INSERT INTO public.olist_seed_imports(seed_sha256,source_manifest) VALUES (%s,%s::jsonb)",
            (digest, os.environ["OLIST_MANIFEST"]),
        )
    print("Dataset transaction committed.", flush=True)


def verify_dataset(connection: psycopg.Connection) -> None:
    scenarios = json.loads(os.environ["OLIST_SCENARIOS"])
    expected = json.loads(os.environ["OLIST_EXPECTATIONS"])
    if len(scenarios) != len(expected["scenarios"]) or {item["scenario_id"] for item in scenarios} != set(
        expected["scenarios"]
    ):
        raise RuntimeError("Scenario list differs from the acceptance contract")
    orders = [scenario["order_id"] for scenario in scenarios]
    with connection.transaction():
        connection.execute("SET LOCAL lock_timeout = '10s'")
        connection.execute("SELECT pg_advisory_xact_lock(20261031, 1)")
        shops = connection.execute(
            "SELECT s.shop_id,s.owner_seller_id FROM sfd.shops s JOIN sfd.orders o USING (shop_id) "
            "WHERE o.order_id=ANY(%s) AND o.batch_id='olist_vn_sim' AND s.batch_id='olist_vn_sim' FOR UPDATE OF s",
            (orders,),
        ).fetchall()
        if any(owner not in (None, "live:bootstrap-seller") for _, owner in shops):
            raise RuntimeError("A scenario shop already has another owner; refusing to take it over")
        connection.execute(
            "UPDATE sfd.shops SET owner_seller_id='live:bootstrap-seller' WHERE shop_id=ANY(%s) "
            "AND batch_id='olist_vn_sim'",
            ([shop for shop, _ in shops],),
        )
    with httpx.Client(base_url="http://127.0.0.1:1114", timeout=120, trust_env=False) as client:
        for scenario in scenarios:
            order_id = scenario["order_id"]
            row = connection.execute(
                "SELECT shop_id FROM sfd.orders WHERE order_id=%s AND batch_id='olist_vn_sim'",
                (order_id,),
            ).fetchone()
            if row is None:
                raise RuntimeError(f"Missing scenario {scenario['scenario_id']}")
            path = f"/api/v1/orders/{quote(order_id, safe='')}"
            headers = {"X-Shop-ID": row[0]}
            response = client.post(path + "/score?force=true", headers=headers)
            response.raise_for_status()
            result = response.json()
            contract = expected["scenarios"][scenario["scenario_id"]]
            if (
                result["score"] != contract["score"]
                or result["risk_level"] != contract["risk_level"]
                or {reason["rule_id"] for reason in result["reasons"]} != set(contract["fired_rules"])
                or result["rule_version"] != expected["rule_version"]
                or datetime.fromisoformat(result["input_cutoff_at"])
                != datetime.fromisoformat(scenario["input_cutoff_at"])
            ):
                raise RuntimeError(f"KBS acceptance failed: {scenario['scenario_id']}; inspect its assessment")
            client.get(path + "/network", headers=headers).raise_for_status()
            print(
                f"PASS {scenario['scenario_id']}: {result['score']} {result['risk_level']} "
                f"| shop={row[0]} | order={order_id}",
                flush=True,
            )
    print(f"PASS: all {len(scenarios)} historical scenarios scored by KBS; Network endpoints responded.", flush=True)


def main() -> None:
    with psycopg.connect(os.environ["DATABASE_URL"], autocommit=True) as connection:
        mode = sys.argv[1]
        if mode == "preflight":
            connection.execute("SELECT owner_seller_id FROM sfd.shops LIMIT 0")
            connection.execute("SELECT verification_id FROM sfd.order_verifications LIMIT 0")
            with httpx.Client(timeout=15, trust_env=False) as client:
                client.get("http://127.0.0.1:1114/ready").raise_for_status()
                client.get("http://backend:8000/health").raise_for_status()
        elif mode == "import":
            import_dataset(connection)
        elif mode == "verify":
            verify_dataset(connection)
        else:
            raise RuntimeError("Unknown seed action")


if __name__ == "__main__":
    try:
        main()
    except (psycopg.Error, httpx.HTTPError) as error:
        # Database exception details can contain row contents or credentials.
        print(
            f"Seed failed ({type(error).__name__}); inspect database/service logs. No partial import was committed.",
            file=sys.stderr,
        )
        sys.exit(1)
