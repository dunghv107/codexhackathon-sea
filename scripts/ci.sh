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
CI_CACHE_DIR=${CI_CACHE_DIR:-${RUNNER_TEMP:-/tmp}/codex-ci-cache}
mkdir -p reports/ci "$CI_CACHE_DIR"

case "${1:-}" in
  prepare)
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
    bash tests/ci/artifacts.sh
    python3 -m unittest discover -s tests/ci -p 'test_*.py' -v
    for service in backend frontend kbs; do
      docker build --target test --tag "$IMAGE_PREFIX/$service-test:$IMAGE_TAG" "$service" 2>&1 | tee "reports/ci/test-$service.log"
    done
    mkdir -p reports/ci/coverage
    for service in backend kbs; do
      container=$(docker create "$IMAGE_PREFIX/$service-test:$IMAGE_TAG" /bin/true)
      trap 'docker rm "$container" >/dev/null' EXIT
      docker cp "$container:/reports/." reports/ci/coverage/
      docker rm "$container" >/dev/null
      trap - EXIT
    done
    python3 - <<'PY'
from pathlib import Path
import xml.etree.ElementTree as ET

path = Path('reports/ci/coverage/coverage-python.xml')
tree = ET.parse(path)
for source in tree.findall('./sources/source'):
    source.text = 'kbs/app'
tree.write(path, encoding='utf-8', xml_declaration=True)
PY
    ;;
  sonar)
    : "${SONAR_HOST_URL:?}" "${SONAR_PROJECT_KEY:?}" "${SONAR_TOKEN:?}"
    mkdir -p reports/ci/sonar "$CI_CACHE_DIR/sonar"
    rm -f -- reports/ci/sonar/{report-task.txt,analysis.json,quality-gate.json,measures.json,issues.json,export-error.json}
    scanner_status=0
    docker run --rm --user "$(id -u):$(id -g)" --cap-drop ALL \
      --security-opt no-new-privileges --network host \
      -e SONAR_HOST_URL -e SONAR_TOKEN -e SONAR_USER_HOME=/tmp \
      --tmpfs /usr/src/reports:rw,noexec,nosuid,size=16m \
      -v "$CI_CACHE_DIR/sonar:/tmp/cache" -v "$PWD/reports/ci:/reports" \
      -v "$PWD:/usr/src:ro" "$SONAR_IMAGE" \
      "-Dsonar.projectKey=$SONAR_PROJECT_KEY" \
      "-Dsonar.scm.revision=${GITHUB_SHA:-$(git rev-parse HEAD)}" \
      -Dsonar.scanner.metadataFilePath=/reports/sonar/report-task.txt \
      -Dsonar.go.coverage.reportPaths=/reports/coverage/coverage-go.out \
      -Dsonar.python.coverage.reportPaths=/reports/coverage/coverage-python.xml \
      -Dsonar.working.directory=/tmp/sonar-work \
      -Dsonar.qualitygate.wait=false \
      2>&1 | tee reports/ci/sonar/scanner.log || scanner_status=$?
    export_status=0
    python3 scripts/sonar-report.py || export_status=$?
    (( scanner_status == 0 )) || exit "$scanner_status"
    exit "$export_status"
    ;;
  scan)
    # No Docker socket in scanners: export the exact locally built image instead.
    mkdir -p reports/ci/trivy "$CI_CACHE_DIR/trivy"
    raw_report=$(mktemp "${RUNNER_TEMP:-/tmp}/ci-trivy.XXXXXX.json")
    archive=$(mktemp "${RUNNER_TEMP:-/tmp}/ci-image.XXXXXX.tar")
    trap 'rm -f -- "$archive" "$raw_report"' EXIT
    scan_status=0
    docker run --rm --user "$(id -u):$(id -g)" --cap-drop ALL \
      --security-opt no-new-privileges -v "$PWD:/src:ro" \
      -v "$CI_CACHE_DIR/trivy:/tmp/trivy" -v "$raw_report:/report.json" \
      "$TRIVY_IMAGE" fs --cache-dir /tmp/trivy --exit-code 1 \
      --format json --output /report.json \
      --scanners vuln,misconfig,secret --severity HIGH,CRITICAL \
      --skip-dirs /src/.git,/src/reports --skip-files /src/.env /src || scan_status=$?
    python3 scripts/redact-trivy.py < "$raw_report" > reports/ci/trivy/source.json
    (( scan_status == 0 )) || exit "$scan_status"
    docker compose pull postgres redis migrate
    # shortcut: runtime CVEs are report-only by user approval; restore the gate after base-image remediation.
    echo '::warning::Runtime CVEs are report-only. Review image scan output; source findings and scanner errors still block.'
    index=0
    : > reports/ci/trivy/images.tsv
    while IFS= read -r image; do
      index=$((index + 1))
      docker image save --output "$archive" "$image"
      docker run --rm --user "$(id -u):$(id -g)" --cap-drop ALL \
        --security-opt no-new-privileges -v "$archive:/image.tar:ro" -v "$PWD/reports/ci/trivy:/reports" \
        -v "$CI_CACHE_DIR/trivy:/tmp/trivy" \
        "$TRIVY_IMAGE" image --cache-dir /tmp/trivy --input /image.tar \
        --format json --output "/reports/image-$index.json" \
        --scanners vuln --severity HIGH,CRITICAL --exit-code 0
      printf '%s\t%s\n' "image-$index.json" "$image" >> reports/ci/trivy/images.tsv
    done < <(docker compose config --images | sort -u)
    ;;
  export-images)
    mkdir -p reports/ci/images
    mapfile -t images < <(docker compose config --images | sort -u)
    docker image save --output reports/ci/images/images.tar "${images[@]}"
    (cd reports/ci/images && sha256sum images.tar > images.tar.sha256)
    ;;
  import-images)
    (cd reports/ci/images && sha256sum -c images.tar.sha256)
    docker image load --input reports/ci/images/images.tar
    while IFS= read -r image; do docker image inspect "$image" >/dev/null; done \
      < <(docker compose config --images | sort -u)
    ;;
  load)
    mkdir -p reports/ci/k6
    docker run --rm --user "$(id -u):$(id -g)" --cap-drop ALL \
      --security-opt no-new-privileges --network "${COMPOSE_PROJECT_NAME}_app" \
      -v "$PWD/tests/load:/tests:ro" -v "$PWD/reports/ci/k6:/reports" \
      "$K6_IMAGE" run --summary-export=/reports/summary.json /tests/baseline.js \
      2>&1 | tee reports/ci/k6/output.log
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
  *) echo 'Usage: ci.sh prepare|test|sonar|scan|export-images|import-images|load|cleanup' >&2; exit 2 ;;
esac
