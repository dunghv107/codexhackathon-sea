#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")/.."

: "${IMAGE_PREFIX:?}" "${IMAGE_TAG:?}" "${GITHUB_SHA:?}"
[[ "$GITHUB_SHA" =~ ^[0-9a-f]{40}$ ]] || { echo 'Invalid release revision' >&2; exit 1; }
deploy_dir=${AFD_DEPLOY_DIR:-/home/alex/codexhackathon-sea}
[[ -d "$deploy_dir" && -w "$deploy_dir" && -f "$deploy_dir/.env" ]] || {
  echo 'Provision the deployment directory and persistent .env first' >&2; exit 1;
}
[[ ! -e "$deploy_dir/current" || -L "$deploy_dir/current" ]] || {
  echo 'Deployment current path must be a symlink' >&2; exit 1;
}

# Keep the exact tested images under separate tags so CI cleanup cannot remove them.
for service in backend frontend kbs; do
  docker image tag "$IMAGE_PREFIX/$service:$IMAGE_TAG-runtime" "afd/$service:$GITHUB_SHA-runtime"
done

release="$deploy_dir/releases/$GITHUB_SHA"
mkdir -p "$release/backend/migrations" "$release/docker"
cp compose.yaml "$release/compose.yaml"
cp backend/migrations/*.sql "$release/backend/migrations/"
cp docker/postgres-init.sh "$release/docker/"

# Override CI settings; the persistent secret file is never copied from the CI stack.
export COMPOSE_PROJECT_NAME=afd COMPOSE_FILE="$release/compose.yaml"
export COMPOSE_ENV_FILES="$deploy_dir/.env" COMPOSE_DISABLE_ENV_FILE=true
export IMAGE_PREFIX=afd IMAGE_TAG="$GITHUB_SHA" RUNTIME_VARIANT=runtime
export FRONTEND_BIND_IP=192.168.2.14 FRONTEND_PORT=18081 COOKIE_SECURE=true
cat > "$release/release.env" <<EOF
COMPOSE_PROJECT_NAME=afd
IMAGE_PREFIX=afd
IMAGE_TAG=$GITHUB_SHA
RUNTIME_VARIANT=runtime
FRONTEND_BIND_IP=192.168.2.14
FRONTEND_PORT=18081
COOKIE_SECURE=true
EOF

docker compose config --quiet
docker compose up -d --no-build --wait --wait-timeout 180 postgres redis
# Run migrations explicitly on every release; preserve data if migration fails.
docker compose run --rm --no-deps migrate
docker compose up -d --no-build --no-deps --wait --wait-timeout 180 backend kbs frontend
python3 - <<'PY'
from urllib.request import urlopen

for route in ('/health', '/', '/api/catalog/categories'):
    with urlopen('http://192.168.2.14:18081' + route, timeout=10) as response:
        assert response.status == 200, route
print('PASS: persistent deployment HTTP/SPA/catalog')
PY
ln -sfn "$release" "$deploy_dir/current"
echo "Deployed $GITHUB_SHA to afd at 192.168.2.14:18081"
