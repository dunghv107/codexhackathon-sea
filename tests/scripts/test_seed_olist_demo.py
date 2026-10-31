import contextlib
import importlib.util
import io
import json
import os
from pathlib import Path
import threading
import time
import unittest
from unittest.mock import MagicMock, call, patch
import uuid

import httpx
import psycopg
from psycopg import sql


SPEC = importlib.util.spec_from_file_location(
    "seed_olist", Path(__file__).resolve().parents[2] / "scripts/seed-olist-demo.py"
)
seed = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(seed)


class SeedHelperTests(unittest.TestCase):
    def test_preflight_rejects_different_database_before_health_or_mutation(self):
        connection = MagicMock()
        connection.execute.return_value.fetchone.return_value = ("another_database",)
        with (
            patch.dict(os.environ, {"DATABASE_URL": "test-dsn", "OLIST_TARGET_DB": "expected"}),
            patch.object(seed.sys, "argv", ["seed", "preflight"]),
            patch.object(seed.psycopg, "connect") as connect,
            patch.object(seed.httpx, "Client") as client,
        ):
            connect.return_value.__enter__.return_value = connection
            with self.assertRaisesRegex(RuntimeError, "KBS database differs"):
                seed.main()
            connection.execute.assert_called_once_with("SELECT current_database()")
            client.assert_not_called()

    def test_preflight_matching_database_checks_schema_and_service(self):
        connection = MagicMock()
        connection.execute.return_value.fetchone.return_value = ("expected",)
        with (
            patch.dict(os.environ, {"DATABASE_URL": "test-dsn", "OLIST_TARGET_DB": "expected"}),
            patch.object(seed.sys, "argv", ["seed", "preflight"]),
            patch.object(seed.psycopg, "connect") as connect,
            patch.object(seed.httpx, "Client") as client,
        ):
            connect.return_value.__enter__.return_value = connection
            seed.main()
            self.assertEqual(3, connection.execute.call_count)
            self.assertEqual(
                client.return_value.__enter__.return_value.get.call_args_list,
                [call("http://127.0.0.1:1114/ready"), call("http://backend:8000/health")],
            )

    def test_scenario_validation_never_partially_assigns_shops(self):
        scenarios = [
            {"scenario_id": "S1", "order_id": "order1"},
            {"scenario_id": "S2", "order_id": "order2"},
        ]
        environment = {
            "OLIST_SCENARIOS": json.dumps(scenarios),
            "OLIST_EXPECTATIONS": json.dumps({"scenarios": {"S1": {}, "S2": {}}}),
        }
        for rows in (
            [("order1", "shop1", None)],
            [("order1", "shop1", None), ("order2", "shop2", "another-seller")],
        ):
            with (
                self.subTest(rows=rows),
                patch.dict(os.environ, environment),
                patch.object(seed.httpx, "Client") as client,
            ):
                connection = MagicMock()
                connection.execute.return_value.fetchall.return_value = rows
                with self.assertRaises(RuntimeError):
                    seed.verify_dataset(connection)
                self.assertFalse(
                    any(str(call.args[0]).startswith("UPDATE") for call in connection.execute.call_args_list)
                )
                client.assert_not_called()
                self.assertEqual(
                    RuntimeError,
                    connection.transaction.return_value.__exit__.call_args.args[0],
                )

    def test_duplicate_scenario_orders_rejected_before_database_access(self):
        scenarios = [
            {"scenario_id": "S1", "order_id": "same"},
            {"scenario_id": "S2", "order_id": "same"},
        ]
        with patch.dict(
            os.environ,
            {
                "OLIST_SCENARIOS": json.dumps(scenarios),
                "OLIST_EXPECTATIONS": json.dumps({"scenarios": {"S1": {}, "S2": {}}}),
            },
        ):
            connection = MagicMock()
            with self.assertRaisesRegex(RuntimeError, "must be unique"):
                seed.verify_dataset(connection)
            connection.transaction.assert_not_called()

    def test_matching_receipt_skips_and_different_receipt_refuses_import(self):
        for previous, rejected in (("a" * 64, False), ("b" * 64, True)):
            with (
                self.subTest(rejected=rejected),
                patch.dict(
                    os.environ,
                    {"OLIST_MANIFEST": '{"tables":{}}', "OLIST_SEED_SHA": "a" * 64},
                ),
                patch.object(seed.psycopg, "connect") as source,
                contextlib.redirect_stdout(io.StringIO()),
            ):
                connection = MagicMock()
                connection.execute.return_value.fetchall.return_value = [(previous,)]
                if rejected:
                    with self.assertRaisesRegex(RuntimeError, "different seed"):
                        seed.import_dataset(connection)
                else:
                    seed.import_dataset(connection)
                source.assert_not_called()

    def test_http_failure_does_not_claim_earlier_import_was_rolled_back_or_leak_url(
        self,
    ):
        output = io.StringIO()
        with (
            patch.object(
                seed,
                "main",
                side_effect=httpx.ConnectError("private credentials in upstream error"),
            ),
            contextlib.redirect_stderr(output),
        ):
            self.assertEqual(1, seed.run())
        self.assertIn("Earlier successful steps remain committed", output.getvalue())
        self.assertNotIn("private credentials", output.getvalue())
        self.assertNotIn("No partial import was committed", output.getvalue())


@unittest.skipUnless(
    os.environ.get("SEED_TEST_DATABASE_URL"),
    "requires isolated PostgreSQL with CREATEDB",
)
class SeedImportDatabaseTests(unittest.TestCase):
    def setUp(self):
        self.dsn = os.environ["SEED_TEST_DATABASE_URL"]
        self.admin = psycopg.connect(self.dsn, autocommit=True)
        self.addCleanup(self.admin.close)
        suffix = uuid.uuid4().hex
        self.target_name, self.stage_name = (
            "seed_helper_target_" + suffix,
            "seed_helper_stage_" + suffix,
        )
        for name in (self.target_name, self.stage_name):
            self.admin.execute(sql.SQL("CREATE DATABASE {}").format(sql.Identifier(name)))
            self.addCleanup(
                lambda name=name: self.admin.execute(
                    sql.SQL("DROP DATABASE {} WITH (FORCE)").format(sql.Identifier(name))
                )
            )
        self.target = psycopg.connect(self.dsn, dbname=self.target_name, autocommit=True)
        self.source = psycopg.connect(self.dsn, dbname=self.stage_name, autocommit=True)
        self.addCleanup(self.target.close)
        self.addCleanup(self.source.close)
        for connection in (self.target, self.source):
            connection.execute("""
                CREATE SCHEMA sfd;
                CREATE TABLE sfd.data_batches(batch_id text PRIMARY KEY, is_synthetic boolean NOT NULL);
                CREATE TABLE sfd.rule_versions(rule_version text PRIMARY KEY, batch_id text REFERENCES sfd.data_batches,
                    config jsonb NOT NULL, config_sha256 text NOT NULL);
                CREATE TABLE public.olist_seed_imports(seed_sha256 text PRIMARY KEY, source_manifest jsonb NOT NULL);
            """)
        self.source.execute("INSERT INTO sfd.data_batches VALUES ('demo',true)")
        self.source.execute(
            "INSERT INTO sfd.rule_versions VALUES ('v1','demo','{\"threshold\":10}',%s)",
            ("a" * 64,),
        )
        environment = {
            "DATABASE_URL": psycopg.conninfo.make_conninfo(self.dsn, dbname=self.target_name),
            "OLIST_STAGE_DB": self.stage_name,
            "OLIST_SEED_SHA": "c" * 64,
            "OLIST_MANIFEST": json.dumps({"tables": {"data_batches": 1, "rule_versions": 1}}),
        }
        self.environment = patch.dict(os.environ, environment)
        self.environment.start()
        self.addCleanup(self.environment.stop)

    def test_equal_rule_config_preserves_existing_byte_hash(self):
        self.target.execute("INSERT INTO sfd.data_batches VALUES ('demo',true)")
        self.target.execute(
            "INSERT INTO sfd.rule_versions VALUES ('v1','demo','{\"threshold\":10}',%s)",
            ("b" * 64,),
        )
        with contextlib.redirect_stdout(io.StringIO()):
            seed.import_dataset(self.target)
        self.assertEqual(
            ("b" * 64,),
            self.target.execute("SELECT config_sha256 FROM sfd.rule_versions").fetchone(),
        )
        self.assertEqual(
            (1,),
            self.target.execute("SELECT count(*) FROM public.olist_seed_imports").fetchone(),
        )

    def test_count_mismatch_rolls_back_preceding_table_and_receipt(self):
        with (
            patch.dict(
                os.environ,
                {"OLIST_MANIFEST": json.dumps({"tables": {"data_batches": 1, "rule_versions": 2}})},
            ),
            contextlib.redirect_stdout(io.StringIO()),
            self.assertRaisesRegex(RuntimeError, "count mismatch"),
        ):
            seed.import_dataset(self.target)
        self.assertEqual(
            (0,),
            self.target.execute("SELECT count(*) FROM sfd.data_batches").fetchone(),
        )
        self.assertEqual(
            (0,),
            self.target.execute("SELECT count(*) FROM public.olist_seed_imports").fetchone(),
        )

    def test_rule_batch_mismatch_rolls_back_new_batch(self):
        self.target.execute("INSERT INTO sfd.data_batches VALUES ('other',true)")
        self.target.execute(
            "INSERT INTO sfd.rule_versions VALUES ('v1','other','{\"threshold\":10}',%s)",
            ("a" * 64,),
        )
        with (
            contextlib.redirect_stdout(io.StringIO()),
            self.assertRaisesRegex(RuntimeError, "Conflicting rule_versions"),
        ):
            seed.import_dataset(self.target)
        self.assertEqual(
            [("other",)],
            self.target.execute("SELECT batch_id FROM sfd.data_batches").fetchall(),
        )
        self.assertEqual(
            (0,),
            self.target.execute("SELECT count(*) FROM public.olist_seed_imports").fetchone(),
        )

    def test_missing_scenario_does_not_commit_ownership_of_existing_shop(self):
        self.target.execute("""
            CREATE TABLE sfd.shops(shop_id text PRIMARY KEY, owner_seller_id text, batch_id text);
            CREATE TABLE sfd.orders(order_id text PRIMARY KEY, shop_id text REFERENCES sfd.shops, batch_id text);
            INSERT INTO sfd.shops VALUES ('shop',NULL,'olist_vn_sim');
            INSERT INTO sfd.orders VALUES ('existing','shop','olist_vn_sim');
        """)
        scenarios = [
            {"scenario_id": "S1", "order_id": "existing"},
            {"scenario_id": "S2", "order_id": "missing"},
        ]
        with (
            patch.dict(
                os.environ,
                {
                    "OLIST_SCENARIOS": json.dumps(scenarios),
                    "OLIST_EXPECTATIONS": json.dumps({"scenarios": {"S1": {}, "S2": {}}}),
                },
            ),
            patch.object(seed.httpx, "Client") as client,
            self.assertRaisesRegex(RuntimeError, "Missing scenario"),
        ):
            seed.verify_dataset(self.target)
        client.assert_not_called()
        self.assertEqual(
            (None,),
            self.target.execute("SELECT owner_seller_id FROM sfd.shops").fetchone(),
        )

    def test_concurrent_incompatible_rule_is_detected_after_insert_conflict(self):
        self.target.execute("INSERT INTO sfd.data_batches VALUES ('demo',true)")
        with psycopg.connect(self.dsn, dbname=self.target_name, autocommit=True) as competing:
            competing.execute("BEGIN")
            competing.execute(
                "INSERT INTO sfd.rule_versions VALUES ('v1','demo','{\"threshold\":99}',%s)",
                ("b" * 64,),
            )
            failures = []

            def run_import():
                try:
                    seed.import_dataset(self.target)
                except Exception as error:
                    failures.append(error)

            worker = threading.Thread(target=run_import)
            worker.start()
            blocked = False
            try:
                deadline = time.monotonic() + 5
                while time.monotonic() < deadline:
                    row = self.admin.execute(
                        "SELECT wait_event_type FROM pg_stat_activity WHERE pid=%s",
                        (self.target.info.backend_pid,),
                    ).fetchone()
                    if row and row[0] == "Lock":
                        blocked = True
                        break
                    time.sleep(0.02)
            finally:
                competing.execute("COMMIT")
                worker.join(timeout=10)
            self.assertTrue(blocked, "import never reached the concurrent unique-key conflict")
            self.assertFalse(worker.is_alive(), "import did not complete after concurrent commit")
        self.assertEqual(1, len(failures))
        self.assertIsInstance(failures[0], RuntimeError)
        self.assertIn("Conflicting rule_versions", str(failures[0]))
        self.assertEqual(
            (0,),
            self.target.execute("SELECT count(*) FROM public.olist_seed_imports").fetchone(),
        )
        self.assertEqual(
            ({"threshold": 99},),
            self.target.execute("SELECT config FROM sfd.rule_versions").fetchone(),
        )


if __name__ == "__main__":
    unittest.main()
