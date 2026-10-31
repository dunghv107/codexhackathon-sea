#!/usr/bin/env bash
# Requires Docker and a hydrated LFS seed. Creates and removes its own isolated Compose project.
set -euo pipefail
cd "$(dirname "$0")/.."
ROOT_DIR=$PWD
qa_dir=$(mktemp -d)
qa_project="olist-seed-test-$(date +%s)-$RANDOM"
export SEED_TEST_PASSWORD
SEED_TEST_PASSWORD=$(openssl rand -hex 24)
export BOOTSTRAP_BUYER_PASSWORD="qa-buyer-'quote" BOOTSTRAP_SELLER_PASSWORD=qa-seller-password
cat > "$qa_dir/compose.yaml" <<'YAML'
services:
  postgres:
    image: postgres:17-alpine@sha256:266d3c257594e195b309ae9d1805e54c788f1d7b375f56bf6c367aac6c917e7b
    environment:
      POSTGRES_USER: postgres
      POSTGRES_PASSWORD: ${SEED_TEST_PASSWORD}
      POSTGRES_DB: seed_test
      APP_DB_USER: app
      APP_DB_PASSWORD: ${SEED_TEST_PASSWORD}
    volumes:
      - ./docker/postgres-init.sh:/docker-entrypoint-initdb.d/10-app-role.sh:ro
      - seed_test_data:/var/lib/postgresql/data
    healthcheck:
      test: [CMD-SHELL, 'pg_isready -U postgres -d seed_test']
      interval: 2s
      retries: 30
  kbs:
    image: codexhackathon/kbs:seed-test
    environment:
      DATABASE_URL: postgresql://app:${SEED_TEST_PASSWORD}@postgres:5432/seed_test
      ENV_FILE: /dev/null
      MCP_ENABLED: 'false'
  backend:
    image: codexhackathon/backend:seed-test
    environment:
      DB_HOST: postgres
      DB_USER: app
      DB_PASSWORD: ${SEED_TEST_PASSWORD}
      DB_NAME: seed_test
      DB_SSLMODE: disable
      JWT_SECRET: ${SEED_TEST_PASSWORD}
      PHONE_HMAC_SECRET: ${SEED_TEST_PASSWORD}
      DEMO_MODE: ${SEED_TEST_DEMO_MODE:-true}
      RATE_LIMIT_ENABLED: 'false'
      KBS_URL: http://kbs:1114
      LOG_FILE_PATH: /tmp/app.log
volumes:
  seed_test_data:
YAML
args=(--project-directory "$ROOT_DIR" -p "$qa_project" -f "$qa_dir/compose.yaml")
cleanup() {
  docker compose "${args[@]}" down -v >/dev/null
  rm -f -- "$qa_dir/compose.yaml" "$qa_dir/conflict.log" "$qa_dir/phone-failure.log" "$qa_dir/mismatch.yaml" "$qa_dir/mismatch.log"
  rmdir -- "$qa_dir"
  unset SEED_TEST_PASSWORD BOOTSTRAP_BUYER_PASSWORD BOOTSTRAP_SELLER_PASSWORD
}
trap cleanup EXIT
query() { docker compose "${args[@]}" exec -T postgres psql -X -qAt -v ON_ERROR_STOP=1 -U postgres -d seed_test "$@"; }
docker compose "${args[@]}" up -d --wait postgres
for migration in backend/migrations/*.up.sql; do query < "$migration" > /dev/null; done
SEED_TEST_DEMO_MODE=false docker compose "${args[@]}" up -d --wait --wait-timeout 120 kbs backend
if bash scripts/seed-olist-demo.sh "${args[@]}" > "$qa_dir/phone-failure.log" 2>&1; then
  echo 'FAIL: accepted disabled demo mode' >&2; exit 1
fi
grep -q 'DEMO_MODE=true' "$qa_dir/phone-failure.log"
[[ $(query -c 'SELECT count(*) FROM sfd.checkouts') == 0 ]]
docker compose "${args[@]}" up -d --wait --wait-timeout 120 backend
export OLIST_SCENARIOS OLIST_SEED_SHA
OLIST_SCENARIOS=$(cat kbs/docs/data-structure/olist_cod/olist_cod/scenarios.json)
read -r OLIST_SEED_SHA _ < kbs/docs/data-structure/olist_cod/olist_cod/seed.sha256
phone_setup() {
  docker compose "${args[@]}" exec -T -e OLIST_SCENARIOS -e OLIST_SEED_SHA \
    -e BOOTSTRAP_BUYER_PASSWORD -e BOOTSTRAP_SELLER_PASSWORD \
    kbs /opt/venv/bin/python - < scripts/seed-demo-phones.py
}
bash scripts/bootstrap-demo.sh "${args[@]}"
if phone_setup > "$qa_dir/phone-failure.log" 2>&1; then
  echo 'FAIL: accepted a database with no phone history' >&2; exit 1
fi
grep -q 'No mapping was written' "$qa_dir/phone-failure.log"
[[ $(query -c 'SELECT count(*) FROM sfd.orders') == 0 ]]
[[ $(query -c 'SELECT count(*) FROM sfd.demo_phone_mappings') == 0 ]]
if BOOTSTRAP_BUYER_PASSWORD=wrong-password phone_setup > "$qa_dir/phone-failure.log" 2>&1; then
  echo 'FAIL: accepted incorrect demo credentials' >&2; exit 1
fi
grep -q 'Cannot log in as buyer_demo' "$qa_dir/phone-failure.log"

# Reject split database configuration before creating a stage or import receipt.
cat > "$qa_dir/mismatch.yaml" <<'YAML'
services:
  postgres:
    environment:
      POSTGRES_DB: other_database
YAML
docker compose "${args[@]}" -f "$qa_dir/mismatch.yaml" up -d --wait --no-deps postgres
if bash scripts/seed-olist-demo.sh "${args[@]}" > "$qa_dir/mismatch.log" 2>&1; then
  echo 'FAIL: accepted different import and bootstrap database targets' >&2; exit 1
fi
grep -q 'KBS database differs from PostgreSQL POSTGRES_DB' "$qa_dir/mismatch.log"
[[ $(query -c "SELECT to_regclass('public.olist_seed_imports') IS NULL") == t ]]
[[ $(query -c "SELECT count(*) FROM pg_database WHERE datname LIKE 'olist_seed_%'") == 0 ]]
docker compose "${args[@]}" up -d --wait --no-deps postgres
docker compose "${args[@]}" restart kbs
docker compose "${args[@]}" up -d --wait --wait-timeout 120 kbs

# A colliding source ID must roll back even the preceding tables in the import.
query <<'SQL'
INSERT INTO sfd.shops(shop_id,display_name,batch_id,source_record_id)
VALUES ('sim:da8622b14eb17ae2831f4ac5b9dab84a','Collision sentinel','olist_vn_sim','seed-test:collision');
SQL
if bash scripts/seed-olist-demo.sh "${args[@]}" > "$qa_dir/conflict.log" 2>&1; then
  echo 'FAIL: imported over a conflicting source ID' >&2; exit 1
fi
if ! grep -q 'UniqueViolation' "$qa_dir/conflict.log"; then
  cat "$qa_dir/conflict.log" >&2
  echo 'FAIL: did not reach the intended duplicate-ID check' >&2; exit 1
fi
[[ $(query -c 'SELECT count(*) FROM sfd.buyers') == 1 ]]
[[ $(query -c 'SELECT count(*) FROM public.olist_seed_imports') == 0 ]]
[[ $(query -c "SELECT count(*) FROM pg_database WHERE datname LIKE 'olist_seed_%'") == 0 ]]
query -c "DELETE FROM sfd.shops WHERE source_record_id='seed-test:collision'"

bash scripts/seed-olist-demo.sh "${args[@]}"
[[ $(query -c 'SELECT count(*) FROM sfd.orders') == 61207 ]]
[[ $(query -c 'SELECT count(*) FROM sfd.checkouts') == 60003 ]]
[[ $(query -c 'SELECT count(*) FROM sfd.buyers') == 58741 ]]
[[ $(query -c 'SELECT count(*) FROM public.olist_seed_imports') == 1 ]]
[[ $(query -c "SELECT count(*) FROM sfd.shops WHERE owner_seller_id='live:bootstrap-seller'") -gt 1 ]]
query -c "UPDATE sfd.listings SET stock_quantity=7,current_price=99000 WHERE listing_id='live:bootstrap-listing-1'"
hashes=$(query -c 'SELECT password FROM public.users ORDER BY username')
bash scripts/seed-olist-demo.sh "${args[@]}"
[[ $(query -c 'SELECT count(*) FROM sfd.orders') == 61207 ]]
[[ $(query -c 'SELECT count(*) FROM sfd.buyers') == 58741 ]]
[[ $(query -c 'SELECT password FROM public.users ORDER BY username') == "$hashes" ]]
[[ $(query -c "SELECT stock_quantity || ':' || current_price FROM sfd.listings WHERE listing_id='live:bootstrap-listing-1'") == '7:99000.00' ]]
[[ $(query -c "SELECT source_system FROM sfd.data_batches WHERE batch_id='olist_vn_sim'") == marketplace_demo ]]
[[ $(query -c "SELECT count(*) FROM pg_database WHERE datname LIKE 'olist_seed_%'") == 0 ]]
[[ $(query -c 'SELECT count(*) FROM sfd.demo_phone_mappings') == 3 ]]
[[ $(query -c "SELECT count(*) FROM sfd.order_verifications WHERE status='pending'") == 1 ]]

# Preserve an existing mapping that no longer meets its intended level; leave no probe orders behind.
query -c "UPDATE sfd.demo_phone_mappings SET phone_key=(SELECT phone_key FROM sfd.demo_phone_mappings WHERE scenario_label='low') WHERE scenario_label='medium'"
if phone_setup > "$qa_dir/phone-failure.log" 2>&1; then
  echo 'FAIL: accepted a Medium mapping that now scores Low' >&2; exit 1
fi
grep -q 'Existing mapping was preserved' "$qa_dir/phone-failure.log"
[[ $(query -c 'SELECT count(*) FROM sfd.orders') == 61207 ]]
[[ $(query -c "SELECT count(DISTINCT phone_key) FROM sfd.demo_phone_mappings") == 2 ]]
echo 'PASS: dataset, rollback, rerun, preserved live edits, three mapped Go checkouts, Medium verification and mapping drift'
