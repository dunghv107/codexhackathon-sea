# syntax=docker/dockerfile:1
ARG RUNTIME_VARIANT=runtime
# Python 3.13 matches pyproject.toml (<3.14); both bases have the same ABI.
FROM gcr.io/distroless/python3-debian13:debug-nonroot@sha256:168097d6d706a066edff2c15383b1b40fb0f2fc625d6b7950e6437122bc2112f AS debug-base
FROM gcr.io/distroless/python3-debian13:nonroot@sha256:64e54c4b1200da244d80181d98e86fc01d73c8101f107d5baf472152d8e119a2 AS runtime-base

FROM debug-base AS dependencies
USER 0
SHELL ["/busybox/sh", "-c"]
COPY --from=ghcr.io/astral-sh/uv:0.8.22@sha256:9874eb7afe5ca16c363fe80b294fe700e460df29a55532bbfea234a0f12eddb1 /uv /usr/local/bin/uv
ENV UV_PROJECT_ENVIRONMENT=/opt/venv \
    UV_PYTHON_DOWNLOADS=never \
    UV_LINK_MODE=copy \
    UV_NO_BUILD=1
WORKDIR /app
COPY pyproject.toml uv.lock ./
RUN --mount=type=cache,target=/root/.cache/uv \
    uv sync --locked --no-dev --no-install-project --python /usr/bin/python3

FROM dependencies AS test
RUN --mount=type=cache,target=/root/.cache/uv \
    uv sync --locked --no-install-project --python /usr/bin/python3
COPY app ./app
COPY tests ./tests
COPY examples ./examples
ENV PATH="/opt/venv/bin:${PATH}" ENV_FILE=/dev/null PYTHONPATH=/app
RUN ruff check . && ruff format --check . && ty check && pytest

FROM ${RUNTIME_VARIANT}-base AS runtime
WORKDIR /app
COPY --from=dependencies /opt/venv /opt/venv
COPY app ./app
ENV PATH="/opt/venv/bin:${PATH}" \
    PYTHONPATH=/app \
    PYTHONDONTWRITEBYTECODE=1 \
    PYTHONUNBUFFERED=1 \
    ENV_FILE=/dev/null \
    LOG_DIR=/tmp/kbs-logs \
    SERVER_RELOAD=false
USER 65532:65532
EXPOSE 1114
HEALTHCHECK --interval=30s --timeout=5s --start-period=15s --retries=3 \
    CMD ["/opt/venv/bin/python", "-c", "import urllib.request; urllib.request.urlopen('http://127.0.0.1:1114/ready', timeout=3)"]
# Clear the inherited entrypoint so existing Compose command overrides work.
ENTRYPOINT []
CMD ["/opt/venv/bin/python", "-m", "uvicorn", "app.main:app", "--host", "0.0.0.0", "--port", "1114", "--no-proxy-headers"]
