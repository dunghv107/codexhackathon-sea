# syntax=docker/dockerfile:1
ARG RUNTIME_VARIANT=runtime
FROM golang:1.26-alpine@sha256:a2012b4a1d1f9f7e9f44e0d39b805a901e856b1dd80b4e6ee4338db547050308 AS dependencies

WORKDIR /src
ENV CGO_ENABLED=0
COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod go mod download

FROM dependencies AS source
COPY cmd ./cmd
COPY internal ./internal
COPY pkg ./pkg

FROM source AS test
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    mkdir -p /reports && go test -coverprofile=/reports/coverage-go.out ./... && go vet ./...

FROM source AS builder
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    go build -mod=readonly -trimpath -tags=timetzdata -ldflags="-s -w" -o /out/server ./cmd/server

# Only debug contains a shell. Both variants run the same static binary.
FROM cgr.dev/chainguard/busybox:latest@sha256:9cf938251dfb3cc081ef9847e79c203b5de90ecc20855e40bef8a7cc191bb218 AS debug-base
FROM cgr.dev/chainguard/static:latest@sha256:fc42af33e643f20d08baf03cddcc1c02ba57052dfb02edb0d79f3fc47bcea8d9 AS runtime-base

FROM ${RUNTIME_VARIANT}-base AS runtime
WORKDIR /app
COPY --from=builder /out/server ./server
ENV PORT=8000 LOG_FILE_PATH=/tmp/app.log
USER 65532:65532
EXPOSE 8000
HEALTHCHECK --interval=30s --timeout=5s --start-period=15s --retries=3 \
    CMD ["/app/server", "healthcheck"]
ENTRYPOINT ["/app/server"]
