#!/usr/bin/env bash
# battery.sh — the native-engine docker self-test battery. Runs INSIDE
# the selftest container as the entrypoint (never on the host).
#
# For each scenario: start vh-mockllm with the scenario script in the
# background → wait for /healthz → run the REAL vh-agent-client driving
# the REAL vh-agentd against the mock → assert on stdout/stderr, the
# produced session log (--verify-log), the mock's /count and /journal
# (the model request plane) → kill the mock → next.
#
# SCENARIO NOTES (honest scope):
#   - approval_flow (P3.5): the daemon's --ask-tools flag is the real
#     daemon-side ask source (previously none existed — the old
#     re-scope note here said approval grant/deny could only be
#     covered at the library seam). The scenario drives the FULL
#     waterfall over the real binaries: ask-routed run_shell calls,
#     approval/request on the wire, the client's --policy engine
#     answering (a broad allow for the benign call, the git-mutation
#     HARD-DENY for `git push` under the SAME broad rule), the tool
#     executing on grant, and a typed denial — executor never ran —
#     on hard-deny. The fail-closed policy parse (policy_bad_file) and
#     the loaded-but-unconsulted posture (policy_loaded_clean) stay.
#     The timeout/disconnect deny directions remain covered at the
#     bridge seam (internal/protocol/disconnect_test.go).
#   - empty_response_retry exercises the mock's "empty" step class
#     through the real adapter retry ladder (a real engine behavior).
#   - spill_paging is a TWO-RUN scenario: run A creates the spill (its
#     locator is content-addressed and unpredictable — durationMs is in
#     the spilled content), then battery.sh extracts the locator from
#     run A's session log, GENERATES run B's script with the embedded
#     locator, and run B (a second session in the same session dir,
#     whose daemon-wide spill walk reaches run A's store) pages the
#     spill through the real spill_read tool.
#   - resume_restart (P4) is a TWO-DAEMON-LIFETIME scenario: run A
#     creates the session (one tool turn), its daemon exits; run B is
#     a fresh daemon on the SAME session dir whose client resumes by
#     pointer (session/resume over the wire) and continues the SAME
#     durable log — surface continuity, derived title, event-count
#     growth, and the deterministic replay prover across the restart.
#   - compaction (P5) drives the post-turn compaction trigger over the
#     real binaries with a tiny --context-tokens budget. The mock
#     script accounts for the SUMMARIZE call (a real LLM call through
#     the same adapter, consumed as step 2 BETWEEN the tool turn and
#     the continuation's final answer); the KV-prefix request shape is
#     asserted from the mock journal, and the compacted log must pass
#     the deterministic replay prover with the post-compaction
#     message count.
#   - job_tailing (P6) drives the background run_shell round-trip: the
#     model dispatches a multi-second ticker with background:true, the
#     tool result is the non-blocking receipt, the CLIENT's drain loop
#     tails jobs/output mid-flight (partial chunks while running, the
#     complete reassembled stream at settlement), settlement + report
#     events land on the durable log, and the log replays
#     deterministically.
#   - skills (P7) drives the three-tier skills delivery over the real
#     binaries with a fixture catalog (--skills-dir): tier-1 prompt
#     advertisement (sanitized, no raw angle tokens) proven from the
#     mock journal; tier-2 skill_load body + allowed-tools ceiling
#     footer and tier-3 reference load in the --json stream; unknown
#     name + traversal ref rejected fail-closed; the log-only
#     skill/loaded provenance events; malformed-skill exclusion with
#     the honest startup count; and the REPLAY-AFTER-MUTATION crux —
#     the fixture SKILL.md is mutated on disk post-run and --verify-log
#     must stay byte-identical (replay derives from the log, never
#     disk). The fixture is restored after the mutation so image-baked
#     content is left as shipped.
#   - mcp (P8/P8.2) drives the MCP-host surface over the real binaries
#     in THREE runs against one mockmcp server: (a) the DEFAULT
#     ask-by-default posture — no --policy, no --ask-tools, EOF-stdin
#     client: approval/request on the wire, fail-closed denials, the
#     tools provably NEVER executed (the mock's /calls counter stays
#     0); (b) POLICY promotion — exact-name [[allow]] rules plus the
#     per-server underscore glob grant the asks and the same tools
#     round-trip (the degraded-sentinel / garbage-call twins, journal,
#     log, replay-determinism, and token-redaction assertions live
#     here); (c) the --mcp-auto-allow opt-in round-trips without
#     asking.
#   - Cross-run byte-comparison of DIFFERENT sessions is a NON-GOAL —
#     session ids are random by design (sess-<random hex>). Determinism
#     is proven per-log: two --verify-log runs on the SAME log must
#     print byte-identical lines (verify_determinism + every
#     verify_log_ok assertion).
#   - No curl in the runtime image: HTTP GETs against the mock go
#     through bash's /dev/tcp.
set -u

MOCK_ADDR=127.0.0.1
MOCK_PORT=8099
MOCK_URL="http://${MOCK_ADDR}:${MOCK_PORT}"
# The API key VALUE: passed to the daemon via the environment only
# (the daemon reads MOCK_KEY; the mock only ever sees it as a header).
# The battery asserts this value NEVER appears in the mock journal.
MOCK_KEY="dummy-key-value-9f1c2ab7"
export MOCK_KEY
DAEMON=/usr/local/bin/vh-agentd
CLIENT=/usr/local/bin/vh-agent-client
MOCK=/usr/local/bin/vh-mockllm
SCEN=/app/scenarios
WORK=/tmp/battery

PASS=0
FAIL=0
FAILED=""
SCEN_PASS=0
SCEN_TOTAL=0

note() { printf 'battery: %s\n' "$*"; }
pass() { PASS=$((PASS + 1)); note "PASS: $1"; }
fail() { FAIL=$((FAIL + 1)); FAILED="$FAILED $1"; note "FAIL: $1 — $2"; }
check() { # check <scenario> <description> <command...>
    local sc="$1" desc="$2"; shift 2
    if "$@"; then
        PASS=$((PASS + 1))
        note "  ok: $desc"
    else
        fail "$sc" "$desc"
        return 1
    fi
    return 0
}

# run_scenario <name> <func> — assertion-level results roll up into a
# per-scenario verdict: a scenario passes iff it added zero failures.
run_scenario() {
    local name="$1" func="$2" before=$FAIL
    note "--- scenario: $name"
    "$func"
    SCEN_TOTAL=$((SCEN_TOTAL + 1))
    if [ "$FAIL" -eq "$before" ]; then
        SCEN_PASS=$((SCEN_PASS + 1))
        note "SCENARIO PASS: $name"
    else
        note "SCENARIO FAIL: $name"
    fi
}

mkdir -p "$WORK"

# ---- mock control ----------------------------------------------------------

MOCK_PID=""
start_mock() { # start_mock <script-path|none> [journal-path]
    MOCK_PID=""
    [ "$1" = "none" ] && return 0
    local jargs=()
    if [ "${2:-}" != "" ]; then
        jargs=(--journal "$2")
    fi
    "$MOCK" --addr "${MOCK_ADDR}:${MOCK_PORT}" --script "$1" "${jargs[@]+"${jargs[@]}"}" \
        >"$WORK/mock.out" 2>"$WORK/mock.err" &
    MOCK_PID=$!
    local i
    for i in $(seq 1 100); do
        if http_get /healthz 2>/dev/null | grep -q '"ok"'; then
            return 0
        fi
        sleep 0.1
    done
    note "mock failed to become healthy; stderr:"
    sed -n '1,10p' "$WORK/mock.err"
    exit 1
}
stop_mock() {
    if [ -n "$MOCK_PID" ]; then
        kill "$MOCK_PID" 2>/dev/null
        wait "$MOCK_PID" 2>/dev/null
        MOCK_PID=""
    fi
}

# http_get <path> — GET against the mock over bash /dev/tcp; body on
# stdout (headers stripped). HTTP/1.0 + Connection: close so the read
# terminates at EOF.
http_get() {
    http_get_port "$MOCK_PORT" "$1"
}

# http_get_port <port> <path> — http_get against an arbitrary local
# port (the MCP scenario's vh-mockmcp server runs beside vh-mockllm).
http_get_port() {
    exec 3<>"/dev/tcp/${MOCK_ADDR}/$1" || return 1
    printf 'GET %s HTTP/1.0\r\nHost: mock\r\nConnection: close\r\n\r\n' "$2" >&3
    cat <&3 | awk 'begin{skip=0} /^\r?$/{skip=1; next} skip{print}'
    exec 3<&- 3>&-
}

chat_count() { http_get /count | grep -o '"/v1/chat/completions":[0-9]*' | cut -d: -f2; }
msg_count() { http_get /count | grep -o '"/v1/messages":[0-9]*' | cut -d: -f2; }
journal_raw() { http_get /journal; }

# ---- client / log helpers --------------------------------------------------

fresh_session() { # fresh_session <name> — per-scenario session dir
    SESS="$WORK/sessions/$1"
    rm -rf "$SESS"
    mkdir -p "$SESS"
}

run_client() { # run_client <args...> → $CODE, $WORK/client.out/.err
    timeout 90 "$CLIENT" "$@" >"$WORK/client.out" 2>"$WORK/client.err" </dev/null
    CODE=$?
}

newest_log() { # newest sess-*.jsonl directly under $SESS → $LOG
    LOG=$(ls -t "$SESS"/sess-*.jsonl 2>/dev/null | head -1)
}

verify_log_ok() { # verify_log_ok <log> — two runs, both exit 0, byte-identical, non-empty
    local log="$1"
    "$DAEMON" --verify-log "$log" >"$WORK/v1.out" 2>"$WORK/v1.err"; local c1=$?
    "$DAEMON" --verify-log "$log" >"$WORK/v2.out" 2>"$WORK/v2.err"; local c2=$?
    [ "$c1" -eq 0 ] && [ "$c2" -eq 0 ] && [ -s "$WORK/v1.out" ] \
        && cmp -s "$WORK/v1.out" "$WORK/v2.out"
}

retry_events() { grep -c '"type":"llm/retry"' "$1"; }

# ---- scenarios -------------------------------------------------------------

sc_greeting() {
    local sc=greeting
    fresh_session "$sc"
    rm -f "$WORK/greeting-journal.jsonl"
    start_mock "$SCEN/greeting.json" "$WORK/greeting-journal.jsonl"
    run_client --session-dir "$SESS" --prompt "say the greeting" \
        --exec "$DAEMON" --adapter openai --model mock-model \
        --base-url "$MOCK_URL/v1" --api-key-env MOCK_KEY
    check "$sc" "client exits 0" [ "$CODE" -eq 0 ]
    check "$sc" "final assistant text on stdout" \
        grep -Fxq "greeting final: hello engine" "$WORK/client.out"
    newest_log
    check "$sc" "session log found + --verify-log deterministic ok" verify_log_ok "$LOG"
    local journal
    journal=$(journal_raw)
    check "$sc" "journal records auth-header PRESENCE" grep -q '"auth":true' <<<"$journal"
    if grep -q "$MOCK_KEY" <<<"$journal"; then
        fail "$sc" "API-key VALUE leaked into the mock journal"
    else
        PASS=$((PASS + 1))
        note "  ok: API-key value never appears in the journal (redaction)"
    fi
    # The on-disk --journal mirror: same redaction discipline, one JSONL
    # line per received request.
    sleep 0.2 # the disk mirror writes best-effort per request; let it flush
    check "$sc" "disk --journal file recorded the request" [ -s "$WORK/greeting-journal.jsonl" ]
    if grep -q "$MOCK_KEY" "$WORK/greeting-journal.jsonl" 2>/dev/null; then
        fail "$sc" "API-key VALUE leaked into the DISK journal"
    else
        PASS=$((PASS + 1))
        note "  ok: API-key value never appears in the disk journal (redaction)"
    fi
    stop_mock
}

sc_tool_roundtrip() {
    local sc=tool_roundtrip
    fresh_session "$sc"
    start_mock "$SCEN/tool_roundtrip.json"
    run_client --session-dir "$SESS" --json --prompt "use echo" \
        --exec "$DAEMON" --adapter openai --model mock-model \
        --base-url "$MOCK_URL/v1" --api-key-env MOCK_KEY
    check "$sc" "client exits 0" [ "$CODE" -eq 0 ]
    check "$sc" "tool result visible in --json stream" grep -Fq "round-trip-payload" "$WORK/client.out"
    check "$sc" "final text in prompt-result" grep -Fq "tool round trip complete" "$WORK/client.out"
    local journal
    journal=$(journal_raw)
    check "$sc" "journal shows the echo tool ADVERTISED in the request tools array" \
        grep -q '"name":"echo"' <<<"$journal"
    local n
    n=$(chat_count)
    check "$sc" "model called exactly twice (tool call turn + final turn) — got ${n:-none}" \
        [ "${n:-0}" -eq 2 ]
    newest_log
    check "$sc" "--verify-log ok" verify_log_ok "$LOG"
    stop_mock
}

sc_policy_bad_file() {
    local sc=policy_bad_file
    fresh_session "$sc"
    start_mock none
    run_client --session-dir "$SESS" --prompt "x" \
        --policy "$SCEN/policy_bad.policy" \
        --exec "$DAEMON" --adapter openai --model mock-model \
        --base-url "$MOCK_URL/v1" --api-key-env MOCK_KEY
    check "$sc" "client exits 2 (fail-closed policy parse)" [ "$CODE" -eq 2 ]
    check "$sc" "error names the offending policy line" grep -q "policy_bad.policy:3" "$WORK/client.err"
    check "$sc" "NO daemon was spawned" \
        bash -c '! grep -q "vh-agent-client: daemon " '"$WORK"'/client.err'
    check "$sc" "no session log was created" \
        bash -c '[ -z "$(ls '"$SESS"'/sess-*.jsonl 2>/dev/null)" ]'
}

sc_policy_loaded_clean() {
    local sc=policy_loaded_clean
    fresh_session "$sc"
    start_mock "$SCEN/policy_loaded_clean.json"
    run_client --session-dir "$SESS" --prompt "echo through a loaded policy" \
        --policy "$SCEN/policy_loaded_clean.policy" \
        --exec "$DAEMON" --adapter openai --model mock-model \
        --base-url "$MOCK_URL/v1" --api-key-env MOCK_KEY
    check "$sc" "client exits 0" [ "$CODE" -eq 0 ]
    check "$sc" "policy load note present" grep -q "policy loaded (1 rules)" "$WORK/client.err"
    check "$sc" "run completes with final text" \
        grep -Fxq "policy clean run done" "$WORK/client.out"
    if grep -q "policy: " "$WORK/client.err"; then
        fail "$sc" "a policy DECISION line appeared — nothing asks with the shipped tools, so the engine must never be consulted"
    else
        PASS=$((PASS + 1))
        note "  ok: no policy decision line (the engine is loaded but unconsulted — honest)"
    fi
    newest_log
    check "$sc" "--verify-log ok" verify_log_ok "$LOG"
    stop_mock
}

sc_empty_response_retry() {
    local sc=empty_response_retry
    fresh_session "$sc"
    start_mock "$SCEN/empty_response.json"
    run_client --session-dir "$SESS" --prompt "empty then answer" \
        --exec "$DAEMON" --adapter openai --model mock-model \
        --base-url "$MOCK_URL/v1" --api-key-env MOCK_KEY
    check "$sc" "client exits 0" [ "$CODE" -eq 0 ]
    check "$sc" "final text after recovery" \
        grep -Fxq "recovered after empty" "$WORK/client.out"
    local n
    n=$(chat_count)
    check "$sc" "model called exactly twice (empty class retried) — got ${n:-none}" [ "${n:-0}" -eq 2 ]
    newest_log
    local r
    r=$(retry_events "$LOG")
    check "$sc" "session log carries exactly 1 llm/retry event — got ${r:-0}" [ "${r:-0}" -eq 1 ]
    check "$sc" "--verify-log ok" verify_log_ok "$LOG"
    stop_mock
}

sc_sandbox_denial() {
    local sc=sandbox_denial
    rm -f /tmp/sbx-denied-marker
    fresh_session "$sc"
    start_mock "$SCEN/sandbox_denial.json"
    run_client --session-dir "$SESS" --json --prompt "try writing a file" \
        --exec "$DAEMON" --adapter openai --model mock-model \
        --base-url "$MOCK_URL/v1" --api-key-env MOCK_KEY \
        --sandbox read-only
    check "$sc" "client exits 0 (the turn completes honestly)" [ "$CODE" -eq 0 ]
    check "$sc" "kernel EACCES diagnostic in the tool result" grep -Fq "Permission denied" "$WORK/client.out"
    # The tool result rides the NDJSON stream as an escaped JSON string,
    # so the inner quotes appear as \" — match the escaped form.
    check "$sc" "outcome carries sandbox:\"read-only\"" grep -Fq '\"sandbox\":\"read-only\"' "$WORK/client.out"
    check "$sc" "the denied write left NO file behind" [ ! -f /tmp/sbx-denied-marker ]
    check "$sc" "final text still delivered" grep -Fq "sandbox denial observed" "$WORK/client.out"
    newest_log
    check "$sc" "--verify-log ok" verify_log_ok "$LOG"
    stop_mock
}

sc_retry_ladder() {
    local sc=retry_ladder
    fresh_session "$sc"
    start_mock "$SCEN/retry_ladder.json"
    run_client --session-dir "$SESS" --prompt "survive the ladder" \
        --exec "$DAEMON" --adapter openai --model mock-model \
        --base-url "$MOCK_URL/v1" --api-key-env MOCK_KEY
    check "$sc" "client exits 0" [ "$CODE" -eq 0 ]
    check "$sc" "final text after both faults" \
        grep -Fxq "retry ladder complete" "$WORK/client.out"
    local n
    n=$(chat_count)
    check "$sc" "mock saw exactly 3 chat/completions (500, 429, success) — got ${n:-none}" [ "${n:-0}" -eq 3 ]
    newest_log
    local r
    r=$(retry_events "$LOG")
    check "$sc" "session log carries 2 llm/retry events — got ${r:-0}" [ "${r:-0}" -eq 2 ]
    check "$sc" "--verify-log ok" verify_log_ok "$LOG"
    stop_mock
}

sc_subagent_recursion() {
    local sc=subagent_recursion
    fresh_session "$sc"
    start_mock "$SCEN/subagent_recursion.json"
    run_client --session-dir "$SESS" --json --prompt "delegate the task" \
        --exec "$DAEMON" --adapter openai --model mock-model \
        --base-url "$MOCK_URL/v1" --api-key-env MOCK_KEY
    check "$sc" "client exits 0" [ "$CODE" -eq 0 ]
    check "$sc" "child report surfaced in the parent stream" \
        grep -Fq "child report: computed answer" "$WORK/client.out"
    check "$sc" "root final text" grep -Fq "root done after subagent" "$WORK/client.out"
    local kids
    kids=$(find "$SESS/subagents" -name '*.jsonl' 2>/dev/null | head -1)
    check "$sc" "child session log exists under <session-dir>/subagents/" [ -n "$kids" ]
    newest_log
    check "$sc" "parent --verify-log ok" verify_log_ok "$LOG"
    if [ -n "$kids" ]; then
        check "$sc" "child log --verify-log ok" verify_log_ok "$kids"
    fi
    stop_mock
}

sc_spill_paging() {
    local sc=spill_paging
    fresh_session "$sc"
    start_mock "$SCEN/spill_create.json"
    # Run A: the oversize run_shell result spills; the model gets the
    # bounded preview + notice.
    run_client --session-dir "$SESS" --json --prompt "run the big command" \
        --exec "$DAEMON" --adapter openai --model mock-model \
        --base-url "$MOCK_URL/v1" --api-key-env MOCK_KEY
    check "$sc" "run A exits 0" [ "$CODE" -eq 0 ]
    check "$sc" "spill notice present" grep -Fq "[spilled " "$WORK/client.out"
    check "$sc" "spill notice names spill_read" grep -Fq "read via spill_read" "$WORK/client.out"
    newest_log
    local logA="$LOG"
    check "$sc" "run A --verify-log ok" verify_log_ok "$logA"
    # Extract the spill locator (a nested JSON object in the tool/result
    # payload) and GENERATE run B's script with it embedded.
    local loc size
    loc=$(jq -c 'select(.type == "tool/result") | .payload.spillLocator // empty' "$logA" | head -1)
    if [ -z "$loc" ]; then
        fail "$sc" "no spillLocator found in run A's log"
        stop_mock
        return
    fi
    size=$(jq -r '.size' <<<"$loc")
    note "  extracted spill locator (size=$size)"
    stop_mock
    local gen="$WORK/spill_page.json"
    jq -n --argjson loc "$loc" --argjson size "$size" '[
        {"tool_calls": [
            {"id": "pg-1", "name": "spill_read", "args": {"locator": $loc, "offset": 0, "length": 200}},
            {"id": "pg-term", "name": "spill_read", "args": {"locator": $loc, "offset": $size}}
        ]},
        {"text": "paging complete"}
    ]' >"$gen"
    # Run B: a SECOND session in the SAME session dir — the spill_read
    # tool's daemon-wide walk reaches run A's store by locator.
    start_mock "$gen"
    run_client --session-dir "$SESS" --json --prompt "page the spill" \
        --exec "$DAEMON" --adapter openai --model mock-model \
        --base-url "$MOCK_URL/v1" --api-key-env MOCK_KEY
    check "$sc" "run B exits 0" [ "$CODE" -eq 0 ]
    check "$sc" "windowed content with paging notice" grep -Fq "[window offset=0 length=" "$WORK/client.out"
    check "$sc" "terminal [window complete:...] notice" grep -Fq "[window complete:" "$WORK/client.out"
    check "$sc" "run B final text" grep -Fq "paging complete" "$WORK/client.out"
    newest_log
    check "$sc" "run B --verify-log ok" verify_log_ok "$LOG"
    stop_mock
}

sc_anthropic_dialect() {
    local sc=anthropic_dialect
    fresh_session "$sc"
    start_mock "$SCEN/anthropic.json"
    # NOTE: the anthropic adapter appends /v1/messages itself, so the
    # base URL is the mock root (no /v1 suffix, unlike openai).
    run_client --session-dir "$SESS" --prompt "hello in anthropic shape" \
        --exec "$DAEMON" --adapter anthropic --model mock-model \
        --base-url "$MOCK_URL" --api-key-env MOCK_KEY
    check "$sc" "client exits 0" [ "$CODE" -eq 0 ]
    check "$sc" "final assistant text on stdout" \
        grep -Fxq "anthropic dialect works" "$WORK/client.out"
    local n
    n=$(msg_count)
    check "$sc" "mock served exactly 1 /v1/messages request — got ${n:-none}" [ "${n:-0}" -eq 1 ]
    local journal
    journal=$(journal_raw)
    check "$sc" "journal shows the anthropic request body" grep -q '"/v1/messages"' <<<"$journal"
    newest_log
    check "$sc" "--verify-log ok" verify_log_ok "$LOG"
    stop_mock
}

sc_verify_determinism() {
    local sc=verify_determinism
    fresh_session "$sc"
    start_mock "$SCEN/verify_determinism.json"
    run_client --session-dir "$SESS" --prompt "make me a log" \
        --exec "$DAEMON" --adapter openai --model mock-model \
        --base-url "$MOCK_URL/v1" --api-key-env MOCK_KEY
    check "$sc" "client exits 0" [ "$CODE" -eq 0 ]
    newest_log
    "$DAEMON" --verify-log "$LOG" >"$WORK/d1.out" 2>"$WORK/d1.err"; local c1=$?
    "$DAEMON" --verify-log "$LOG" >"$WORK/d2.out" 2>"$WORK/d2.err"; local c2=$?
    check "$sc" "both verify runs exit 0" bash -c "[ '$c1' -eq 0 ] && [ '$c2' -eq 0 ]"
    check "$sc" "two runs on the SAME log are byte-identical" cmp -s "$WORK/d1.out" "$WORK/d2.out"
    check "$sc" "output carries surface_sha256" grep -q '"surface_sha256"' "$WORK/d1.out"
    stop_mock
}

sc_approval_flow() {
    local sc=approval_flow
    fresh_session "$sc"
    start_mock "$SCEN/approval_flow.json"
    # The daemon ask-routes run_shell (--ask-tools); the client's
    # --policy engine answers the wire asks. A BROAD run_shell allow
    # rule proves the floor: the benign echo auto-allows and EXECUTES;
    # `git push` under the SAME rule HARD-DENIES (git-mutation class)
    # — the executor never runs.
    run_client --session-dir "$SESS" --json \
        --policy "$SCEN/approval_flow.policy" \
        --prompt "run the routed tools" \
        --exec "$DAEMON" --ask-tools run_shell \
        --adapter openai --model mock-model \
        --base-url "$MOCK_URL/v1" --api-key-env MOCK_KEY
    check "$sc" "client exits 0" [ "$CODE" -eq 0 ]
    check "$sc" "daemon startup log names the routed tools" \
        grep -q "ask-tools: run_shell" "$WORK/client.err"
    check "$sc" "policy loaded (1 rules)" grep -q "policy loaded (1 rules)" "$WORK/client.err"
    check "$sc" "approval/request fired on the wire" grep -q "ask-routed by --ask-tools" "$WORK/client.out"
    check "$sc" "policy auto-ALLOWED the benign call" \
        grep -q "policy: allow run_shell(command=echo approval-flow-ran)" "$WORK/client.err"
    check "$sc" "policy HARD-DENIED git push under the same broad rule" \
        grep -q "policy: HARD-DENY run_shell(command=git push origin main)" "$WORK/client.err"
    check "$sc" "final text delivered" grep -Fq "approval flow exercised" "$WORK/client.out"
    newest_log
    # jq on the durable session log: grant → executed with the real
    # stdout; hard-deny → typed denial, executor never ran. The
    # capture-then-test idiom (spill_paging's shape) is deliberate:
    # jq 1.6 (bookworm) exits 4 from `jq -e` whenever the LAST input
    # line matches nothing, even when earlier lines matched — the
    # var-emptiness test is version-robust.
    local gexec ddenied dnever
    gexec=$(jq -c 'select(.type == "tool/result" and .payload.callId == "call-af-1"
                     and (.payload.denied != true)
                     and (.payload.content | contains("approval-flow-ran")))' "$LOG" | head -1)
    check "$sc" "granted call EXECUTED (result present, not denied)" [ -n "$gexec" ]
    ddenied=$(jq -c 'select(.type == "tool/result" and .payload.callId == "call-af-2"
                     and .payload.denied == true
                     and .payload.deniedBy == "ask-tools")' "$LOG" | head -1)
    check "$sc" "denied call settled as a typed denial by ask-tools" [ -n "$ddenied" ]
    dnever=$(jq -c 'select(.type == "tool/result" and .payload.callId == "call-af-2")
                     | select(.payload.content | contains("exitCode") or contains("fatal:") | not)' "$LOG" | head -1)
    check "$sc" "the denied call's executor NEVER ran (no shell outcome)" [ -n "$dnever" ]
    local n
    n=$(chat_count)
    check "$sc" "model called exactly twice (tool turn + final turn) — got ${n:-none}" \
        [ "${n:-0}" -eq 2 ]
    check "$sc" "--verify-log ok" verify_log_ok "$LOG"
    stop_mock
}

# sc_resume_restart (P4): the restart-and-resume crux over REAL
# binaries. Run A creates the session and runs one tool turn; the
# daemon exits (client EOF ladder). Run B is a SECOND daemon lifetime
# on the SAME session dir: the client resumes by pointer
# (session/resume on the wire), asserts surface continuity (same log
# file, resume line carrying the derived title + event count), sends a
# continuation prompt, and the grown log passes the deterministic
# replay prover across the restart boundary.
sc_resume_restart() {
    local sc=resume_restart
    fresh_session "$sc"
    start_mock "$SCEN/resume_restart.json"
    run_client --session-dir "$SESS" --prompt "run A prompt for the resume test" \
        --exec "$DAEMON" --adapter openai --model mock-model \
        --base-url "$MOCK_URL/v1" --api-key-env MOCK_KEY
    check "$sc" "run A exits 0" [ "$CODE" -eq 0 ]
    check "$sc" "run A final text" grep -Fxq "run A final: session seeded" "$WORK/client.out"
    newest_log
    local logA="$LOG"
    check "$sc" "run A --verify-log ok" verify_log_ok "$logA"
    local evA
    evA=$("$DAEMON" --verify-log "$logA" | jq -r .events)
    stop_mock

    start_mock "$SCEN/resume_restart_b.json"
    run_client --session-dir "$SESS" --prompt "run B continues the session" --resume \
        --exec "$DAEMON" --adapter openai --model mock-model \
        --base-url "$MOCK_URL/v1" --api-key-env MOCK_KEY
    check "$sc" "run B (resume) exits 0" [ "$CODE" -eq 0 ]
    check "$sc" "run B final text" grep -Fxq "run B final: resumed and continued" "$WORK/client.out"
    check "$sc" "resume announces the session over the wire" grep -q "resumed session sess-" "$WORK/client.err"
    check "$sc" "resume line carries the derived title (run A's prompt)" \
        grep -q "title: run A prompt for the resume test" "$WORK/client.err"
    # Surface continuity: SAME durable log (no fork, no new session).
    newest_log
    check "$sc" "resume continued the SAME log (no fork)" [ "$LOG" = "$logA" ]
    local evB
    evB=$("$DAEMON" --verify-log "$logA" | jq -r .events)
    check "$sc" "event count grew across the restart (${evA:-?} → ${evB:-?})" \
        bash -c "[ '${evB:-0}' -gt '${evA:-1}' ]"
    check "$sc" "grown log --verify-log deterministic across the restart boundary" verify_log_ok "$logA"
    local n
    n=$(chat_count)
    check "$sc" "run B model called exactly once — got ${n:-none}" [ "${n:-0}" -eq 1 ]
    stop_mock
}

# sc_compaction (P5): the compaction wire crux over REAL binaries. A
# tiny --context-tokens budget makes ONE tool turn cross the pressure
# threshold; the turn-boundary trigger shadows the surface head behind
# the mock's summarize response (the summarize call IS an LLM call
# through the same adapter — the mock script accounts for it as step 2,
# BEFORE the continuation turn's final answer). Asserts: the client
# renders the one-line compaction notice; the daemon logs the shadow
# summary; the mock saw exactly 3 chat calls (turn, summarize,
# continuation); the summarize request is a KV-CACHE PREFIX of the
# running conversation (journal: req2's first messages == req1's, plus
# exactly one appended instruction); the durable log carries the full
# compaction bracket with citations; and the compacted log replays
# deterministically with the post-compaction message count (5 = summary
# + 2 retained results + continuation prompt + final answer; the
# un-compacted surface would be 6).
sc_compaction() {
    local sc=compaction
    fresh_session "$sc"
    start_mock "$SCEN/compaction.json"
    # 1200-char working brief: surface ≈ 1224 chars ≈ 306 estimated
    # tokens over a 300-token budget ⇒ pressure ≈ 1.02 ≥ 0.8. Post-
    # compaction surface ≈ 41 tokens ⇒ no re-trigger at later
    # boundaries.
    local big
    big=$(printf 'x%.0s' $(seq 1 1200))
    run_client --session-dir "$SESS" --prompt "$big" \
        --exec "$DAEMON" --adapter openai --model mock-model \
        --base-url "$MOCK_URL/v1" --api-key-env MOCK_KEY \
        --context-tokens 300
    check "$sc" "client exits 0 (compaction never fails the turn)" [ "$CODE" -eq 0 ]
    check "$sc" "final answer correct after compaction" \
        grep -Fxq "compaction e2e final: surface was shadowed and continued" "$WORK/client.out"
    check "$sc" "client rendered the one-line compaction notice" \
        grep -q "⤾ compacted: 2 events shadowed (generation 1)" "$WORK/client.err"
    check "$sc" "daemon logged the shadow summary" \
        grep -q "compaction: shadowed surface messages \[0,2)" "$WORK/client.err"
    local n
    n=$(chat_count)
    check "$sc" "mock saw exactly 3 chat calls (turn + summarize + continuation) — got ${n:-none}" \
        [ "${n:-0}" -eq 3 ]
    # KV-prefix proof from the request journal: the summarize request
    # (req 2) carries req 1's messages verbatim as its head, exactly
    # one appended instruction message, and the same tool ads.
    local journal kv pfx len4 lastr role tools
    journal=$(journal_raw)
    pfx=$(jq -c '[.[] | select(.path=="/v1/chat/completions")] | .[1].body.messages[0:2] == .[0].body.messages[0:2]' <<<"$journal")
    check "$sc" "summarize request is a KV prefix of the running conversation (journal)" \
        [ "$pfx" = "true" ]
    len4=$(jq -c '[.[] | select(.path=="/v1/chat/completions")] | .[1].body.messages | length' <<<"$journal")
    check "$sc" "summarize request = prefix + ONE instruction (4 messages) — got ${len4:-none}" \
        [ "${len4:-0}" -eq 4 ]
    lastr=$(jq -r '[.[] | select(.path=="/v1/chat/completions")] | .[1].body.messages[3].role' <<<"$journal")
    check "$sc" "appended instruction rides a user-role message — got ${lastr:-none}" \
        [ "$lastr" = "user" ]
    tools=$(jq -c '[.[] | select(.path=="/v1/chat/completions")] | .[1].body.tools | map(.function.name) | index("echo") != null' <<<"$journal")
    check "$sc" "summarize request advertises the same tools (echo present)" \
        [ "$tools" = "true" ]
    # Durable log: the full bracket, the citations, the generation.
    newest_log
    local cstart csum cend cgen csrc
    cstart=$(jq -s '[.[] | select(.type=="compaction/start")] | length' "$LOG")
    csum=$(jq -s '[.[] | select(.type=="compaction/summary")] | length' "$LOG")
    cend=$(jq -s '[.[] | select(.type=="compaction/end")] | length' "$LOG")
    check "$sc" "log carries compaction start+summary+end — ${cstart:-0}/${csum:-0}/${cend:-0}" \
        bash -c "[ '${cstart:-0}' -eq 1 ] && [ '${csum:-0}' -eq 1 ] && [ '${cend:-0}' -eq 1 ]"
    cgen=$(jq -s '[.[] | select(.type=="compaction/summary")] | .[0].payload.replaceGeneration' "$LOG")
    csrc=$(jq -s '[.[] | select(.type=="compaction/summary")] | .[0].payload.sourceEventSeqs | length' "$LOG")
    check "$sc" "summary cites 2 shadowed events at generation 1 — gen=${cgen:-?} srcs=${csrc:-?}" \
        bash -c "[ '${cgen:-0}' -eq 1 ] && [ '${csrc:-0}' -eq 2 ]"
    # Post-compaction surface + deterministic replay of the compacted
    # log: 5 messages (summary + 2 retained + continuation + final).
    local vmsg
    "$DAEMON" --verify-log "$LOG" >"$WORK/cv.out" 2>"$WORK/cv.err"
    vmsg=$(jq -r .messages "$WORK/cv.out")
    check "$sc" "verify-log reports the post-compaction surface (5 messages) — got ${vmsg:-none}" \
        [ "${vmsg:-0}" -eq 5 ]
    check "$sc" "compacted log --verify-log deterministic" verify_log_ok "$LOG"
    stop_mock
}

# sc_job_tailing (P6): the background run_shell round-trip over REAL
# binaries. The model dispatches a 5-tick producer (~2s) with
# background:true; the turn returns on the receipt (non-blocking). The
# client's drain loop polls jobs/status + jobs/output: mid-flight tails
# are CLIENT-SYNTHESIZED {"kind":"job-output"} NDJSON records on
# stdout. Asserts: receipt shape; at least one mid-flight (state
# running) partial chunk; the cursor chain (each record's offset ==
# the previous nextOffset — never re-serve, never skip); byte-exact
# reassembly of the FULL expected producer output at settlement;
# job/settled + job/report events in the stream; and the durable log
# (with the job lifecycle + exit-facts detail) replays deterministically.
sc_job_tailing() {
    local sc=job_tailing
    fresh_session "$sc"
    start_mock "$SCEN/job_tailing.json"
    run_client --session-dir "$SESS" --json --prompt "start the ticker in background" \
        --exec "$DAEMON" --adapter openai --model mock-model \
        --base-url "$MOCK_URL/v1" --api-key-env MOCK_KEY
    check "$sc" "client exits 0" [ "$CODE" -eq 0 ]
    check "$sc" "background receipt in the stream (jobId shell-1)" \
        grep -Fq '"jobId":"shell-1"' "$WORK/client.out"
    check "$sc" "receipt marks background:true" \
        grep -Fq '"background":true' "$WORK/client.out"
    check "$sc" "client-synthesized job-output records present" \
        grep -Fq '"kind":"job-output"' "$WORK/client.out"
    check "$sc" "mid-flight partial tail while running" \
        grep -Eq '"kind":"job-output".*"state":"running"' "$WORK/client.out"
    check "$sc" "settled-state tail records present" \
        grep -Eq '"kind":"job-output".*"state":"settled"' "$WORK/client.out"
    check "$sc" "settlement event streamed (job/settled completed)" \
        grep -Eq '"type":"job/settled".*"result":"completed"' "$WORK/client.out"
    check "$sc" "job/report notice streamed" \
        grep -Fq '"type":"job/report"' "$WORK/client.out"
    check "$sc" "final text delivered" grep -Fq "job tailing complete" "$WORK/client.out"
    # Cursor chain over the client's job-output records: consecutive
    # reads continue EXACTLY at the previous nextOffset (jq 1.6-safe:
    # capture-then-test, no -e).
    local chain
    chain=$(jq -s '[.[] | select(.kind=="job-output" and .jobId=="shell-1")] as $r
                   | ($r | length) > 0
                   and ([range(1; $r | length) | $r[.].offset == $r[. - 1].nextOffset] | all)' \
        "$WORK/client.out")
    check "$sc" "cursor chain exact (offset == previous nextOffset)" [ "$chain" = "true" ]
    # Byte-exact reassembly: concatenated chunks (by offset order) ==
    # the complete producer output — no re-served bytes, no gaps.
    local joined
    joined=$(jq -s -r '[.[] | select(.kind=="job-output" and .jobId=="shell-1")]
                | sort_by(.offset) | map(.chunk) | join("")' "$WORK/client.out")
    check "$sc" "reassembled tail is byte-exact" \
        [ "$joined" = "$(printf 'tick 1\ntick 2\ntick 3\ntick 4\ntick 5')" ]
    # B-F1 (hotfix 3): the ONE-TIME empty terminal record — the drain's
    # LAST job-output record is the deterministic end-of-tail marker
    # (empty chunk, settled, offset == nextOffset) even when the final
    # settled read carried bytes (jq 1.6-safe: last of a non-empty
    # stream; null record → false).
    local term
    term=$(jq -s '[.[] | select(.kind=="job-output" and .jobId=="shell-1")] | last
                  | .chunk == "" and .state == "settled"
                    and .offset == .nextOffset and .hasMore == false' \
        "$WORK/client.out")
    check "$sc" "one-time empty terminal record (settled, offset == nextOffset)" [ "$term" = "true" ]
    local n
    n=$(chat_count)
    check "$sc" "model called exactly twice (tool turn + final turn) — got ${n:-none}" \
        [ "${n:-0}" -eq 2 ]
    newest_log
    # Durable log: the full job lifecycle + the exit-facts detail.
    local settledetail
    settledetail=$(jq -c 'select(.type == "job/settled" and .payload.jobId == "shell-1")
                    | .payload.detail // empty' "$LOG" | head -1)
    check "$sc" "job/settled carries exit facts detail" \
        bash -c "[[ '$settledetail' == *'cause=exit exitCode=0'* ]]"
    local report
    report=$(jq -c 'select(.type == "job/report" and .payload.jobId == "shell-1")' "$LOG" | head -1)
    check "$sc" "job/report landed in the durable log" [ -n "$report" ]
    check "$sc" "--verify-log ok" verify_log_ok "$LOG"
    stop_mock
}

# sc_skills (P7): the three-tier native skills delivery over the real
# binaries, with a fixture catalog of three skills — clean-one (with a
# tier-3 reference and an inert script), brackets (angle-laden
# description proving sanitization), broken (name/folder mismatch
# proving per-skill fail-closed exclusion + the honest startup line).
# The scripted model calls skill_load for the clean body, the tier-3
# reference, an unknown name (typed fail-closed error), and a traversal
# ref (confinement rejection). THE CRUX: after the run, the fixture
# SKILL.md is MUTATED on disk and --verify-log must print byte-identical
# output — replay derives tool content from the LOG, never disk.
sc_skills() {
    local sc=skills
    fresh_session "$sc"
    start_mock "$SCEN/skills.json"
    run_client --session-dir "$SESS" --json --prompt "exercise the skills surface" \
        --exec "$DAEMON" --skills-dir "$SCEN/skills-catalog" \
        --adapter openai --model mock-model \
        --base-url "$MOCK_URL/v1" --api-key-env MOCK_KEY
    check "$sc" "client exits 0" [ "$CODE" -eq 0 ]
    check "$sc" "final text delivered" grep -Fq "skills round trip complete" "$WORK/client.out"
    # Startup honesty: loaded count + one per-skill exclusion warning.
    check "$sc" "daemon logs the loaded-skill count" \
        grep -q "skills: 2 loaded from" "$WORK/client.err"
    check "$sc" "malformed skill excluded with a warning naming it" \
        grep -q "skills: excluding \"broken\"" "$WORK/client.err"
    # (a) Tier 1: the journal proves names + descriptions advertised in
    # the SYSTEM prompt — sanitized (no raw angle tokens).
    local journal
    journal=$(journal_raw)
    check "$sc" "journal advertises clean-one in the system prompt" \
        grep -q "clean-one: The clean fixture skill for validation." <<<"$journal"
    check "$sc" "journal carries the SANITIZED brackets description" \
        grep -q "brackets: Uses angle markers and /system tokens to prove sanitization." <<<"$journal"
    if grep -q '<angle>' <<<"$journal"; then
        fail "$sc" "raw angle tokens leaked into the system prompt"
    else
        PASS=$((PASS + 1))
        note "  ok: no raw angle tokens in the system prompt (sanitization)"
    fi
    # (b) Tier 2/3: body + ceiling footer + reference payload in the
    # --json tool-result stream.
    check "$sc" "whole-body load returns the body" \
        grep -Fq "Body instructions here." "$WORK/client.out"
    check "$sc" "load result carries the allowed-tools ceiling footer" \
        grep -Fq "allowed-tools ceiling: run_shell, read (intersected with the registry — never a grant)" "$WORK/client.out"
    check "$sc" "tier-3 reference load returns its payload" \
        grep -Fq "tier-three reference payload" "$WORK/client.out"
    # (d) Fail-closed rejections: unknown name + traversal ref. Quotes
    # are JSON-escaped in the --json NDJSON stream (sandbox_denial
    # idiom: grep for the escaped form).
    check "$sc" "unknown skill name rejected fail-closed" \
        grep -Fq 'unknown skill \"does-not-exist\"' "$WORK/client.out"
    check "$sc" "traversal ref rejected by confinement" \
        grep -Fq "rejected by confinement policy [escape]" "$WORK/client.out"
    # Durable log: the log-only skill/loaded provenance events (2 — one
    # per successful load, with the ref on the tier-3 one). jq 1.6:
    # capture-then-test, not jq -e.
    newest_log
    local loadevents
    loadevents=$(jq -s '[.[] | select(.type == "skill/loaded")] | length' "$LOG")
    check "$sc" "two skill/loaded provenance events — got ${loadevents:-none}" \
        [ "${loadevents:-0}" -eq 2 ]
    local refsha
    refsha=$(jq -s '[.[] | select(.type == "skill/loaded" and .payload.ref == "references/note.md")]
                    | .[0].payload.sha256 // empty' "$LOG")
    check "$sc" "tier-3 provenance event carries ref + sha256" [ -n "$refsha" ]
    # Verify-log deterministic BEFORE mutation (baseline green).
    check "$sc" "--verify-log deterministic ok (pre-mutation)" verify_log_ok "$LOG"
    # (c) THE CRUX — replay-after-mutation: mutate the clean SKILL.md on
    # disk, replay again, output must be byte-identical.
    printf -- '---\nname: clean-one\ndescription: MUTATED post-run description.\n---\n\n# MUTATED BODY\n' \
        >"$SCEN/skills-catalog/clean-one/SKILL.md"
    "$DAEMON" --verify-log "$LOG" >"$WORK/skills-v3.out" 2>"$WORK/skills-v3.err"; local c3=$?
    "$DAEMON" --verify-log "$LOG" >"$WORK/skills-v4.out" 2>"$WORK/skills-v4.err"; local c4=$?
    check "$sc" "replay after on-disk mutation still exits 0" [ "$c3" -eq 0 ] && [ "$c4" -eq 0 ]
    check "$sc" "replay output byte-identical to pre-mutation (log is authority, not disk)" \
        cmp -s "$WORK/v1.out" "$WORK/skills-v3.out"
    if grep -q "MUTATED" "$WORK/skills-v3.out"; then
        fail "$sc" "mutated disk content leaked into replay output"
    else
        PASS=$((PASS + 1))
        note "  ok: no mutated disk content in replay output"
    fi
    # Restore the fixture (the scenarios dir is image content; leave it
    # as shipped for any rerun).
    printf -- '%s\n' \
        '---' \
        'name: clean-one' \
        'description: The clean fixture skill for validation.' \
        'allowed-tools: run_shell, read' \
        '---' \
        '' \
        '# Clean one' \
        '' \
        'Body instructions here.' \
        >"$SCEN/skills-catalog/clean-one/SKILL.md"
    local n
    n=$(chat_count)
    check "$sc" "model called exactly 4 times (3 tool turns + final) — got ${n:-none}" \
        [ "${n:-0}" -eq 4 ]
    stop_mock
}

# sc_mcp (P8/P8.2): the MCP-host crux over REAL binaries, THREE runs
# against ONE mockmcp HTTP server + one config covering BOTH transport
# shapes plus the two fail-closed twins: localmock (vh-mockmcp stdio
# subprocess), remotemock (vh-mockmcp --http --sse on a computed port,
# URL carrying a FAKE TOKEN in the path — the redaction proof),
# garbagemock (stdio whose tools/call responses are garbage — error
# not crash), and deadmock (remote at a dead port — the degraded
# posture).
#
#   (a) DEFAULT POSTURE (P8.2 — the gap that hid the operator's live
#       finding): NO --policy, NO --ask-tools, non-interactive client
#       (EOF stdin). The mcp tool calls ASK: approval/request on the
#       wire, the EOF responder denies fail-closed, the tools NEVER
#       execute (the http mock's /calls counter stays 0; the stdio
#       twin's denial is proven from the typed denied tool/result),
#       the turn still completes, replay stays deterministic.
#   (b) POLICY PROMOTION: --policy with exact-name allows plus the
#       per-server underscore glob (mcp_remotemock_*) grants the asks —
#       the SAME tools round-trip content through BOTH transports, the
#       degraded sentinel + garbage-call errors land with the turn
#       completing, names advertised in the model request (journal),
#       tool/call+tool/result on the durable log, deterministic
#       --verify-log replay, and ZERO occurrences of the fake URL
#       token in daemon stderr and the session log.
#   (c) AUTO-ALLOW OPT-IN: --mcp-auto-allow registers no mcp ask —
#       the call round-trips with no approval on the wire (the
#       pre-P8.2 posture as an explicit operator choice).
MCPMCP=/usr/local/bin/vh-mockmcp
MCPMCP_PORT=8177
MCPMCP_PID=""
sc_mcp() {
    local sc=mcp
    # The MCP remote server: vh-mockmcp over HTTP with SSE framing (the
    # harder response shape exercised over the real wire).
    "$MCPMCP" --http "127.0.0.1:${MCPMCP_PORT}" --sse >"$WORK/mockmcp.out" 2>"$WORK/mockmcp.err" &
    MCPMCP_PID=$!
    local i mcp_ready=0
    for i in $(seq 1 100); do
        if http_get_port "${MCPMCP_PORT}" /healthz 2>/dev/null | grep -q '"ok"'; then
            mcp_ready=1
            break
        fi
        sleep 0.1
    done
    if [ "$mcp_ready" -ne 1 ]; then
        fail "$sc" "vh-mockmcp http server failed to become healthy"
        kill "$MCPMCP_PID" 2>/dev/null
        return 0
    fi
    # The config: both shapes + the garbage and dead twins. The remote
    # URL embeds a fake token in the path (credential-posture proof).
    local token="fake-path-token-77aa88bb99cc"
    cat >"$WORK/mcp-config.json" <<MCPJSON
{
  "localmock":  {"type": "local",  "command": ["vh-mockmcp", "--stdio"]},
  "remotemock": {"type": "remote", "url": "http://127.0.0.1:${MCPMCP_PORT}/${token}/mcp"},
  "garbagemock":{"type": "local",  "command": ["vh-mockmcp", "--stdio", "--call-garbage"]},
  "deadmock":   {"type": "remote", "url": "http://127.0.0.1:1/mcp"}
}
MCPJSON
    local base_cfg=(
        --exec "$DAEMON" --mcp-config "$WORK/mcp-config.json" --mcp-timeout-ms 15000
        --adapter openai --model mock-model
        --base-url "$MOCK_URL/v1" --api-key-env MOCK_KEY
    )

    # ---- (a) DEFAULT POSTURE: ask, EOF-deny fail-closed, no execution.
    fresh_session "$sc-default"
    start_mock "$SCEN/mcp_default.json"
    run_client --session-dir "$SESS" --json \
        --prompt "try the mcp tools with nothing configured" \
        "${base_cfg[@]}"
    check "$sc" "(a) client exits 0 (no hang — got $CODE)" [ "$CODE" -ne 124 ]
    check "$sc" "(a) client exits cleanly" [ "$CODE" -eq 0 ]
    check "$sc" "(a) final text delivered after the denials" \
        grep -Fq "mcp default posture complete" "$WORK/client.out"
    check "$sc" "(a) startup posture line (ask-by-default)" \
        grep -q "mcp tools: ask-by-default" "$WORK/client.err"
    # The asks surfaced on the wire: the --json stream carries the
    # approval/request (approvalId + the ask-by-default reason).
    check "$sc" "(a) approval/request on the wire" grep -q "approvalId" "$WORK/client.out"
    check "$sc" "(a) ask reason states the posture" grep -q "ask-by-default" "$WORK/client.out"
    check "$sc" "(a) typed denials name the mcp-ask source" \
        grep -q "denied by mcp-ask" "$WORK/client.out"
    # NOT executed: no echo content anywhere, and the http mock's
    # execution counter is ZERO.
    if grep -q "echo: must-not-run" "$WORK/client.out"; then
        fail "$sc" "(a) a denied mcp tool EXECUTED (echo content present)"
    else
        PASS=$((PASS + 1)); note "  ok: (a) no denied tool's content ever produced"
    fi
    local calls_a
    calls_a=$(http_get_port "${MCPMCP_PORT}" /calls | grep -o '"total":[0-9]*' | cut -d: -f2)
    check "$sc" "(a) mockmcp executed 0 tool calls — got ${calls_a:-none}" [ "${calls_a:-1}" -eq 0 ]
    newest_log
    local dcount
    dcount=$(jq -s '[.[] | select(.type == "tool/result" and .payload.denied == true)] | length' "$LOG")
    check "$sc" "(a) log carries 2 typed denials — got ${dcount:-none}" [ "${dcount:-0}" -eq 2 ]
    local dby
    dby=$(jq -s '[.[] | select(.type == "tool/result" and .payload.deniedBy == "mcp-ask")] | length' "$LOG")
    check "$sc" "(a) denials attributed to mcp-ask — got ${dby:-none}" [ "${dby:-0}" -eq 2 ]
    check "$sc" "(a) --verify-log deterministic" verify_log_ok "$LOG"
    local n_a
    n_a=$(chat_count)
    check "$sc" "(a) model called exactly twice (tool turn + final) — got ${n_a:-none}" [ "${n_a:-0}" -eq 2 ]
    stop_mock

    # ---- (b) POLICY PROMOTION: exact names + the per-server glob.
    fresh_session "$sc-policy"
    start_mock "$SCEN/mcp.json"
    run_client --session-dir "$SESS" --json \
        --policy "$SCEN/mcp.policy" \
        --prompt "exercise both mcp transports" \
        "${base_cfg[@]}"
    check "$sc" "(b) client exits 0 (no hang — got $CODE)" [ "$CODE" -ne 124 ]
    check "$sc" "(b) client exits cleanly" [ "$CODE" -eq 0 ]
    check "$sc" "(b) policy loaded (4 rules)" grep -q "policy loaded (4 rules)" "$WORK/client.err"
    check "$sc" "(b) policy auto-ALLOWED the exact-named stdio echo" \
        grep -q "policy: allow mcp_localmock_echo" "$WORK/client.err"
    check "$sc" "(b) policy auto-ALLOWED via the underscore glob" \
        grep -q "policy: allow mcp_remotemock_echo" "$WORK/client.err"
    check "$sc" "(b) final text delivered" grep -Fq "mcp round trip complete" "$WORK/client.out"
    # Round-trip content through BOTH transports (the --json stream
    # carries tool result content).
    check "$sc" "(b) stdio transport round trip" grep -Fq "echo: stdio-payload" "$WORK/client.out"
    check "$sc" "(b) http(SSE) transport round trip" grep -Fq "echo: http-payload" "$WORK/client.out"
    # Startup honesty: per-server lines + summary; the degraded twin
    # names its server (typed, credential-free).
    check "$sc" "(b) stdio server startup line" grep -q "mcp: localmock (stdio) up — 3 tool(s)" "$WORK/client.err"
    check "$sc" "(b) remote server startup line" grep -q "mcp: remotemock (remote) up — 3 tool(s)" "$WORK/client.err"
    check "$sc" "(b) dead server DEGRADED startup line" grep -q "mcp: deadmock DEGRADED" "$WORK/client.err"
    check "$sc" "(b) startup summary counts" \
        grep -q "mcp: 3 server(s) connected, 1 degraded; 9 tool(s) registered" "$WORK/client.err"
    # Fail-closed twins: the typed degraded error (sentinel call) and
    # the garbage-call error — errors, not crashes; the turn completed.
    check "$sc" "(b) degraded sentinel typed error" \
        grep -Fq "mcp: server deadmock is degraded" "$WORK/client.out"
    check "$sc" "(b) garbage call fails with an error" \
        grep -Fq "tool mcp_garbagemock_echo failed" "$WORK/client.out"
    # Namespaced names advertised to the model (journal = request
    # plane): both transports' echo tools present.
    local journal
    journal=$(journal_raw)
    check "$sc" "(b) journal advertises mcp_localmock_echo" grep -q "mcp_localmock_echo" <<<"$journal"
    check "$sc" "(b) journal advertises mcp_remotemock_echo" grep -q "mcp_remotemock_echo" <<<"$journal"
    check "$sc" "(b) journal carries the degraded sentinel mcp_deadmock" grep -q '"mcp_deadmock"' <<<"$journal"
    # Durable log: 4 tool calls, 4 tool results, all EXECUTED (none
    # denied — the policy granted every ask).
    newest_log
    local calls results denied_b
    calls=$(jq -s '[.[] | select(.type == "tool/call")] | length' "$LOG")
    results=$(jq -s '[.[] | select(.type == "tool/result")] | length' "$LOG")
    denied_b=$(jq -s '[.[] | select(.type == "tool/result" and .payload.denied == true)] | length' "$LOG")
    check "$sc" "(b) log carries 4 tool/call events — got ${calls:-none}" [ "${calls:-0}" -eq 4 ]
    check "$sc" "(b) log carries 4 tool/result events — got ${results:-none}" [ "${results:-0}" -eq 4 ]
    check "$sc" "(b) zero denials under the granting policy — got ${denied_b:-none}" [ "${denied_b:-1}" -eq 0 ]
    # Replay determinism (MCP results are logged content like every
    # tool — replay needs no server).
    check "$sc" "(b) --verify-log deterministic" verify_log_ok "$LOG"
    # THE REDACTION CRUX: the fake URL token appears ZERO times in the
    # daemon stderr AND the durable session log.
    local tok_err tok_log
    tok_err=$(grep -c "$token" "$WORK/client.err" || true)
    tok_log=$(grep -c "$token" "$LOG" || true)
    check "$sc" "(b) URL token absent from daemon stderr (${tok_err:-0} hits)" [ "${tok_err:-0}" -eq 0 ]
    check "$sc" "(b) URL token absent from session log (${tok_log:-0} hits)" [ "${tok_log:-0}" -eq 0 ]
    local n
    n=$(chat_count)
    check "$sc" "(b) model called exactly 3 times (2 tool turns + final) — got ${n:-none}" [ "${n:-0}" -eq 3 ]
    stop_mock

    # ---- (c) AUTO-ALLOW OPT-IN: no ask, direct execution.
    fresh_session "$sc-auto"
    start_mock "$SCEN/mcp_auto.json"
    local calls_before_c
    calls_before_c=$(http_get_port "${MCPMCP_PORT}" /calls | grep -o '"total":[0-9]*' | cut -d: -f2)
    # --mcp-auto-allow rides AFTER --exec (it is a DAEMON flag).
    run_client --session-dir "$SESS" --json \
        --prompt "auto-allow path" \
        "${base_cfg[@]}" --mcp-auto-allow
    check "$sc" "(c) client exits 0 (no hang — got $CODE)" [ "$CODE" -ne 124 ]
    check "$sc" "(c) client exits cleanly" [ "$CODE" -eq 0 ]
    check "$sc" "(c) startup posture line (auto-allow opt-in)" \
        grep -q "mcp tools: auto-allow (operator opt-in via --mcp-auto-allow" "$WORK/client.err"
    check "$sc" "(c) round trip without asking" grep -Fq "echo: auto-allowed-payload" "$WORK/client.out"
    if grep -q "approvalId" "$WORK/client.out"; then
        fail "$sc" "(c) an approval was requested under --mcp-auto-allow"
    else
        PASS=$((PASS + 1)); note "  ok: (c) no approval/request on the opt-in path"
    fi
    local calls_c
    calls_c=$(http_get_port "${MCPMCP_PORT}" /calls | grep -o '"total":[0-9]*' | cut -d: -f2)
    check "$sc" "(c) mockmcp executed exactly 1 more tool call — ${calls_before_c:-?}→${calls_c:-none}" \
        [ "$((calls_c - calls_before_c))" -eq 1 ]
    local n_c
    n_c=$(chat_count)
    check "$sc" "(c) model called exactly twice (tool turn + final) — got ${n_c:-none}" [ "${n_c:-0}" -eq 2 ]
    stop_mock

    kill "$MCPMCP_PID" 2>/dev/null
    wait "$MCPMCP_PID" 2>/dev/null
    MCPMCP_PID=""
}

# ---- run the battery -------------------------------------------------------

note "=== native-engine docker self-test battery ==="
run_scenario greeting sc_greeting
run_scenario tool_roundtrip sc_tool_roundtrip
run_scenario policy_bad_file sc_policy_bad_file
run_scenario policy_loaded_clean sc_policy_loaded_clean
run_scenario empty_response_retry sc_empty_response_retry
run_scenario sandbox_denial sc_sandbox_denial
run_scenario retry_ladder sc_retry_ladder
run_scenario subagent_recursion sc_subagent_recursion
run_scenario spill_paging sc_spill_paging
run_scenario anthropic_dialect sc_anthropic_dialect
run_scenario verify_determinism sc_verify_determinism
run_scenario approval_flow sc_approval_flow
run_scenario resume_restart sc_resume_restart
run_scenario compaction sc_compaction
run_scenario job_tailing sc_job_tailing
run_scenario skills sc_skills
run_scenario mcp sc_mcp

TOTAL=$((PASS + FAIL))
echo
note "assertions: ${PASS}/${TOTAL} passed"
note "SELFTEST: ${SCEN_PASS}/${SCEN_TOTAL} scenarios passed"
[ "$FAIL" -eq 0 ] || { note "failed scenarios:${FAILED}"; exit 1; }
exit 0
