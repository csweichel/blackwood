# Codex-backed Search and Chat Specification

## Goal

Integrate Codex into Blackwood and replace the current RAG search and conversation flow with Codex-driven note interaction. All agentic or RAG-style behavior that answers from notes must go through Codex instead of `internal/rag` or direct OpenAI chat completions.

The first implementation should use the raw Codex CLI as the integration surface. This is the easiest fit for the current single-binary Blackwood architecture because it avoids running or supervising a separate app server and lets the existing Connect/HTTP APIs remain mostly stable.

## Requirements

- Use the installed `codex` CLI in non-interactive mode.
- Assume the CLI is already authenticated on hosts where the feature is meant to work.
- If the CLI is missing, cannot run non-interactively, is not authenticated, or returns an auth/setup error, Codex-backed functionality must be disabled and return a clear unavailable error.
- `ChatService` should remain the browser-facing conversation API. Preserve the existing proto shape unless implementation proves a new field is strictly required.
- Multi-turn conversations must work. Blackwood should persist user and assistant messages in SQLite as it does today and replay the conversation transcript into Codex for every turn. If the installed Codex CLI exposes a reliable resumable session ID, store and use it as an optimization, but the required behavior must not depend on it.
- `/api/search` must be backed by Codex, not the semantic index. Keep the response shape compatible with the existing `SearchPage` where possible: `entry_id`, `date`, `snippet`, and `score`.
- Chat source chips should continue to work. Codex responses should include structured source metadata with at least a date and snippet. `entry_id` may be a deterministic synthetic ID such as `codex:<date>:<hash>` when the source is not a storage entry.
- Daily-note summarization and nightly digest currently depend on `rag.Engine`. During this migration, port those calls to Codex or disable them when Codex is unavailable so no direct OpenAI chat-completion path remains for agentic note interaction.
- Existing capture, editing, import, OCR, transcription, WhatsApp, Telegram, Granola, and PDF behavior should remain outside this change except for the loss of semantic index dependencies where they are no longer needed.

## Constraints

- Codex must not be able to modify note content in the real markdown directory under `server.data_dir/notes`.
- Codex must not ask for file-read permissions during normal chat or search. The implementation should run Codex from the notes storage directory with approval prompts disabled and a read-only filesystem policy.
- Run Codex with the real notes storage directory, `server.data_dir/notes`, as its workspace and working directory.
- Do not pass the absolute notes directory path in prompts or generated context. Use relative note paths in prompts and source metadata.
- If the installed Codex CLI cannot run non-interactively with read access to the notes storage directory and no write access to note files, disable Codex-backed functionality.
- Treat note content as untrusted data. Notes may contain prompt-injection text and must not be allowed to override Blackwood's generated task instructions.
- Do not pass Blackwood secrets to the Codex process. Use a minimal environment that includes only what the CLI needs to find its login state, such as `PATH`, `HOME`, `CODEX_HOME` if set, and temp-related variables.
- Honor request cancellation. If the HTTP/Connect request is canceled, terminate the Codex subprocess.
- Add timeouts and bounded output sizes so a stuck or noisy Codex run cannot block the server indefinitely or exhaust memory.
- Preserve local-first behavior. If Codex is unavailable, note editing and capture must continue to work.
- Update `README.md`, `blackwood.example.yaml`, and `AGENTS.md` as part of implementation because this changes packages, configuration, and the AI/search architecture.

## Architecture

### New Package

Add `internal/codex` to own the integration.

Suggested exported types:

- `Config`: resolved Codex settings such as path, enabled flag, timeout, notes workspace root, max corpus bytes, max output bytes, and extra CLI args.
- `Engine`: high-level API used by handlers: `Available`, `Chat`, `Search`, and `Summarize`.
- `Runner`: small interface for executing Codex. Production implementation wraps `exec.CommandContext`; tests use a fake runner.
- `SourceReference`: Codex-backed source metadata matching the API shape used by chat/search.
- `Message`: role/content conversation message, replacing the current dependency on `rag.Message`.

### Configuration

Add a `codex` config block to `internal/config`.

Suggested YAML:

```yaml
codex:
  enabled: true
  path: codex
  timeout: 2m
  max_corpus_bytes: 10485760
  max_output_bytes: 1048576
  extra_args: []
```

Behavior:

- Model `enabled` as an optional boolean in Go so omitted can be distinguished from explicit `false`.
- If the block or `enabled` value is omitted, default to auto-detection: try to enable if `codex` is on `PATH`; otherwise mark unavailable.
- If `enabled: false`, do not probe and always disable Codex features.
- If `enabled: true`, probe on startup and log a warning if unavailable without crashing the server.
- Codex's workspace directory is always the notes storage directory at `server.data_dir/notes`; it is not separately configurable.
- Environment fallback may be added for `CODEX_PATH`, but avoid adding secret-style config because authentication is owned by the Codex CLI.

### Codex Execution Mode

Use raw CLI mode via a per-request subprocess.

Implementation details:

- Call Codex with arguments that disable approval prompts and restrict filesystem access to a read-only view of the notes workspace. The implementation should verify exact flag names against the installed CLI during development. The intended policy is equivalent to:
  - no approval prompts,
  - read-only workspace sandboxing,
  - non-interactive execution,
  - machine-readable output when available.
- Run the process with `cmd.Dir` set to `server.data_dir/notes`, never the repository or `server.data_dir`.
- Use `exec.CommandContext` directly, not shell interpolation.
- Provide the task prompt through stdin or an argument supported by the CLI.
- Prefer structured or JSON CLI output if available. If the local CLI only returns plain text, buffer stdout/stderr and parse the final response according to the generated prompt contract.

The initial implementation may stream a single final chunk to the existing Connect stream after Codex completes. Token-level streaming is optional and should only be added if the CLI exposes a stable event stream.

### Notes Workspace

Codex operates directly in the notes storage directory at `server.data_dir/notes`.

Expected note layout:

```text
notes/
  2026/
    06/
      01/
        index.md
```

Rules:

- Build a request-time manifest from storage and include it in the Codex prompt through stdin or another non-file prompt channel.
- Use relative paths only in prompts, manifests, and source metadata.
- Include date metadata for every note in the manifest.
- Do not write `BLACKWOOD_TASK.md`, `NOTES_MANIFEST.json`, or any other generated file into the notes storage directory.
- Enforce `max_corpus_bytes` by summing candidate note content from storage before invoking Codex.
- If the corpus exceeds `max_corpus_bytes`, return a clear error for v1 rather than silently falling back to app-side search. Later work can add paging, date narrowing, or a Codex-managed index.

### Prompt Contracts

Generate task instructions that clearly separate Blackwood's trusted instructions from untrusted note content.

Chat prompt contract:

- Inputs: current user message, prior conversation transcript, and the notes workspace manifest.
- Codex task: answer from the notes when relevant, say when the notes do not contain enough information, cite dates naturally, and return structured JSON.
- Output JSON:

```json
{
  "answer": "string",
  "sources": [
    {
      "date": "YYYY-MM-DD",
      "snippet": "short relevant excerpt",
      "score": 0.0
    }
  ]
}
```

Search prompt contract:

- Inputs: query, limit, and the notes workspace manifest.
- Codex task: inspect the notes and return the most relevant matches.
- Output JSON:

```json
{
  "results": [
    {
      "date": "YYYY-MM-DD",
      "snippet": "short relevant excerpt",
      "score": 0.0
    }
  ]
}
```

Summary prompt contract:

- Inputs: one note's content.
- Codex task: produce the same kind of one-sentence summary currently generated by `internal/rag`.
- Output JSON:

```json
{
  "summary": "string"
}
```

The parser should accept fenced JSON as a convenience but reject malformed or missing required fields.

### API Rewiring

- Change `internal/api.ChatHandler` to depend on a small Codex chat interface instead of `*rag.Engine`.
- Keep conversation creation, message persistence, list, and get behavior in `internal/storage`.
- Convert Codex sources into existing proto `SourceReference` values.
- Register `ChatService` even when Codex is unavailable so clients receive a proper Connect `unavailable` error instead of falling through to the SPA handler.
- Change `internal/api/search.go` to call the Codex search interface instead of `index.Indexer`.
- Change `internal/api/summarize.go` and `internal/api/digest.go` to use the Codex summary interface.
- In `cmd/blackwood/main.go`, initialize the Codex engine after storage setup and wire chat/search/summarize/digest to it.
- Stop constructing `rag.Engine` for chat. Remove `internal/rag` if it becomes unused after summaries/digest are ported.
- Keep OpenAI/QMD indexing only if another non-chat/search feature still needs it. Otherwise stop enabling the semantic index as part of chat/search availability.

### Frontend Changes

- Keep routes `/chat`, `/chat/:slug`, and `/search`.
- Update unavailable copy from "Configure an OpenAI API key" to "Codex CLI is not available" or equivalent.
- Do not show a misleading "semantic search" label if search is now Codex-backed.
- No UI redesign is required.

## Implementation Steps

1. Add Codex configuration to `internal/config`, defaults, config tests, and `blackwood.example.yaml`.
2. Add `internal/codex` with runner, availability probing, notes manifest generation, prompt generation, output parsing, and fake-runner tests.
3. Rewire `ChatHandler` to the Codex interface while preserving conversation storage and Connect response behavior.
4. Rewire `/api/search`, daily summarization, and nightly digest to Codex.
5. Update `cmd/blackwood/main.go` to initialize and pass the Codex engine, disable Codex-backed features cleanly when unavailable, and remove RAG wiring.
6. Update frontend error/help text for chat and search.
7. Remove or leave unused legacy RAG/index wiring only after confirming no remaining package depends on it. Delete `internal/rag` if it is fully unused.
8. Update `README.md` and `AGENTS.md` to document the new Codex-backed architecture and configuration.
9. Run verification: `make test` and `cd web && npm run lint`. Run build checks if frontend or static embedding behavior changed.

## Tests

- Config defaults and env/config override tests for the new Codex settings.
- Codex availability tests:
  - missing CLI disables feature,
  - nonzero auth/setup output disables feature,
  - explicit `enabled: false` skips probing.
- Runner tests using a fake executable or fake `Runner`:
  - chat parses valid JSON and sources,
  - search parses valid JSON results,
  - summary parses valid JSON,
  - malformed JSON returns an error,
  - context cancellation terminates the run,
  - output over `max_output_bytes` fails.
- Workspace tests:
  - Codex runs with `cmd.Dir` set to `server.data_dir/notes`,
  - generated prompt and source metadata use relative note paths, not the absolute notes path,
  - feature availability fails when read-only sandboxing cannot be configured,
  - corpus size cap is enforced.
- API tests:
  - `ChatService.Chat` creates and continues conversations with a fake Codex engine,
  - `ListConversations` and `GetConversation` work when Codex is available,
  - chat/search/summarize return unavailable when Codex is disabled,
  - `/api/search` returns the existing JSON shape.
- Regression test that no chat/search/summarize path calls direct OpenAI chat completions.

## Success Criteria

- Chat questions in the existing `/chat` UI are answered by Codex and persisted as multi-turn conversations.
- Search requests from `/search` are answered by Codex and return date/snippet results compatible with the current UI.
- Daily summarization and nightly digest no longer call direct OpenAI chat completions.
- When `codex` is absent or unusable, chat/search/summarize fail with clear unavailable errors while normal note editing and capture continue.
- The Codex process runs from the real markdown notes directory but does not receive write access to note files or prompt-visible absolute notes paths.
- There is no remaining RAG/search/conversation dependency on `internal/rag` or direct OpenAI chat completions.
- Documentation and `AGENTS.md` reflect the new `internal/codex` package and Codex-backed search/chat conventions.
- `make test` passes.
- `cd web && npm run lint` passes.
