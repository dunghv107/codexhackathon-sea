#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")/../.."
test_dir=$(mktemp -d /tmp/afd-artifact-test.XXXXXX)
trap 'rm -rf -- "$test_dir"' EXIT
mkdir -p "$test_dir/bin" "$test_dir/scripts"
cp scripts/ci.sh "$test_dir/scripts/ci.sh"
cat > "$test_dir/bin/docker" <<'SH'
#!/bin/sh
printf '%s\n' "$*" >> "$MOCK_DOCKER_LOG"
case "$1 $2" in
  'compose config') printf '%s\n' ci-test/backend:test ci-test/frontend:test ci-test/kbs:test ;;
  'image save') printf 'image archive' > "$4" ;;
  'run --rm') printf 'scanner failed\n'; exit 42 ;;
esac
SH
cat > "$test_dir/bin/python3" <<'SH'
#!/bin/sh
printf 'export attempted\n' > "$MOCK_EXPORT_LOG"
SH
chmod +x "$test_dir/bin/"*
export PATH="$test_dir/bin:$PATH"
export MOCK_DOCKER_LOG="$test_dir/docker.log" MOCK_EXPORT_LOG="$test_dir/export.log"
export COMPOSE_PROJECT_NAME=ci-1-2-1 COMPOSE_ENV_FILES="$test_dir/ci.env"
export IMAGE_PREFIX=ci-test IMAGE_TAG=test CI_CACHE_DIR="$test_dir/cache"
export SONAR_HOST_URL=http://127.0.0.1:9000 SONAR_PROJECT_KEY=test SONAR_TOKEN=test-token
export GITHUB_SHA=1111111111111111111111111111111111111111

bash "$test_dir/scripts/ci.sh" export-images
bash "$test_dir/scripts/ci.sh" import-images
grep -Fq 'image load' "$MOCK_DOCKER_LOG"
: > "$MOCK_DOCKER_LOG"
printf 'corruption' >> "$test_dir/reports/ci/images/images.tar"
if bash "$test_dir/scripts/ci.sh" import-images >"$test_dir/corrupt.log" 2>&1; then
  echo 'Corrupt artifact was accepted' >&2; exit 1
fi
if grep -Fq 'image load' "$MOCK_DOCKER_LOG"; then
  echo 'Corrupt artifact reached Docker' >&2; exit 1
fi

status=0
bash "$test_dir/scripts/ci.sh" sonar >"$test_dir/sonar.log" 2>&1 || status=$?
[[ "$status" == 42 ]]
[[ -f "$MOCK_EXPORT_LOG" ]]
grep -Fq 'scanner failed' "$test_dir/reports/ci/sonar/scanner.log"
echo 'PASS: artifact roundtrip, corruption rejected before load, scanner failure preserved after export'
