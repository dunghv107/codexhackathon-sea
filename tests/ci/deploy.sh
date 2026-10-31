#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")/../.."
test_dir=$(mktemp -d /tmp/afd-deploy-test.XXXXXX)
trap 'rm -rf -- "$test_dir"' EXIT
mkdir -p "$test_dir/bin" "$test_dir/deploy"
cat > "$test_dir/bin/docker" <<'SH'
#!/bin/sh
printf '%s|%s|%s\n' "$COMPOSE_PROJECT_NAME" "$COMPOSE_ENV_FILES" "$*" >> "$MOCK_DOCKER_LOG"
if [ "$*" = 'compose run --rm --no-deps migrate' ]; then
  exit "${MOCK_MIGRATION_FAILURE:-0}"
fi
SH
cat > "$test_dir/bin/python3" <<'SH'
#!/bin/sh
cat >/dev/null
SH
chmod +x "$test_dir/bin/docker" "$test_dir/bin/python3"
export PATH="$test_dir/bin:$PATH" AFD_DEPLOY_DIR="$test_dir/deploy"
export MOCK_DOCKER_LOG="$test_dir/docker.log"
export IMAGE_PREFIX=ci-test IMAGE_TAG=test-candidate
export COMPOSE_PROJECT_NAME=ci-test COMPOSE_ENV_FILES=/tmp/ci-test.env
export GITHUB_SHA=1111111111111111111111111111111111111111

# A missing persistent credential file must fail before touching Docker.
if bash scripts/deploy.sh >"$test_dir/missing-env.log" 2>&1; then
  echo 'Deployment accepted missing credentials' >&2; exit 1
fi
[[ ! -e "$MOCK_DOCKER_LOG" ]]
printf 'DB_PASSWORD=keep-existing-credentials\n' > "$AFD_DEPLOY_DIR/.env"
cp "$AFD_DEPLOY_DIR/.env" "$test_dir/expected.env"
bash scripts/deploy.sh
cmp "$AFD_DEPLOY_DIR/.env" "$test_dir/expected.env"
[[ $(readlink "$AFD_DEPLOY_DIR/current") == "$AFD_DEPLOY_DIR/releases/$GITHUB_SHA" ]]
grep -Fq "image tag ci-test/backend:test-candidate-runtime afd/backend:$GITHUB_SHA-runtime" "$MOCK_DOCKER_LOG"
grep -Fq "afd|$AFD_DEPLOY_DIR/.env|compose run --rm --no-deps migrate" "$MOCK_DOCKER_LOG"
if grep -q 'compose down' "$MOCK_DOCKER_LOG"; then exit 1; fi

# Migration failure must keep the last successful release and skip app replacement.
previous_release=$(readlink "$AFD_DEPLOY_DIR/current")
export GITHUB_SHA=2222222222222222222222222222222222222222 MOCK_MIGRATION_FAILURE=1
: > "$MOCK_DOCKER_LOG"
if bash scripts/deploy.sh >"$test_dir/failed-migration.log" 2>&1; then
  echo 'Deployment ignored migration failure' >&2; exit 1
fi
[[ $(readlink "$AFD_DEPLOY_DIR/current") == "$previous_release" ]]
if grep -q 'compose up .*backend kbs frontend' "$MOCK_DOCKER_LOG"; then exit 1; fi
cmp "$AFD_DEPLOY_DIR/.env" "$test_dir/expected.env"
echo 'PASS: deployment guards, tested-image tags, secret isolation and migration failure'
