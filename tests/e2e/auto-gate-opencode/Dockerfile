# Dockerfile — single-container real-runtime e2e for the auto-gate plugin.
#
# Proves the auto-classifier plugin's enforcement path works against a REAL
# opencode runtime. One container acquires the LATEST upstream opencode, then
# runs a test driver that orchestrates a mock LLM server + the real
# `opencode run` / `opencode serve` runtime.
#
# Acquisition: the image clones the LATEST HEAD of the canonical upstream
# (https://github.com/sst/opencode.git) as a SHALLOW clone at build time — no
# fork, no pinned commit, no host clone dependency. This is intentionally
# non-reproducible: latest HEAD means the suite catches upstream drift. The
# serve bug the e2e USED to patch (InstanceMiddleware outlier mount + in-process
# plugin fetch) was resolved upstream by the Effect HttpApi rewrite + per-request
# `x-opencode-directory` threading, so NO source patches are applied anymore.
#
# Multi-stage:
#   Stage 1 (builder): shallow-clone latest upstream opencode + bun install.
#   Stage 2 (runtime): copy the opencode tree + test files + plugin sources,
#     run the driver.
#
# Build context: REPO ROOT (so plugin sources + test files are reachable).

# ===========================================================================
# Stage 1: acquire + install latest upstream opencode from source.
# ===========================================================================
FROM oven/bun:1-debian AS builder

# Build tools for native modules (node-pty, tree-sitter, etc.) + git for clone.
RUN apt-get update && \
    apt-get install -y --no-install-recommends \
        build-essential python3 git ca-certificates && \
    rm -rf /var/lib/apt/lists/*

WORKDIR /opt

# Acquire the LATEST upstream opencode (canonical repo, shallow HEAD).
# No fork, no pinned commit — the image always builds against current upstream.
ARG OPENCODE_REPO=https://github.com/sst/opencode.git
RUN git clone --depth 1 "${OPENCODE_REPO}" opencode

# Install all workspace dependencies (including patchedDependencies patches).
RUN cd /opt/opencode && bun install

# Verify the source tree has the expected entrypoint.
RUN test -f /opt/opencode/packages/opencode/src/index.ts

# ===========================================================================
# Stage 2: runtime image.
# ===========================================================================
FROM oven/bun:1-debian

# Runtime deps: ripgrep (opencode's grep tool needs it) + ca-certificates (HTTPS).
RUN apt-get update && \
    apt-get install -y --no-install-recommends ripgrep ca-certificates && \
    rm -rf /var/lib/apt/lists/*

# Copy the opencode source tree (run-from-source, no compile step).
COPY --from=builder /opt/opencode /opt/opencode

# Copy test files.
WORKDIR /opt/test
COPY tests/e2e/auto-gate-opencode/mock-llm-server.js /opt/test/
COPY tests/e2e/auto-gate-opencode/run-e2e.mjs /opt/test/
COPY tests/e2e/auto-gate-opencode/target.txt /opt/test/
COPY tests/e2e/auto-gate-opencode/classifier-prompt.md /opt/test/

# Copy plugin sources (5 modules: main + live + verdict + scrub + tiered).
# auto-gate-tiered.js is required at build time — auto-tool-gate.js imports
# normalizeLeafOutcome + aggregateLeafOutcomes from it. The bun build step
# inlines all imports into a single bundled file, so only auto-tool-gate.js
# is discovered as a plugin (the sibling modules do not export `server`).
COPY templates/overlays/auto-classifier-pilot/plugins/auto-tool-gate.js /tmp/plugins-src/
COPY templates/overlays/auto-classifier-pilot/plugins/auto-gate-live.js /tmp/plugins-src/
COPY templates/overlays/auto-classifier-pilot/plugins/auto-gate-verdict.js /tmp/plugins-src/
COPY templates/overlays/auto-classifier-pilot/plugins/auto-gate-scrub.js /tmp/plugins-src/
COPY templates/overlays/auto-classifier-pilot/plugins/auto-gate-tiered.js /tmp/plugins-src/

# Bundle the 5 plugin modules into a SINGLE self-contained file.
# REASON: OpenCode's plugin auto-discovery scans plugins/*.{ts,js} and loads
# EACH file as a separate plugin. The sibling modules (live/verdict/scrub/tiered)
# do not export `server` and crash the loader. Bundling inlines all imports into
# one file so only one plugin is discovered. This does NOT modify the
# committed plugin source — it is a build-time transform.
RUN bun build /tmp/plugins-src/auto-tool-gate.js \
        --outfile /tmp/plugins-src/bundled.js \
        --target=bun \
        --external node:* \
        --format=esm && \
    mkdir -p /workspace/.opencode/plugins && \
    cp /tmp/plugins-src/bundled.js /workspace/.opencode/plugins/auto-tool-gate.js && \
    rm -rf /tmp/plugins-src

# Place the target fixture inside /workspace (read.ts asserts the path is
# inside the project directory).
RUN cp /opt/test/target.txt /workspace/target.txt

# Symlink node_modules into /workspace (needed by bun's module resolution
# even with --cwd; some imports resolve from process.cwd()).
RUN ln -sf /opt/opencode/node_modules /workspace/node_modules

ENTRYPOINT ["bun", "/opt/test/run-e2e.mjs"]
