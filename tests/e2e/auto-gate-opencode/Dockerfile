# Dockerfile — single-container real-runtime e2e for the auto-gate plugin.
#
# Proves the auto-classifier plugin's enforcement path works against a REAL
# opencode runtime. One container builds opencode from source, then runs a test
# driver that orchestrates a mock LLM server + the real `opencode run` CLI.
#
# Multi-stage:
#   Stage 1 (builder): clone opencode at a pinned commit + bun install.
#   Stage 2 (runtime): copy the opencode tree + test files + plugin sources,
#     run the driver.
#
# Build context: REPO ROOT (so plugin sources + test files are reachable).

# ===========================================================================
# Stage 1: build opencode from source.
# ===========================================================================
FROM oven/bun:1-debian AS builder

# Build tools for native modules (node-pty, tree-sitter, etc.) + git for clone.
RUN apt-get update && \
    apt-get install -y --no-install-recommends \
        build-essential python3 git ca-certificates && \
    rm -rf /var/lib/apt/lists/*

WORKDIR /opt

# Clone the opencode repo at a pinned commit.
# HTTPS public mirror; commit pinned for reproducibility.
ARG OPENCODE_REPO=https://github.com/vhqtvn/opencode.git
ARG OPENCODE_COMMIT=889f979c0ba547842ee6716cc7f38329bff729b7

RUN git clone "${OPENCODE_REPO}" opencode && \
    cd opencode && \
    git checkout "${OPENCODE_COMMIT}"

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

# ── FIX 1: InstanceMiddleware on line-139 InstanceRoutes mount ─────────────
# ROOT CAUSE: In createHono() Branch 2 (no OPENCODE_WORKSPACE_ID — the default
# multi-workspace path), InstanceRoutes is mounted at line 139 WITHOUT
# InstanceMiddleware. Branch 1 (line 123) and workspaceLegacyApp (line 130)
# BOTH wrap their InstanceRoutes mounts with InstanceMiddleware; only the
# line-139 mount is the inconsistent outlier.
#
# Without InstanceMiddleware, the permission reply route handler
# (POST /session/{id}/permissions/{permissionID}) has no instance context:
# the permission pending map is an InstanceState ScopedCache keyed by
# directory, and without instance context the handler resolves the wrong
# directory → empty pending map → `if (!existing) return` → silent no-op.
#
# Under `opencode run`, effectCmd (instance:true) establishes ambient ALS
# instance context for the whole handler, so the reply resolves via ambient
# context WITHOUT needing the middleware. Under `opencode serve`,
# effectCmd uses instance:false (no ambient ALS), so the middleware IS
# required.
#
# FIX: Wrap the line-139 InstanceRoutes mount with InstanceMiddleware.
# The sed targets ONLY line 139 (6-space indent, no trailing comma) to
# avoid touching line 123 (8-space indent, trailing comma).
RUN cd /opt/opencode/packages/opencode/src/server && \
    sed -i 's|^      \.route("/", InstanceRoutes(runtime\.upgradeWebSocket, opts))$|      .route("/", new Hono().use(InstanceMiddleware()).route("/", InstanceRoutes(runtime.upgradeWebSocket, opts)))|' server.ts

# ── FIX 2: Route plugin reply through HTTP when serve listener is active ──
# ROOT CAUSE (serve-specific): The plugin creates its SDK client with an
# in-process fetch override: `fetch: Server.Default().app.fetch(...)`. This
# processes the permission reply in the CALLER's async context (the bus
# stream-consumer fiber), which has a different Effect fiber/scope lineage
# than the serve listener that processed the ASK. The ScopedCache that backs
# the permission pending map resolves to a different State object for the
# reply vs the ask, so `pending.get(requestID)` returns undefined →
# `if (!existing) return` → silent no-op.
#
# FIX: When Server.url is set (serve mode), use globalThis.fetch to send
# the reply over HTTP to the live serve listener. This routes the reply
# through Bun's HTTP server → a fresh async context → InstanceMiddleware →
# the same fiber/scope lineage as the ASK. When Server.url is unset (run
# mode), fall back to the original in-process fetch (which works via
# ambient ALS from effectCmd instance:true).
RUN cd /opt/opencode/packages/opencode/src/plugin && \
    sed -i 's|baseUrl: "http://localhost:4096"|baseUrl: Server.url ? Server.url.origin : "http://localhost:4096"|' index.ts && \
    sed -i 's|fetch: async (\.\.\.args) => Server\.Default()\.app\.fetch(\.\.\.args)|fetch: Server.url ? globalThis.fetch : (async (...args) => Server.Default().app.fetch(...args))|' index.ts

# Copy test files.
WORKDIR /opt/test
COPY tests/e2e/auto-gate-opencode/mock-llm-server.js /opt/test/
COPY tests/e2e/auto-gate-opencode/run-e2e.mjs /opt/test/
COPY tests/e2e/auto-gate-opencode/target.txt /opt/test/
COPY tests/e2e/auto-gate-opencode/classifier-prompt.md /opt/test/

# Copy plugin sources (4 modules: main + live + verdict + scrub).
# Do NOT copy auto-gate-tiered.js — it may export `server` and conflict.
COPY .vh-agent-harness/overlays/auto-classifier-pilot/plugins/auto-tool-gate.js /tmp/plugins-src/
COPY .vh-agent-harness/overlays/auto-classifier-pilot/plugins/auto-gate-live.js /tmp/plugins-src/
COPY .vh-agent-harness/overlays/auto-classifier-pilot/plugins/auto-gate-verdict.js /tmp/plugins-src/
COPY .vh-agent-harness/overlays/auto-classifier-pilot/plugins/auto-gate-scrub.js /tmp/plugins-src/

# Bundle the 4 plugin modules into a SINGLE self-contained file.
# REASON: OpenCode's plugin auto-discovery scans plugins/*.{ts,js} and loads
# EACH file as a separate plugin. The sibling modules (live/verdict/scrub) do
# not export `server` and crash the loader. Bundling inlines all imports into
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
