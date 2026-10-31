"""Smoke-test the running root Compose stack without printing its secrets."""

import json
import subprocess
from pathlib import Path
from urllib.error import HTTPError
from urllib.request import urlopen

ROOT = Path(__file__).resolve().parents[1]


def docker(*args):
    return subprocess.check_output(["docker", *args], cwd=ROOT, text=True).strip()


def main():
    config = json.loads(docker("compose", "config", "--format", "json"))
    services = config["services"]
    for name in ("backend", "frontend", "kbs"):
        container_id = docker("compose", "ps", "-q", name)
        assert container_id, f"{name} is not running"
        container = json.loads(docker("inspect", container_id))[0]
        host = container["HostConfig"]
        assert container["Config"]["User"] == "65532:65532", f"{name} must run nonroot"
        assert host["ReadonlyRootfs"], f"{name} root filesystem must be read-only"
        assert "ALL" in [cap.upper() for cap in host["CapDrop"]], f"{name} capabilities"
        assert any(opt.startswith("no-new-privileges") for opt in host["SecurityOpt"])
        assert container["State"]["Health"]["Status"] == "healthy", f"{name} healthcheck"
        if services[name]["build"]["args"]["RUNTIME_VARIANT"] == "runtime":
            shell = "/busybox/sh" if name == "kbs" else "/bin/sh"
            probe = subprocess.run(
                ["docker", "compose", "exec", "-T", name, shell, "-c", "true"],
                cwd=ROOT, capture_output=True,
            )
            assert probe.returncode != 0, f"{name} runtime must not contain a shell"

    for name in ("backend", "kbs", "postgres", "redis"):
        assert not services[name].get("ports"), f"{name} must not publish a host port"
    port = services["frontend"]["ports"][0]
    assert port["host_ip"] == "127.0.0.1", "local baseline must bind only loopback"
    base = f"http://127.0.0.1:{port['published']}"
    for path in ("/health", "/", "/seller/orders", "/api/catalog/categories"):
        with urlopen(base + path, timeout=10) as response:
            assert response.status == 200, path
            if path.startswith("/api/"):
                assert "application/json" in response.headers.get("Content-Type", ""), path
            else:
                assert response.headers.get("X-Content-Type-Options") == "nosniff", path
    try:
        urlopen(base + "/api/auth/me", timeout=10)
    except HTTPError as error:
        assert error.code == 401, f"unauthenticated /api/auth/me returned {error.code}"
    else:
        raise AssertionError("unauthenticated /api/auth/me must fail")

    docker("compose", "exec", "-T", "kbs", "python", "-c",
           "import urllib.request; urllib.request.urlopen('http://127.0.0.1:1114/ready', timeout=3)")
    # Authenticate as the app role, then prove it can read migrated data without DDL rights.
    role_check = docker(
        "compose", "exec", "-T", "postgres", "sh", "-ec",
        'export PGPASSWORD="$APP_DB_PASSWORD"; exec psql -h 127.0.0.1 '
        '-U "$APP_DB_USER" -d "$POSTGRES_DB" -At -v ON_ERROR_STOP=1 '
        '-c "SELECT count(*) >= 0 FROM public.users" '
        '-c "SELECT rolsuper OR rolcreatedb OR rolcreaterole OR rolbypassrls '
        'FROM pg_roles WHERE rolname = current_user" '
        '-c "SELECT has_schema_privilege(current_user, \'sfd\', \'CREATE\')"',
    )
    assert role_check.splitlines() == ["t", "f", "f"], "app database permissions"
    print("PASS: HTTP/SPA/API, unauthenticated access, KBS readiness, container isolation, DB role")


if __name__ == "__main__":
    main()
