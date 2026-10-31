#!/usr/bin/env bash
# One entry point: bash scripts/seed-olist-demo.sh [docker compose global options]
set -euo pipefail
ROOT_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.." && pwd)"
DATA_REL=kbs/docs/data-structure/olist_cod/olist_cod
DATA_DIR="$ROOT_DIR/$DATA_REL"

if [[ ! -f "$DATA_DIR/seed.sql" ]] || [[ $(wc -c < "$DATA_DIR/seed.sql") -lt 1024 ]]; then
  git -C "$ROOT_DIR" lfs pull --include="$DATA_REL/seed.sql" --exclude=""
fi
(cd "$DATA_DIR" && sha256sum --check seed.sha256)
export OLIST_MANIFEST OLIST_SCENARIOS OLIST_EXPECTATIONS OLIST_SEED_SHA OLIST_STAGE_DB
OLIST_MANIFEST=$(cat "$DATA_DIR/manifest.json")
OLIST_SCENARIOS=$(cat "$DATA_DIR/scenarios.json")
OLIST_EXPECTATIONS=$(cat "$DATA_DIR/evaluation/scenario_expectations.json")
read -r OLIST_SEED_SHA _ < "$DATA_DIR/seed.sha256"
OLIST_STAGE_DB="olist_seed_$(date +%s)_$RANDOM"

# All Compose options are captured once; the caller's working directory remains intact.
COMPOSE_ARGS=("$@")
backend_id=$(docker compose "${COMPOSE_ARGS[@]}" ps -q backend)
if [[ -z "$backend_id" ]] || [[ $(docker inspect --format \
  '{{range .Config.Env}}{{if eq . "DEMO_MODE=true"}}enabled{{end}}{{end}}' "$backend_id") != enabled ]]; then
  printf '%s\n' 'Start backend with DEMO_MODE=true (recreate it after changing .env) before seeding demo phones.' >&2
  exit 1
fi
for account in BUYER SELLER; do
  credential="BOOTSTRAP_${account}_PASSWORD"
  if [[ -z ${!credential:-} ]]; then
    read -r -s -p "Existing or new password for ${account,,}_demo (8-72 bytes): " "$credential"
    printf '\n'
  fi
  export "$credential"
done
pg() {
  docker compose "${COMPOSE_ARGS[@]}" exec -T -e OLIST_STAGE_DB postgres sh -ec '
    export PGPASSWORD="$POSTGRES_PASSWORD"
    exec psql -X -q -v ON_ERROR_STOP=1 -v VERBOSITY=terse -U "$POSTGRES_USER" -d "${1:-$POSTGRES_DB}"
  ' sh "$1"
}
run_python() {
  docker compose "${COMPOSE_ARGS[@]}" exec -T \
    -e OLIST_STAGE_DB -e OLIST_SEED_SHA -e OLIST_MANIFEST -e OLIST_SCENARIOS -e OLIST_EXPECTATIONS -e OLIST_TARGET_DB \
    -e BOOTSTRAP_BUYER_PASSWORD -e BOOTSTRAP_SELLER_PASSWORD \
    kbs /opt/venv/bin/python - "$1" < "$ROOT_DIR/scripts/${2:-seed-olist-demo.py}"
}
# Import, receipt creation and bootstrap must select the same application database.
export OLIST_TARGET_DB
OLIST_TARGET_DB=$(docker compose "${COMPOSE_ARGS[@]}" exec -T postgres printenv POSTGRES_DB)
run_python preflight
seed_log=$(mktemp)
stage_created=false
cleanup() {
  if [[ "$stage_created" == true ]]; then
    docker compose "${COMPOSE_ARGS[@]}" exec -T -e OLIST_STAGE_DB postgres sh -ec '
      export PGPASSWORD="$POSTGRES_PASSWORD"
      dropdb --if-exists -U "$POSTGRES_USER" "$OLIST_STAGE_DB"
    ' || printf 'Could not remove staging database %s; remove it manually.\n' "$OLIST_STAGE_DB" >&2
  fi
  rm -f -- "$seed_log"
  unset BOOTSTRAP_BUYER_PASSWORD BOOTSTRAP_SELLER_PASSWORD
}
trap cleanup EXIT
pg '' <<'SQL'
\getenv stage_db OLIST_STAGE_DB
CREATE DATABASE :"stage_db";
SQL
stage_created=true
printf '%s\n' 'Loading the packaged dataset into an isolated staging database...'
if ! pg "$OLIST_STAGE_DB" < "$DATA_DIR/schema.sql" > /dev/null 2> "$seed_log" ||
   ! pg "$OLIST_STAGE_DB" < "$DATA_DIR/seed.sql" > /dev/null 2>> "$seed_log"; then
  printf '%s\n' 'Dataset load failed; the application database was not imported. Check PostgreSQL server logs.' >&2
  # Report the server error, without echoing the SQL payload or COPY context.
  grep -E '^(ERROR|FATAL):' "$seed_log" >&2 || true
  exit 1
fi
pg "$OLIST_STAGE_DB" <<'SQL'
\getenv app_user APP_DB_USER
\getenv stage_db OLIST_STAGE_DB
REVOKE CONNECT ON DATABASE :"stage_db" FROM PUBLIC;
GRANT CONNECT ON DATABASE :"stage_db" TO :"app_user";
GRANT USAGE ON SCHEMA sfd TO :"app_user";
GRANT SELECT ON ALL TABLES IN SCHEMA sfd TO :"app_user";
SQL
pg '' <<'SQL'
\getenv app_user APP_DB_USER
CREATE TABLE IF NOT EXISTS public.olist_seed_imports (
  seed_sha256 text PRIMARY KEY CHECK (length(seed_sha256)=64),
  source_manifest jsonb NOT NULL,
  imported_at timestamptz NOT NULL DEFAULT now()
);
GRANT SELECT, INSERT ON public.olist_seed_imports TO :"app_user";
SQL
run_python import
bash "$ROOT_DIR/scripts/bootstrap-demo.sh" "${COMPOSE_ARGS[@]}"
run_python verify
run_python phones seed-demo-phones.py
printf '%s\n' 'Done. Log in as seller_demo and select a scenario shop printed above.'
