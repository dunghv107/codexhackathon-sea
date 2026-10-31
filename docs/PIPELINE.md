# Self-hosted pipeline

Target repository: `dunghv107/codexhackathon-sea`, branches `dev` and `main`.
The runner must have the standard `self-hosted`, `Linux`, `X64` labels.
Install Docker Engine, Compose v2 (with `COMPOSE_ENV_FILES` support), Buildx,
Git and Python 3. The `github-runner` user must be able to run `docker info`.
Restart the runner service after changing group membership. Docker access is
effectively host administrator access: only trusted maintainers may push these
branches. This workflow does not execute pull requests or fork code.

The workflow has three dependent jobs: `quality` (lint/tests and Sonar), `build`
(runtime images and Trivy), then `runtime` (Compose, smoke, k6 and dev deployment).
The workflow runs lint/unit tests in the existing Docker test stages, SonarQube
SAST with a report-only Quality Gate (temporary user-approved bypass), runtime builds, Trivy source/configuration/
secret scanning and image vulnerability scanning, isolated Compose startup,
security/HTTP smoke checks, then k6. HIGH/CRITICAL source/dependency/configuration/
secret findings fail the scan. Runtime-image CVEs are temporarily report-only by
user approval, including findings with available library fixes that are not yet
included in the pinned upstream images. Full vulnerability findings remain in JSON artifacts;
scanner execution errors still fail the job. Restore runtime `--exit-code 1`
after remediating the base images. A green workflow does not mean CVE-free images.
Builds and scanner images are kept separate from application runtime images.

CI uses port 18080 and a unique Compose project, database volume and credentials
for each run. Cleanup removes only that run's resources, even after failure.
No daemon-wide prune. Hard termination/host failure can leave resources behind;
inspect Compose labels before cleaning them. BuildKit cache remains on the host.
Reports are uploaded even when their job fails, retained for seven days:
`quality-<run>-<attempt>`, `build-<run>-<attempt>` and `runtime-<run>-<attempt>`.
The quality artifact contains Go/Python/React coverage, test logs, and `sonar/` JSON:
analysis metadata, Quality Gate, measures and paginated unresolved issues.
The gate is pinned to the scanner's analysis ID. Measures/issues are current
project snapshots taken immediately afterwards; avoid concurrent external scans
while exporting. Workflow concurrency serializes this pipeline's own scans.
Hotspot counts are included in measures; hotspot details and analysis history
require Browse permission unavailable to the current analysis token.
A failed gate produces unchanged JSON and a warning without blocking downstream jobs.
Scanner/analysis execution errors and export errors are recorded in `export-error.json` and fail the job.

The build artifact contains Trivy source/image JSON; secret matches and source
snippets are removed while rule, severity and line numbers remain. Runtime reports
include the k6 `summary.json`, startup, smoke and deployment logs. Credentials,
Compose's expanded configuration and database contents are never uploaded.
The temporary credential file is outside the checkout.

`runtime-images-<run>` carries all six scanned Docker images and their archive
SHA-256 checksum between build and runtime jobs, retained for two days. Runtime
loads that artifact without rebuilding and deploys the tested images. Image tags
are stable within a run so rerunning a failed runtime job can reuse its artifact;
after the artifact expires, rerun the build job as well. Sonar/Trivy caches use
`actions/cache`; Docker layers and Go/npm/uv cache mounts stay on the self-hosted
Docker daemon. Cache misses remain safe and Trivy still checks database freshness.

The k6 baseline sends five iterations/second for 30 seconds, each fetching the
catalog and KBS readiness. It requires all checks to pass, <1% HTTP errors,
p95 <500 ms for each service, and no dropped iterations. It raises only the
temporary stack's rate limit to avoid the existing shared nginx-IP bucket.
This is a small regression check with an empty database, not a real-user capacity
benchmark. The quality job starts its own PostgreSQL/Redis services and runs
`scripts/test-integration.sh` against the canonical schema with temporary credentials.
Backend fixtures create separate schemas; KBS fixtures roll back their changes.
Go uses `-coverpkg=./...` so HTTP integration tests count execution across packages.
Python XML and React LCOV join the Go report in Sonar; untested source stays in scope.
KBS combined coverage and each frontend coverage metric require at least 90%.
Acceptance tests needing the representative dataset and live-provider tests remain
outside this CI lane. Local database tests still skip when their test URL is absent.
Coverage below the configured threshold
still fails the Sonar Quality Gate, but the temporary bypass allows CI to continue.
Restore gate enforcement after addressing coverage and other reported findings.

After k6 passes on `dev`, `scripts/deploy.sh` deploys on the same runner host.
`main` only verifies; it does not overwrite the shared `afd` deployment.
Test images are retagged as `afd/<service>:<commit>-runtime` without rebuilding.
The application stays running after the temporary CI stack is removed.

The deployment uses `/home/alex/codexhackathon-sea`, fixed project `afd`, and
`192.168.2.14:18081`. Its persistent `.env` must be provisioned once with random
`DB_PASSWORD`, `DB_ADMIN_PASSWORD`, `JWT_SECRET`, `PHONE_HMAC_SECRET`, `DB_USER=app` and
`DB_NAME=ai_fraud_detection`; use mode 0600, owned by `github-runner`. Do not
regenerate credentials for an existing `afd_postgres_data` volume.
Only the deployment directory must be writable by the runner, not all `/home/alex`.

Each commit gets its own `releases/<commit>` directory containing Compose,
migrations and the init script, so mounts survive checkout cleanup. Migrations
run explicitly before updating the application. `current` points to the last
release whose health/HTTP checks passed. Deployment never runs `down --volumes`.
On failure it fails CI and preserves the database; it does not automatically
reverse migrations or promise atomic application rollback. Back up persisted
data before later schema changes. Release tags/directories are retained.

Inspect the deployed stack on the application server:

```bash
cd /home/alex/codexhackathon-sea/current
docker compose --env-file /home/alex/codexhackathon-sea/.env --env-file release.env -p afd ps
docker compose --env-file /home/alex/codexhackathon-sea/.env --env-file release.env -p afd logs --tail=100
```

Cookies require HTTPS; the separately managed proxy terminates TLS for
`afd.chinsu.click`. Proxy configuration and DNS are not deployed by CI.

## Temporary SonarQube on the runner server

SonarQube's Compose file is deliberately outside this application repository.
Copy the separately supplied file to `/home/alex/sonar-temp/compose.yaml`.
It uses embedded H2 for evaluation only. CI only connects to its HTTP endpoint;
it never starts or deletes SonarQube.
Reserve 4 GB RAM for Sonar in addition to the runner/build/application workload.

Run on the server (raise limits only when below the required minimum):

```bash
cd /home/alex/sonar-temp
if [ "$(sysctl -n vm.max_map_count)" -lt 524288 ]; then
  sudo sysctl -w vm.max_map_count=524288
fi
if [ "$(sysctl -n fs.file-max)" -lt 131072 ]; then
  sudo sysctl -w fs.file-max=131072
fi
docker compose up -d
docker compose logs -f --tail=100 sonarqube
```

Ctrl+C exits log viewing, not the server. Sysctl changes above are temporary until
reboot. Wait for SonarQube to report it is operational; startup takes a few minutes.

From your workstation, use your existing SSH username and server address:

```bash
ssh -N -L 9000:127.0.0.1:9000 YOUR_SSH_USER@YOUR_SERVER
```

Open `http://localhost:9000`, sign in initially with `admin` / `admin`, and change
the password. Create two projects with keys `codexhackathon-dev` and
`codexhackathon-main` (separate projects avoid Community branch-analysis limits).
Create an analysis user with Execute Analysis permission on those two projects,
then generate that user's token; do not use an administrator token in CI.

The workflow fixes the URL to `http://127.0.0.1:9000` and selects
`codexhackathon-dev` / `codexhackathon-main` by branch. No repository variables
are needed. In Settings > Secrets and variables > Actions, add only:

| Type | Name | Value |
| --- | --- | --- |
| Secret | `SONAR_TOKEN` | Analysis user's token |

Scanner uses host networking on the Linux runner to reach the loopback-only
SonarQube. The URL works only when Sonar runs on the same host as the runner.
Missing configuration or a failed Quality Gate stops the workflow.

After evaluation, stop and **delete all temporary Sonar analysis/accounts/data**:

```bash
cd /home/alex/sonar-temp
docker compose down --volumes
```

Remove `SONAR_TOKEN` from GitHub when finished. Future pipelines require SonarQube
again; recreate projects/token after deleting its volumes.

References: [Sonar host limits](https://docs.sonarsource.com/sonarqube-community-build/server-installation/pre-installation/linux),
[Sonar database guidance](https://docs.sonarsource.com/sonarqube-community-build/server-installation/installing-the-database),
[self-hosted runner security](https://docs.github.com/en/actions/reference/security/secure-use),
[k6 thresholds](https://grafana.com/docs/k6/latest/testing-guides/api-load-testing/).
