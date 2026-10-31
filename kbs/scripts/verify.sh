#!/usr/bin/env bash

set -euo pipefail

ROOT_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT_DIR"
source "$ROOT_DIR/scripts/uv-environment.sh"

uv run ruff format --check
uv run ruff check
uv run ty check
scripts/test-suite.sh default
