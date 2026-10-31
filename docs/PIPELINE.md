# Self-hosted pipeline

Target repository: `dunghv107/codexhackathon-sea`, branches `dev` and `main`.
The runner must have the standard `self-hosted`, `Linux`, `X64` labels.
Install Docker Engine, Compose v2 (with `COMPOSE_ENV_FILES` support), Buildx,
Git and Python 3. The `github-runner` user must be able to run `docker info`.
Restart the runner service after changing group membership. Docker access is
effectively host administrator access: only trusted maintainers may push these
branches. This workflow does not execute pull requests or fork code.

The workflow runs lint/unit tests in the existing Docker test stages, SonarQube
SAST with a mandatory Quality Gate, runtime builds, Trivy source/configuration/
secret scanning and image vulnerability scanning, isolated Compose startup,
security/HTTP smoke checks, then k6. HIGH/CRITICAL findings fail the scan, including
unfixed vulnerabilities. Fix or review findings; do not silently disable gates.
Builds and scanner images are kept separate from application runtime images.

CI uses port 18080 and a unique Compose project, database volume and credentials
for each run. Cleanup removes only that run's resources, even after failure.
No daemon-wide prune. Hard termination/host failure can leave resources behind;
inspect Compose labels before cleaning them. BuildKit cache remains on the host.
Results appear in job logs. Secret scan output must be accessible only to trusted
repository users. The temporary credential file is outside the checkout.

The k6 baseline sends five iterations/second for 30 seconds, each fetching the
catalog and KBS readiness. It requires all checks to pass, <1% HTTP errors,
p95 <500 ms for each service, and no dropped iterations. It raises only the
temporary stack's rate limit to avoid the existing shared nginx-IP bucket.
This is a small regression check with an empty database, not a real-user capacity
benchmark. Database-dependent unit/integration suites can skip without their
dedicated fixture/database settings. No coverage reports are imported into Sonar;
a Quality Gate requiring coverage may fail until coverage collection is added.

This pipeline does not publish images or replace a persistent deployment.
The Compose test stack is removed at the end. Production deployment needs its own
persistent credentials, migration/backup policy and endpoint configuration.

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
