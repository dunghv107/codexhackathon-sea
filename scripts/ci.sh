#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")/.."

TRIVY_IMAGE=aquasec/trivy:0.76.1@sha256:790fd66d1697395d8693d1acaa1ea702406a20f109b6fc7bac8db623eca6c06c
SONAR_IMAGE=sonarsource/sonar-scanner-cli:11@sha256:46341f2c2dc560381414e90347885f5cddc716db080adcbafc1cac13e1473094
K6_IMAGE=grafana/k6:1.6.1@sha256:a5ad6bc089a08d77c3ec49f3db8c6fa7a148e4073efcac44c675dbaf3568d8e1

# Never let cleanup or test credentials target the developer/deployment stack.
[[ ${COMPOSE_PROJECT_NAME:-} =~ ^ci-[0-9]+-[0-9]+-[0-9]+$ ]] || {
  echo 'Expected an isolated ci-<repository id>-<run id>-<attempt> project' >&2; exit 1;
}
: "${COMPOSE_ENV_FILES:?}" "${IMAGE_PREFIX:?}" "${IMAGE_TAG:?}"

case "${1:-}" in
  prepare)
    : "${SONAR_HOST_URL:?Set SONAR_HOST_URL in the workflow}"
    : "${SONAR_PROJECT_KEY:?Set SONAR_PROJECT_KEY in the workflow}"
    : "${SONAR_TOKEN:?Set SONAR_TOKEN repository secret}"
    docker info >/dev/null
    docker compose version
    docker buildx version
    python3 - <<'PY'
import os
import secrets
from pathlib import Path

path = Path(os.environ['COMPOSE_ENV_FILES'])
descriptor = os.open(path, os.O_WRONLY | os.O_CREAT | os.O_EXCL, 0o600)
with os.fdopen(descriptor, 'w') as stream:
    for key in ('DB_PASSWORD', 'DB_ADMIN_PASSWORD', 'JWT_SECRET'):
        stream.write(f'{key}={secrets.token_hex(32)}\n')
    # Isolated load test only: proxy IP currently shares one rate-limit bucket.
    stream.write('RATE_LIMIT_REQUESTS_PER_MIN=10000\n')
PY
    docker compose config --quiet
    ;;
  test)
    bash tests/ci/deploy.sh
    for service in backend frontend kbs; do
      docker build --target test --tag "$IMAGE_PREFIX/$service-test:$IMAGE_TAG" "$service"
    done
    ;;
  sonar)
    docker run --rm --user "$(id -u):$(id -g)" --cap-drop ALL \
      --security-opt no-new-privileges --network host \
      -e SONAR_HOST_URL -e SONAR_TOKEN -e SONAR_USER_HOME=/tmp/sonar \
      -v "$PWD:/usr/src:ro" "$SONAR_IMAGE" \
      "-Dsonar.projectKey=$SONAR_PROJECT_KEY" \
      -Dsonar.working.directory=/tmp/sonar-work \
      -Dsonar.qualitygate.wait=true -Dsonar.qualitygate.timeout=300
    ;;
  scan)
    # No Docker socket in scanners: export the exact locally built image instead.
    mkdir -p reports
    docker run --rm --user "$(id -u):$(id -g)" --cap-drop ALL \
      --security-opt no-new-privileges -v "$PWD:/src:ro" \
      "$TRIVY_IMAGE" fs --cache-dir /tmp/trivy --exit-code 1 \
      --scanners vuln,misconfig,secret --severity HIGH,CRITICAL \
      --skip-dirs /src/.git,/src/reports --skip-files /src/.env /src
    archive=$(mktemp "${RUNNER_TEMP:-/tmp}/ci-image.XXXXXX.tar")
    trap 'rm -f -- "$archive"' EXIT
    docker compose pull postgres redis migrate
    # shortcut: runtime CVEs are report-only by user approval; restore the gate after base-image remediation.
    echo '::warning::Runtime CVEs are report-only. Review image scan output; source findings and scanner errors still block.'
    while IFS= read -r image; do
      docker image save --output "$archive" "$image"
      docker run --rm --user "$(id -u):$(id -g)" --cap-drop ALL \
        --security-opt no-new-privileges -v "$archive:/image.tar:ro" \
        "$TRIVY_IMAGE" image --cache-dir /tmp/trivy --input /image.tar \
        --scanners vuln --severity HIGH,CRITICAL --exit-code 0
    done < <(docker compose config --images | sort -u)
    ;;
  load)
    docker run --rm --user "$(id -u):$(id -g)" --cap-drop ALL \
      --security-opt no-new-privileges --network "${COMPOSE_PROJECT_NAME}_app" \
      -v "$PWD/tests/load:/tests:ro" "$K6_IMAGE" run /tests/baseline.js
    ;;
  cleanup)
    # A failure before prepare created credentials has no stack to remove.
    if [[ -f "$COMPOSE_ENV_FILES" ]]; then
      docker compose down --volumes --remove-orphans --timeout 30
      rm -f -- "$COMPOSE_ENV_FILES"
    fi
    # Remove only uniquely tagged images for this run, never prune the daemon.
    for service in backend frontend kbs; do
      docker image rm "$IMAGE_PREFIX/$service:$IMAGE_TAG-runtime" \
        "$IMAGE_PREFIX/$service-test:$IMAGE_TAG" 2>/dev/null || true
    done
    ;;
  *) echo 'Usage: ci.sh prepare|test|sonar|scan|load|cleanup' >&2; exit 2 ;;
esac
