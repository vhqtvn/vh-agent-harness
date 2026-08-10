# hermes-agent — Providers & Adapters (source packet)
**Source:** `refs/hermes-agent/` @ HEAD `005421d888a40865cc61d143ff77efd87a037a1e` on `main` (Nous Research, MIT, v0.20.0). Gitignored reference transport in this repo; NOT this repo's own code.
**Method:** files read in BODY off local disk; giant files sampled at structurally important sections, not dumped whole.
**Status:** NEUTRAL study. Borrow/reject verdicts characterize fit vs vh-agent-harness discipline (static-Go, small-boundary, token-stable, overlay-based, narrow-waist); they are NOT commands to change this repo.

> See `hermes-agent-internals.md` for the consolidated study and the cross-check corrections.

## §7 Providers & adapters (slice 3).
- Master resolver `resolve_runtime_provider()` (`hermes_cli/runtime_provider.py:1665`) returns {provider,api_mode,base_url,api_key,source} via 19-step precedence chain. (source=refs/hermes-agent/hermes_cli/runtime_provider.py:1665, confidence=high, type=fact)
- ~35 providers in `PROVIDER_REGISTRY` (`hermes_cli/auth.py:211`), auto-extended from `plugins/model-providers/`. (source=refs/hermes-agent/hermes_cli/auth.py:211, confidence=high, type=fact)
- **CONTRADICTION: 5 api_modes, NOT 3.** chat_completions, codex_responses, anthropic_messages, **bedrock_converse**, **codex_app_server**. All dispatched in `conversation_loop.py:2547,2659`. (source=refs/hermes-agent/providers.py, confidence=high, type=fact)
- `api_mode` derivation (`providers.py:671`): host-mandated → Nous dual-wire → TRANSPORT_TO_API_MODE → bedrock carve-out → default chat_completions. Host-mandated (`providers.py:614`): `api.anthropic.com`→anthropic_messages; official OpenAI→codex_responses; `bedrock-runtime.*`→bedrock_converse; `api.kimi.com/coding`→anthropic_messages. (source=refs/hermes-agent/providers.py:671, confidence=high, type=fact)
- Transport ABC `agent/transports/base.py:16` ProviderTransport: format conversion only; client/stream/retry lifecycle stays on AIAgent. Registered via `register_transport("api_mode", Class)`. (source=refs/hermes-agent/agent/transports/base.py:16, confidence=high, type=fact)
- **"Universal OpenAI client" is LEAKY:** bedrock bypasses via boto3 `converse()` (`chat_completion_helpers.py:540`); gemini-native uses hand-built `GeminiNativeClient` facade (`gemini_native_adapter.py:956`). 2 of 5 modes bypass. (source=refs/hermes-agent/chat_completion_helpers.py:540, confidence=high, type=fact)
- Prompt caching `agent/prompt_caching.py` (394 lines): **4 cache_control breakpoints**; default layout splits system on static_system_prefix (2 breakpoints) + last 2 carrier-eligible messages; tool-cache layout variant caches tools[-1] schema. TTL 5m/1h. `static_system_prefix` = the stable tier. strip-before-failover safety. (source=refs/hermes-agent/agent/prompt_caching.py, confidence=high, type=fact)

## Verification
| Claim | Verifying command/read | Verified |
|-------|------------------------|----------|
| 5 api_modes instead of 3 | Structural code read pass | yes |
| Bedrock bypasses Universal OpenAI client | Structural code read pass | yes |

## Contradictions
1. 5 api_modes NOT 3 (bedrock_converse + codex_app_server beyond prior framing).
2. "Universal OpenAI client" leaky (bedrock + gemini-native bypass; 2 of 5 modes).

## Could-not-verify flags
- Giant files were structurally sampled; exhaustive line-by-line verification of all edge-case conditionals was not performed. (confidence=medium, type=inference)