#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")/.."

[[ ${COMPOSE_PROJECT_NAME:-} =~ ^ci-[0-9]+-[0-9]+-[0-9]+$ ]] || exit 1
: "${COMPOSE_ENV_FILES:?}" "${IMAGE_PREFIX:?}" "${IMAGE_TAG:?}"
if (( $# == 0 )); then set -- backend kbs; fi
run_backend=false
run_kbs=false
for service in "$@"; do
  case "$service" in
    backend) run_backend=true ;;
    kbs) run_kbs=true ;;
    *) echo 'Expected backend or kbs integration suite' >&2; exit 2 ;;
  esac
done
test_env=$(mktemp "${RUNNER_TEMP:-/tmp}/ci-integration.XXXXXX.env")
trap 'rm -f -- "$test_env"' EXIT
export test_env
python3 - <<'PY'
import os
from pathlib import Path
from urllib.parse import quote

values = dict(line.split('=', 1) for line in Path(os.environ['COMPOSE_ENV_FILES']).read_text().splitlines() if '=' in line)
password = quote(values['DB_ADMIN_PASSWORD'], safe='')
database = quote(os.environ.get('DB_NAME', 'ai_fraud_detection'), safe='')
url = f'postgresql://postgres:{password}@postgres:5432/{database}?sslmode=disable'
bootstrap_url = f'postgresql://postgres:{password}@postgres:5432/bootstrap_demo_test?sslmode=disable'
Path(os.environ['test_env']).write_text(
    f'TEST_DATABASE_URL={url}\nKBS_TEST_DATABASE_URL={url}\nSEED_TEST_DATABASE_URL={url}\n'
    f'BOOTSTRAP_TEST_DATABASE_URL={bootstrap_url}\n'
    'TEST_REDIS_URL=redis://redis:6379/15\n'
)
PY

docker compose up -d --wait --wait-timeout 120 postgres redis
# Canonical KBS fixtures use the original schema; backend tests own separate schemas.
docker compose exec -T postgres psql -v ON_ERROR_STOP=1 -U postgres -d "${DB_NAME:-ai_fraud_detection}" \
  < olist_cod/olist_cod/schema.sql > reports/ci/test-schema.log
mkdir -p reports/ci/coverage
if $run_backend; then
docker run --rm --network "${COMPOSE_PROJECT_NAME}_data" --env-file "$test_env" \
  -v "$PWD/reports/ci/coverage:/reports" \
  -v "$PWD/olist_cod/olist_cod/schema.sql:/olist_cod/olist_cod/schema.sql:ro" \
  "$IMAGE_PREFIX/backend-test:$IMAGE_TAG" \
  go test -count=1 -coverpkg=./... -coverprofile=/reports/coverage-go.out ./... \
  2>&1 | tee reports/ci/integration-backend.log
fi
if $run_kbs; then
docker run --rm --network "${COMPOSE_PROJECT_NAME}_data" --env-file "$test_env" \
  -e COVERAGE_FILE=/tmp/.coverage -v "$PWD/reports/ci/coverage:/reports" \
  -v "$PWD/backend/migrations:/backend/migrations:ro" \
  --entrypoint /opt/venv/bin/python "$IMAGE_PREFIX/kbs-test:$IMAGE_TAG" \
  -m pytest -m 'not acceptance and not live and not slow' \
  --cov=app --cov-fail-under=90 --cov-report=xml:/reports/coverage-python.xml \
  --cov-report=term-missing --junitxml=/reports/pytest.xml \
  2>&1 | tee reports/ci/integration-kbs.log
docker run --rm --network "${COMPOSE_PROJECT_NAME}_data" --env-file "$test_env" \
  --cap-drop ALL --security-opt no-new-privileges \
  -v "$PWD:/workspace:ro" -w /workspace --entrypoint /opt/venv/bin/python \
  "$IMAGE_PREFIX/kbs-test:$IMAGE_TAG" -m unittest discover -s tests/scripts -v \
  2>&1 | tee reports/ci/test-scripts.log
docker compose exec -T postgres createdb -U postgres bootstrap_demo_test
docker run --rm --network "${COMPOSE_PROJECT_NAME}_data" --env-file "$test_env" \
  --cap-drop ALL --security-opt no-new-privileges \
  -v "$PWD:/workspace:ro" -w /workspace --entrypoint /bin/bash \
  postgres:17@sha256:31ba9c94a03ada9988f2bb493a22a75eef3a185884533e5cdbcd498ea835b08f \
  tests/bootstrap-demo.sh \
  2>&1 | tee reports/ci/test-bootstrap-demo.log
docker run --rm --network "${COMPOSE_PROJECT_NAME}_data" --env-file "$test_env" \
  -e ALLOW_DEMO_SCORE=true \
  --cap-drop ALL --security-opt no-new-privileges \
  -v "$PWD:/workspace:ro" -w /workspace --entrypoint /opt/venv/bin/python \
  "$IMAGE_PREFIX/kbs-test:$IMAGE_TAG" scripts/score-sale-burst-demo.py \
  2>&1 | tee reports/ci/test-sale-burst-rules.log
fi
