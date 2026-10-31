#!/usr/bin/env bash

# WSL imports from /mnt are slow enough to exceed the stdio startup deadline.
# Keep dependencies on Linux storage, without overriding an explicitly chosen environment.
if [[ -n "${WSL_DISTRO_NAME:-}" && "$ROOT_DIR" == /mnt/* && -z "${UV_PROJECT_ENVIRONMENT:-}" ]]; then
  read -r kbs_environment_hash _ <<< "$(printf '%s' "$ROOT_DIR" | sha256sum)"
  export UV_PROJECT_ENVIRONMENT="${XDG_CACHE_HOME:-$HOME/.cache}/kbs-venvs/$kbs_environment_hash"
fi
