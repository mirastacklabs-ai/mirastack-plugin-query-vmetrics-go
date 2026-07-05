# MIRASTACK Plugin — Query VMetrics Go (multi-arch: linux/amd64, linux/arm64)
#
# Build:
#   docker buildx build --platform linux/amd64,linux/arm64 \
#     -f agents/oss/mirastack-plugin-query-vmetrics-go/Dockerfile .

FROM --platform=$BUILDPLATFORM golang:1.25-alpine AS builder
ARG TARGETOS
ARG TARGETARCH

WORKDIR /src

# Copy plugin module
COPY agents/oss/mirastack-plugin-query-vmetrics-go/go.mod agents/oss/mirastack-plugin-query-vmetrics-go/go.sum* agents/oss/mirastack-plugin-query-vmetrics-go/
WORKDIR /src/agents/oss/mirastack-plugin-query-vmetrics-go
RUN for attempt in 1 2 3 4 5; do \
      go mod download && exit 0; \
      echo "go mod download failed (attempt ${attempt}/5), retrying..." >&2; \
      sleep $((attempt * 3)); \
    done; \
    echo "go mod download failed after 5 attempts" >&2; \
    exit 1

WORKDIR /src
COPY agents/oss/mirastack-plugin-query-vmetrics-go/ agents/oss/mirastack-plugin-query-vmetrics-go/

WORKDIR /src/agents/oss/mirastack-plugin-query-vmetrics-go
RUN CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH \
    go build -ldflags "-s -w" -o /mirastack-plugin-query-vmetrics .

FROM alpine:3.20
RUN apk add --no-cache ca-certificates tzdata
COPY --from=builder /mirastack-plugin-query-vmetrics /usr/local/bin/mirastack-plugin-query-vmetrics
EXPOSE 50051
ENTRYPOINT ["mirastack-plugin-query-vmetrics"]
