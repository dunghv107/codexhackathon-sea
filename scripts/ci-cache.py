#!/usr/bin/env python3
"""Cache successful test reports by the content of their service inputs."""

import argparse
import hashlib
import json
import os
from pathlib import Path
import shutil
import subprocess
import tempfile


SCHEMA = 1
REPORTS = {
    "backend": ("coverage-go.out",),
    "frontend": ("coverage-frontend.lcov",),
    "kbs": ("coverage-python.xml", "pytest.xml"),
}


def relevant(path, service):
    common = {
        ".github/workflows/pipeline.yml", "scripts/test-integration.sh",
        "compose.yaml", "olist_cod/olist_cod/schema.sql", "docker/postgres-init.sh",
    }
    if path.startswith(service + "/") or path in common or path.startswith("tests/ci/"):
        return True
    if path.startswith("scripts/ci") and path.endswith((".py", ".sh")):
        return True
    if service == "kbs" and (path.startswith("tests/scripts/") or path == "scripts/smoke-phone-demo.py"):
        return True
    if service in ("backend", "kbs"):
        return path.startswith(("backend/migrations/", "kbs/app/schemas/"))
    return path.startswith("backend/internal/interface/") or path == "docs/API_CONTRACT.md"


def input_key(root, service):
    names = subprocess.check_output(
        ["git", "ls-files", "--cached", "--others", "--exclude-standard", "-z"], cwd=root,
    ).decode("utf-8").split("\0")
    digest = hashlib.sha256(f"test-reports:{SCHEMA}:{service}\0".encode())
    for name in sorted(set(names)):
        if not name or not relevant(name, service):
            continue
        path = root / name
        # Deleted tracked files must disappear from the fingerprint as well.
        if not path.exists() and not path.is_symlink():
            continue
        if path.is_symlink() or not path.is_file():
            raise ValueError(f"Unsupported cache input: {name}")
        digest.update(name.encode("utf-8") + b"\0")
        digest.update(hashlib.sha256(path.read_bytes()).digest())
    return digest.hexdigest()


def cached_reports(entry, service, key):
    """Read and verify everything before restoring even one report."""
    try:
        manifest_path = entry / "manifest.json"
        if entry.is_symlink() or manifest_path.is_symlink():
            return None
        manifest = json.loads(manifest_path.read_text(encoding="utf-8"))
        if not isinstance(manifest, dict) or (
            manifest.get("schema") != SCHEMA or manifest.get("service") != service
            or manifest.get("key") != key
        ):
            return None
        hashes = manifest.get("files")
        if not isinstance(hashes, dict) or set(hashes) != set(REPORTS[service]):
            return None
        reports = {}
        for name in REPORTS[service]:
            path = entry / name
            if path.is_symlink() or not path.is_file():
                return None
            data = path.read_bytes()
            if not data or hashlib.sha256(data).hexdigest() != hashes[name]:
                return None
            reports[name] = data
        return reports
    except (OSError, ValueError):
        return None


def restore(root, cache, service):
    key = input_key(root, service)
    reports = cached_reports(cache / "results" / service / key, service, key)
    if reports is None:
        return False
    destination = root / "reports/ci/coverage"
    destination.mkdir(parents=True, exist_ok=True)
    for name, data in reports.items():
        (destination / name).write_bytes(data)
    return True


def save(root, cache, service):
    key = input_key(root, service)
    reports = {}
    for name in REPORTS[service]:
        source = root / "reports/ci/coverage" / name
        if source.is_symlink() or not source.is_file():
            raise ValueError(f"Missing regular test report: {name}")
        reports[name] = source.read_bytes()
        if not reports[name]:
            raise ValueError(f"Empty test report: {name}")
    parent = cache / "results" / service
    parent.mkdir(parents=True, exist_ok=True)
    entry = parent / key
    if cached_reports(entry, service, key) is not None:
        return
    with tempfile.TemporaryDirectory(prefix=".publish-", dir=parent) as temporary:
        staged = Path(temporary) / "entry"
        staged.mkdir()
        for name, data in reports.items():
            (staged / name).write_bytes(data)
        manifest = {
            "schema": SCHEMA, "service": service, "key": key,
            "files": {name: hashlib.sha256(data).hexdigest() for name, data in reports.items()},
        }
        (staged / "manifest.json").write_text(json.dumps(manifest, sort_keys=True), encoding="utf-8")
        # Incomplete/corrupt entries are misses; only a complete directory is published.
        if entry.is_symlink() or entry.is_file():
            entry.unlink()
        elif entry.exists():
            shutil.rmtree(entry)
        staged.rename(entry)


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("action", choices=("key", "restore", "save"))
    parser.add_argument("service", choices=REPORTS)
    args = parser.parse_args()
    root = Path(__file__).resolve().parent.parent
    cache = Path(os.environ.get("CI_CACHE_DIR", "/tmp/codex-ci-cache"))
    if args.action == "key":
        print(input_key(root, args.service))
    elif args.action == "restore":
        return 0 if restore(root, cache, args.service) else 1
    else:
        save(root, cache, args.service)
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
