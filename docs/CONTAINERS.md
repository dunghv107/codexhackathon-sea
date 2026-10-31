# Local container baseline

Run commands from the repository root. Docker Desktop must use Linux containers.
The root `compose.yaml` starts frontend, Go backend, Python KBS, PostgreSQL,
Redis, and a one-shot migration. It is the integration/CI baseline; the older
Compose files under `backend/` and `kbs/` remain standalone development recipes
and do not inherit these protections.

## Images and build tools

| Service | Build/test stage | Default runtime | Debug runtime |
| --- | --- | --- | --- |
| Backend | Go 1.26, `go test`, `go vet` | Chainguard static | Chainguard BusyBox |
| Frontend | Node 24, `npm ci`, oxlint, `npm test`, TypeScript, Vite | Chainguard nginx | Chainguard nginx `latest-dev` |
| KBS | Python 3.13, uv with `uv.lock`, Ruff, Ty, pytest | Distroless Python Debian 13 nonroot | Matching `debug-nonroot` with BusyBox |

KBS currently requires Python `>=3.12,<3.14`; the free Chainguard Python
`latest` does not promise that range. Its builder uses the same distroless Python
base as runtime, so virtualenv paths and native wheel ABI match. Only locked
binary wheels are accepted; a dependency that needs compilation fails rather
than downloading another interpreter or silently adding compilers.

All base images are pinned by digest. Update tags and digests together and rerun
the tests/scanners to pick up security fixes. Pinning is reproducibility, not an
automatic security update. Package managers, compilers, tests, and source maps
from build tooling do not belong in runtime. KBS copies only `app/` and its
production virtualenv; frontend copies only Vite's `dist/`.

## Start

Create `.env` once. This command needs Node and refuses to overwrite an existing
file. It generates separate random hex passwords for PostgreSQL admin/app and JWT:

```sh
node -e "const fs=require('node:fs'),c=require('node:crypto');fs.writeFileSync('.env',fs.readFileSync('.env.example','utf8').replace(/^(DB_PASSWORD|DB_ADMIN_PASSWORD|JWT_SECRET)=.*$/gm,(_,k)=>k+'='+c.randomBytes(32).toString('hex')),{flag:'wx'});"
docker compose config --quiet
docker compose up -d --build --wait --wait-timeout 180
```

Open <http://localhost:18081>. Only frontend is published, bound to loopback.
Frontend forwards `/api/` to `backend:8000`; KBS is reachable inside Docker at
`http://kbs:1114`. Go uses `KBS_URL` for scoring after checkout and seller score/explain requests.
KBS receives `DATABASE_URL` for the shared database and waits for migrations before startup.
Backend can start without KBS: a scoring failure leaves the saved order pending for seller retry.

Migrations create schema, not demo accounts or a catalog dataset. Existing import
and seed scripts are separate, explicit operations. Do not use demo accounts in
production.

```sh
docker compose logs -f --tail=100 backend frontend kbs
docker compose down
```

`down` preserves the PostgreSQL volume. Do not add `-v` unless deleting that
project's database is intended. Changing `.env` passwords does not rotate roles
in an already initialized database; update the database credentials explicitly.
The init script only runs on an empty volume. Keep `DB_USER` distinct from `postgres`.
Database names/usernames must be URI-safe, and migration passwords must be hex.

## One public domain through the separate reverse proxy

The reverse-proxy configuration is managed separately and is not distributed in
this repository. It forwards all paths to the frontend on `192.168.2.14:18081`.
The frontend still forwards `/api/` to `backend:8000`, and backend calls `kbs:1114`.
Only the frontend port needs to be reachable from the reverse-proxy host.

On the application server, set these in its deployment `.env`:

```dotenv
FRONTEND_BIND_IP=192.168.2.14
FRONTEND_PORT=18081
COOKIE_SECURE=true
```

Apply with `docker compose up -d --no-build --wait --wait-timeout 180` from the deployed
checkout after its runtime images have been built. Keep port 18081 restricted to
the reverse-proxy host at the network firewall; backend/KBS/database ports stay
unpublished. CI always binds its temporary frontend to loopback on port 18080.
CI removes its test stack; successful `dev` runs deploy a separate persistent
`afd` stack as described in [PIPELINE.md](PIPELINE.md).

On the reverse-proxy host, copy the supplied config to
`/etc/nginx/sites-available/afd.chinsu.click`, then run:

```sh
sudo mkdir -p /var/www/certbot
sudo ln -s /etc/nginx/sites-available/afd.chinsu.click /etc/nginx/sites-enabled/afd.chinsu.click
sudo nginx -t && sudo systemctl reload nginx
curl -fsS http://192.168.2.14:18081/health
```

The supplied config uses HTTP so nginx can start before a certificate exists.
After DNS points to the public reverse proxy and inbound port 80 reaches it:

```sh
sudo certbot --nginx -d afd.chinsu.click --redirect
sudo nginx -t && sudo systemctl reload nginx
```

Certbot installs the certificate and HTTPS redirect in this site's configuration.
Use `https://afd.chinsu.click` after issuance; secure cookies require HTTPS.
No backend domain or DNS change is made by these repository files.

## Shell/debug

Set `RUNTIME_VARIANT=debug` in the local `.env`, then:

```sh
docker compose up -d --build --wait
docker compose exec backend /bin/sh
docker compose exec frontend /bin/sh
docker compose exec kbs /busybox/sh
```

Debug keeps UID 65532, a read-only root filesystem and dropped capabilities.
BusyBox provides basic inspection commands, not a full admin toolbox. Backend's
file log is `/tmp/app.log`; KBS's is `/tmp/kbs-logs/app.log`. These tmpfs logs are
bounded and ephemeral. Console logs go through Docker's bounded local log driver.
Use `docker compose logs` for routine inspection, including the shell-free runtime.
Set `RUNTIME_VARIANT=runtime` and rebuild/recreate to return to minimal images.

## CI building blocks

Quality checks are explicit targets; a successful image build does not imply
that the test targets ran:

```sh
docker build --target test -t codexhackathon/backend-check ./backend
docker build --target test -t codexhackathon/frontend-check ./frontend
docker build --target test -t codexhackathon/kbs-check ./kbs
docker compose build
docker compose up -d --wait --wait-timeout 180
python scripts/check-containers.py
```

On Windows use `py scripts/check-containers.py` if `python` is unavailable.
The smoke check verifies HTTP/SPA/API routing, authentication rejection, KBS
readiness, app role privileges, host port exposure, UID/capabilities/read-only
filesystem. Nginx's built-in healthcheck validates configuration only; the HTTP
smoke check is required to establish that it serves requests.

CI should supply fresh secrets, a unique `COMPOSE_PROJECT_NAME`, and an unused
`FRONTEND_PORT`. `IMAGE_PREFIX` and `IMAGE_TAG` allow registry/commit tags without
editing Compose. Scan the resulting runtime images with Trivy, run SonarQube
and k6 in separate CI jobs/tool containers, and publish only after the gates pass.
Those scanners and load generators are intentionally not installed in app images.
The self-hosted workflow is documented in [PIPELINE.md](PIPELINE.md); it deploys
tested `dev` images on the runner host without publishing them to a registry.

Verified locally on Docker Desktop Linux/amd64: all three runtime/debug images
build, the Compose smoke check passes for both variants, debug shells run as
UID 65532 and can read app logs, Go tests/vet and frontend lint/test/build pass, and
KBS Ruff/Ty plus 71 pytest tests pass. Docker reports runtime image sizes of
about 6.4 MB (backend), 8.7 MB (frontend), and 38.4 MB (KBS); these are the local
initial baseline image metadata values, not a promise for other platforms or future dependencies.
The backend test target skips the Olist database integration suite unless
`TEST_DATABASE_URL` is supplied; the Compose smoke check covers container wiring
and database roles, not the full Olist business workflow.
The figures above describe the initial container baseline. Current scanner and
load-test results are recorded separately under local `reports/`; a successful
build does not imply that image vulnerability gates pass. ARM64 is unverified.
GitNexus was unavailable locally; review used source/caller inspection.

## Security boundary and remaining deployment work

- App containers run as UID/GID 65532, with no Linux capabilities, no privilege
  escalation, read-only root filesystems and bounded writable `/tmp`.
- Frontend is the only host entry point. PostgreSQL and Redis live on a separate
  internal network; KBS has no host port and MCP/docs are disabled here.
- PostgreSQL admin credentials go only to PostgreSQL and migrations. Backend gets
  a distinct login with DML/sequence access, without superuser, role creation,
  database creation, schema creation or RLS bypass.
- `.env` is excluded from Git and build contexts. No secret enters Dockerfile
  ARG/ENV layers. Docker administrators can still inspect runtime environment
  variables; use your deployment secret store for production.
- The local stack uses HTTP and database `sslmode=disable` on its private bridge.
  Configure the actual HTTPS ingress, origins, secure cookies, database TLS,
  backups and resource limits before public deployment.
- The existing Go router trusts no proxy headers. Consequently its IP-based
  limiter currently groups frontend users by nginx's peer IP. Configure a narrowly
  trusted ingress and verify spoofing protection before multi-user/load testing;
  do not enable trust for all proxy sources to work around this.
- Redis stores only rate-limit state here, without persistence or a published port.
  Restarting Redis resets counters. It is not configured as a durable queue.

References: [Chainguard static](https://images.chainguard.dev/directory/image/static/overview),
[Chainguard nginx](https://images.chainguard.dev/directory/image/nginx/overview),
[Distroless Python](https://github.com/GoogleContainerTools/distroless/tree/main/examples/python3-requirements),
[Compose startup order](https://docs.docker.com/compose/how-tos/startup-order/).
