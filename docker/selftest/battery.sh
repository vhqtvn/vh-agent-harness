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
    exec 3<>"/dev/tcp/${MOCK_ADDR}/${MOCK_PORT}" || return 1
    printf 'GET %s HTTP/1.0\r\nHost: mock\r\nConnection: close\r\n\r\n' "$1" >&3
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

TOTAL=$((PASS + FAIL))
echo
note "assertions: ${PASS}/${TOTAL} passed"
note "SELFTEST: ${SCEN_PASS}/${SCEN_TOTAL} scenarios passed"
[ "$FAIL" -eq 0 ] || { note "failed scenarios:${FAILED}"; exit 1; }
exit 0
