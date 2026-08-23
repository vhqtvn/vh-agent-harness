# docker/selftest/Dockerfile — the native engine's docker self-test
# battery image.
#
# Multi-stage: the builder compiles the three binaries from the tiny
# whitelist context (.dockerignore keeps this to go.mod/go.sum/cmd/
# internal/ + docker/selftest/); the runtime is a slim debian with bash
# + coreutils, which run_shell needs (bash -c, seq, timeout, cmp...).
# The battery (battery.sh) runs INSIDE this container as the entrypoint
# — the daemon/client/mock are never run on the host.
FROM golang:1.25-bookworm AS builder
WORKDIR /src
COPY go.mod go.sum ./
COPY internal/ ./internal/
COPY cmd/ ./cmd/
RUN go build -o /out/ ./cmd/vh-agentd ./cmd/vh-agent-client ./cmd/vh-mockllm

FROM debian:bookworm-slim
RUN apt-get update \
    && apt-get install -y --no-install-recommends bash coreutils jq \
    && rm -rf /var/lib/apt/lists/*
COPY --from=builder /out/ /usr/local/bin/
COPY docker/selftest/scenarios/ /app/scenarios/
COPY docker/selftest/battery.sh /app/battery.sh
RUN chmod +x /app/battery.sh /usr/local/bin/vh-agentd /usr/local/bin/vh-agent-client /usr/local/bin/vh-mockllm \
    && mkdir -p /tmp/battery
WORKDIR /tmp/battery
ENTRYPOINT ["/app/battery.sh"]
