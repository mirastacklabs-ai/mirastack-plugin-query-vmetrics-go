# MIRASTACK Plugin — Query VMetrics Go (multi-arch: linux/amd64, linux/arm64)
#
# Build:
#   docker buildx build --platform linux/amd64,linux/arm64 \
#     -f agents/oss/mirastack-plugin-query-vmetrics-go/Dockerfile .

FROM --platform=$BUILDPLATFORM golang:1.24-alpine AS builder
ENV GOPRIVATE=github.com/mirastacklabs-ai/* GONOSUMCHECK=github.com/mirastacklabs-ai/* GONOSUMDB=github.com/mirastacklabs-ai/*
ARG TARGETOS
ARG TARGETARCH

WORKDIR /src

# Copy plugin module
COPY agents/oss/mirastack-plugin-query-vmetrics-go/go.mod agents/oss/mirastack-plugin-query-vmetrics-go/go.sum* agents/oss/mirastack-plugin-query-vmetrics-go/
WORKDIR /src/agents/oss/mirastack-plugin-query-vmetrics-go
RUN go mod download

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
