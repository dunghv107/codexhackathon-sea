import importlib.util
import json
from pathlib import Path
import subprocess
import tempfile
import unittest


SPEC = importlib.util.spec_from_file_location("ci_cache", Path(__file__).resolve().parents[2] / "scripts/ci-cache.py")
cache_module = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(cache_module)


class TestResultCache(unittest.TestCase):
    def setUp(self):
        temporary = tempfile.TemporaryDirectory()
        self.addCleanup(temporary.cleanup)
        self.root = Path(temporary.name) / "repo"
        self.root.mkdir()
        self.cache = Path(temporary.name) / "cache"
        subprocess.run(["git", "init", "-q", str(self.root)], check=True)
        self.write(".gitignore", "reports/\nignored/\n")
        self.write("backend/main.go", "package main\n")
        self.write("frontend/src/main.ts", "export {}\n")
        self.write("kbs/app/main.py", "pass\n")

    def write(self, name, content):
        path = self.root / name
        path.parent.mkdir(parents=True, exist_ok=True)
        path.write_text(content, encoding="utf-8")

    def keys(self):
        return {service: cache_module.input_key(self.root, service) for service in cache_module.REPORTS}

    def populate(self, service):
        for name in cache_module.REPORTS[service]:
            self.write("reports/ci/coverage/" + name, "report:" + name)
        cache_module.save(self.root, self.cache, service)
        return self.cache / "results" / service / cache_module.input_key(self.root, service)

    def test_content_not_metadata_or_unrelated_docs_controls_key(self):
        before = self.keys()
        self.write("docs/notes.md", "unrelated docs")
        self.write("ignored/local.txt", "ignored")
        (self.root / "backend/main.go").touch()
        subprocess.run(["git", "add", "."], cwd=self.root, check=True)
        self.assertEqual(before, self.keys())
        self.write("frontend/src/main.ts", "export const changed = true\n")
        after = self.keys()
        self.assertNotEqual(before["frontend"], after["frontend"])
        self.assertEqual(before["backend"], after["backend"])
        self.assertEqual(before["kbs"], after["kbs"])

    def test_rename_and_tracked_deletion_invalidate(self):
        subprocess.run(["git", "add", "."], cwd=self.root, check=True)
        before = self.keys()["backend"]
        (self.root / "backend/main.go").rename(self.root / "backend/renamed.go")
        renamed = self.keys()["backend"]
        self.assertNotEqual(before, renamed)
        (self.root / "backend/renamed.go").unlink()
        self.assertNotEqual(renamed, self.keys()["backend"])

    def test_shared_dependencies_invalidate_consumers(self):
        cases = {
            "backend/migrations/new.sql": {"backend", "kbs"},
            "kbs/app/schemas/new.py": {"backend", "kbs"},
            "backend/internal/interface/api/new.go": {"backend", "frontend"},
            "docs/API_CONTRACT.md": {"frontend"},
            "scripts/ci-cache.py": set(cache_module.REPORTS),
            "scripts/ci.sh": set(cache_module.REPORTS),
            "scripts/test-integration.sh": set(cache_module.REPORTS),
            ".github/workflows/pipeline.yml": set(cache_module.REPORTS),
            "tests/ci/test_cache.py": set(cache_module.REPORTS),
            "compose.yaml": set(cache_module.REPORTS),
            "olist_cod/olist_cod/schema.sql": set(cache_module.REPORTS),
            "docker/postgres-init.sh": set(cache_module.REPORTS),
            "tests/scripts/test_smoke_phone_demo.py": {"kbs"},
            "scripts/smoke-phone-demo.py": {"kbs"},
            "scripts/bootstrap-demo.sh": {"kbs"},
            "scripts/seed-olist-demo.py": {"kbs"},
            "scripts/seed-olist-demo.sh": {"kbs"},
            "tests/seed-olist-demo.sh": {"kbs"},
            "kbs/docs/data-structure/olist_cod/olist_cod/seed.sha256": {"kbs"},
        }
        for name, affected in cases.items():
            with self.subTest(name=name):
                before = self.keys()
                self.write(name, "changed")
                after = self.keys()
                self.assertEqual(affected, {service for service in before if before[service] != after[service]})

    def test_roundtrip_every_service_and_missing_entry(self):
        for service, names in cache_module.REPORTS.items():
            with self.subTest(service=service):
                self.assertFalse(cache_module.restore(self.root, self.cache, service))
                self.populate(service)
                for name in names:
                    (self.root / "reports/ci/coverage" / name).unlink()
                self.assertTrue(cache_module.restore(self.root, self.cache, service))
                for name in names:
                    self.assertEqual("report:" + name, (self.root / "reports/ci/coverage" / name).read_text())
                cache_module.save(self.root, self.cache, service)

    def test_corruption_or_missing_report_is_miss_without_partial_restore(self):
        for corrupt in (False, True):
            with self.subTest(corrupt=corrupt):
                entry = self.populate("kbs")
                report = entry / "pytest.xml"
                if corrupt:
                    report.write_text("corrupted")
                else:
                    report.unlink()
                self.write("reports/ci/coverage/coverage-python.xml", "keep existing")
                self.assertFalse(cache_module.restore(self.root, self.cache, "kbs"))
                self.assertEqual("keep existing", (self.root / "reports/ci/coverage/coverage-python.xml").read_text())

    def test_manifest_validation_and_new_inputs_fail_closed(self):
        entry = self.populate("backend")
        manifest_path = entry / "manifest.json"
        valid = json.loads(manifest_path.read_text())
        invalid = [[], {**valid, "schema": 999}, {**valid, "service": "kbs"},
                   {**valid, "key": "wrong"}, {**valid, "files": {"../outside": "hash"}}]
        for manifest in invalid:
            manifest_path.write_text(json.dumps(manifest))
            self.assertFalse(cache_module.restore(self.root, self.cache, "backend"))
        manifest_path.write_text("invalid JSON")
        self.assertFalse(cache_module.restore(self.root, self.cache, "backend"))
        manifest_path.write_text(json.dumps(valid))
        self.write("backend/main.go", "changed source")
        self.assertFalse(cache_module.restore(self.root, self.cache, "backend"))

    def test_save_requires_all_reports(self):
        self.write("reports/ci/coverage/coverage-python.xml", "partial")
        with self.assertRaises(ValueError):
            cache_module.save(self.root, self.cache, "kbs")
        self.assertFalse(cache_module.restore(self.root, self.cache, "kbs"))
        self.write("reports/ci/coverage/pytest.xml", "")
        with self.assertRaises(ValueError):
            cache_module.save(self.root, self.cache, "kbs")


if __name__ == "__main__":
    unittest.main()
