"""Run with the KBS environment: python -m unittest discover -s tests/scripts."""

import importlib.util
import tempfile
import unittest
from pathlib import Path

SCRIPT = Path(__file__).resolve().parents[2] / "scripts/smoke-phone-demo.py"
SPEC = importlib.util.spec_from_file_location("smoke_phone_demo", SCRIPT)
assert SPEC and SPEC.loader
smoke = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(smoke)


class LocalDatabaseTests(unittest.TestCase):
    def test_loopback_addresses_remain_allowed(self):
        for host in ("localhost", "127.0.0.1", "::1"):
            with self.subTest(host=host):
                smoke.validate_local_database({"host": host}, {})
        smoke.validate_local_database({"host": "localhost", "hostaddr": "::1"}, {})
        smoke.validate_local_database({"host": "localhost"}, {"PGHOSTADDR": "127.0.0.1"})

    def test_remote_host_or_libpq_override_is_rejected(self):
        for config, environment in (
            ({"host": "192.0.2.1"}, {}),
            ({"host": "127.0.0.1", "hostaddr": "192.0.2.1"}, {}),
            ({"host": "localhost"}, {"PGHOSTADDR": "192.0.2.1"}),
            ({"host": "localhost"}, {"PGHOSTADDR": "127.0.0.1,192.0.2.1"}),
            ({"host": "localhost", "service": "production"}, {}),
            ({"host": "localhost"}, {"PGSERVICE": "production"}),
        ):
            with self.subTest(config=config, environment=environment), self.assertRaises(ValueError):
                smoke.validate_local_database(config, environment)

    def test_explicit_loopback_hostaddr_overrides_ambient_address(self):
        smoke.validate_local_database(
            {"host": "localhost", "hostaddr": "127.0.0.1"}, {"PGHOSTADDR": "192.0.2.1"}
        )


class PrivateLogTests(unittest.TestCase):
    def test_console_and_file_logs_are_checked_without_echoing_private_values(self):
        with tempfile.TemporaryDirectory() as directory:
            paths = [Path(directory) / "console.log", Path(directory) / "go.log"]
            for path in paths:
                path.write_text("Service healthy\n", encoding="utf-8")
            smoke.check_private_logs(paths)
            for path in paths:
                for case, value in enumerate(("0900000003", "+84900000009", "hmac:v1:fixture", "live:demo:phone:fixture")):
                    with self.subTest(log=path.name, case=case):
                        path.write_text(value, encoding="utf-8")
                        with self.assertRaises(AssertionError) as caught:
                            smoke.check_private_logs(paths)
                        self.assertEqual(str(caught.exception), "Phone identity leaked into logs")
                    path.write_text("Service healthy\n", encoding="utf-8")

    def test_missing_log_is_not_treated_as_a_successful_privacy_check(self):
        with tempfile.TemporaryDirectory() as directory, self.assertRaises(FileNotFoundError):
            smoke.check_private_logs([Path(directory) / "missing.log"])


if __name__ == "__main__":
    unittest.main()
