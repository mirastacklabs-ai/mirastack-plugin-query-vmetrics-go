# MIRASTACK Plugin — Query VMetrics Go (multi-arch: linux/amd64, linux/arm64)
# Build context must be the monorepo root (mirastack/)
# so the local SDK replace directive resolves.
#
# Build:
#   docker buildx build --platform linux/amd64,linux/arm64 \
#     -f agents/oss/mirastack-plugin-query-vmetrics-go/Dockerfile .

FROM --platform=$BUILDPLATFORM golang:1.24-alpine AS builder
ARG TARGETOS
ARG TARGETARCH

WORKDIR /src

# Copy SDK first (referenced via replace directive)
COPY sdk/oss/mirastack-sdk-go/ sdk/oss/mirastack-sdk-go/

# Copy plugin module
COPY agents/oss/mirastack-plugin-query-vmetrics-go/go.mod agents/oss/mirastack-plugin-query-vmetrics-go/go.sum* agents/oss/mirastack-plugin-query-vmetrics-go/
WORKDIR /src/agents/oss/mirastack-plugin-query-vmetrics-go
RUN go mod edit -replace github.com/mirastacklabs-ai/mirastack-sdk-go=../../../sdk/oss/mirastack-sdk-go \
    && go mod tidy \
    && go mod download

WORKDIR /src
COPY agents/oss/mirastack-plugin-query-vmetrics-go/ agents/oss/mirastack-plugin-query-vmetrics-go/

WORKDIR /src/agents/oss/mirastack-plugin-query-vmetrics-go
RUN go mod edit -replace github.com/mirastacklabs-ai/mirastack-sdk-go=../../../sdk/oss/mirastack-sdk-go \
    && CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH \
    go build -ldflags "-s -w" -o /mirastack-plugin-query-vmetrics .

FROM alpine:3.20
RUN apk add --no-cache ca-certificates tzdata
COPY --from=builder /mirastack-plugin-query-vmetrics /usr/local/bin/mirastack-plugin-query-vmetrics
EXPOSE 50051
ENTRYPOINT ["mirastack-plugin-query-vmetrics"]
