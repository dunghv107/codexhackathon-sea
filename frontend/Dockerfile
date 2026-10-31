# syntax=docker/dockerfile:1
ARG RUNTIME_VARIANT=runtime
FROM node:24-alpine@sha256:8a32c8cf649a49915a66e81e7a609223c708d0adabf288ee3f4077d765542f85 AS dependencies
WORKDIR /app
COPY package.json package-lock.json ./
RUN --mount=type=cache,target=/root/.npm npm ci --no-audit --no-fund

FROM dependencies AS source
COPY index.html vite.config.ts tsconfig*.json .oxlintrc.json .gitignore ./
COPY src ./src
COPY public ./public

FROM source AS test
COPY tests ./tests
COPY vitest.config.ts ./
RUN npm run lint && npm test && mkdir -p /reports \
    && cp coverage/lcov.info /reports/coverage-frontend.lcov

FROM source AS builder
RUN npm run build

FROM cgr.dev/chainguard/nginx:latest-dev@sha256:8b15ea7b00c6d65c952e9d14ea75906c86442f5f2ae04349801086bec3e901d7 AS debug-base
FROM cgr.dev/chainguard/nginx:latest@sha256:16d0e55ffe62f55744d9b903d8eb57e533d0f660332a9fbb967d78b4e77c8bdb AS runtime-base

FROM ${RUNTIME_VARIANT}-base AS runtime
COPY nginx.conf /etc/nginx/nginx.conf
COPY --from=builder /app/dist /usr/share/nginx/html
USER 65532:65532
EXPOSE 8080
STOPSIGNAL SIGQUIT
# Checks config only; CI also probes /health over HTTP (docs/CONTAINERS.md).
HEALTHCHECK --interval=30s --timeout=5s --start-period=5s --retries=3 \
    CMD ["/usr/sbin/nginx", "-t"]
ENTRYPOINT ["/usr/sbin/nginx"]
CMD ["-g", "daemon off;"]
