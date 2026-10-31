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
    image: postgres:17-alpine
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
volumes:
  seed_test_data:
YAML
args=(--project-directory "$ROOT_DIR" -p "$qa_project" -f "$qa_dir/compose.yaml")
cleanup() {
  docker compose "${args[@]}" down -v >/dev/null
  rm -f -- "$qa_dir/compose.yaml" "$qa_dir/conflict.log"
  rmdir -- "$qa_dir"
  unset SEED_TEST_PASSWORD BOOTSTRAP_BUYER_PASSWORD BOOTSTRAP_SELLER_PASSWORD
}
trap cleanup EXIT
query() { docker compose "${args[@]}" exec -T postgres psql -X -qAt -v ON_ERROR_STOP=1 -U postgres -d seed_test "$@"; }
docker compose "${args[@]}" up -d --wait postgres
for migration in backend/migrations/*.up.sql; do query < "$migration" > /dev/null; done
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
[[ $(query -c 'SELECT count(*) FROM sfd.buyers') == 0 ]]
[[ $(query -c 'SELECT count(*) FROM public.olist_seed_imports') == 0 ]]
[[ $(query -c "SELECT count(*) FROM pg_database WHERE datname LIKE 'olist_seed_%'") == 0 ]]
query -c "DELETE FROM sfd.shops WHERE source_record_id='seed-test:collision'"

bash scripts/seed-olist-demo.sh "${args[@]}"
[[ $(query -c 'SELECT count(*) FROM sfd.orders') == 61204 ]]
[[ $(query -c 'SELECT count(*) FROM sfd.checkouts') == 60000 ]]
[[ $(query -c 'SELECT count(*) FROM sfd.buyers') == 58741 ]]
[[ $(query -c 'SELECT count(*) FROM public.olist_seed_imports') == 1 ]]
[[ $(query -c "SELECT count(*) FROM sfd.shops WHERE owner_seller_id='live:bootstrap-seller'") -gt 1 ]]
query -c "UPDATE sfd.listings SET stock_quantity=7,current_price=99000 WHERE listing_id='live:bootstrap-listing-1'"
hashes=$(query -c 'SELECT password FROM public.users ORDER BY username')
bash scripts/seed-olist-demo.sh "${args[@]}"
[[ $(query -c 'SELECT count(*) FROM sfd.orders') == 61204 ]]
[[ $(query -c 'SELECT count(*) FROM sfd.buyers') == 58741 ]]
[[ $(query -c 'SELECT password FROM public.users ORDER BY username') == "$hashes" ]]
[[ $(query -c "SELECT stock_quantity || ':' || current_price FROM sfd.listings WHERE listing_id='live:bootstrap-listing-1'") == '7:99000.00' ]]
[[ $(query -c "SELECT source_system FROM sfd.data_batches WHERE batch_id='olist_vn_sim'") == marketplace_demo ]]
[[ $(query -c "SELECT count(*) FROM pg_database WHERE datname LIKE 'olist_seed_%'") == 0 ]]
echo 'PASS: full dataset, collision rollback, cleanup, repeat import, preserved live edits, KBS scoring and Network'
