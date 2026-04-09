# Blackwood — Agent Guide

Blackwood is a local-first daily notes app. Single Go binary serves a React frontend over one port. Data stays on disk as markdown files + SQLite.

> **Keep this file current.** When you add, rename, or remove packages, change conventions, alter the build pipeline, or modify the schema, update the relevant sections of this file as part of the same change.

## Architecture

```
proto/blackwood/v1/*.proto   → buf generate → gen/blackwood/v1/   (DO NOT edit gen/)
internal/                    → Go backend packages
web/                         → React + TypeScript + Vite frontend
cmd/blackwood/               → Entry point, wires everything together
```

The API uses [Connect-RPC](https://connectrpc.com/) (gRPC-compatible, HTTP/JSON wire format). Proto definitions live in `proto/blackwood/v1/`. Generated Go code lands in `gen/blackwood/v1/` and `gen/blackwood/v1/blackwoodv1connect/`.

### Key packages

| Package | Role |
|---------|------|
| `internal/api` | Connect-RPC handlers + plain HTTP endpoints. One file per service/feature. |
| `internal/storage` | SQLite storage layer. Schema in `schema.sql`, embedded via `//go:embed`. Separate read/write connection pools (WAL mode). |
| `internal/config` | YAML config loading. Secrets read from files, env vars as fallback. |
| `internal/index` | Semantic index — `Indexer` interface with two backends: OpenAI embeddings (SQLite + cosine similarity) and QMD (local hybrid search via CLI). |
| `internal/rag` | RAG engine — retrieves context from index, streams OpenAI chat completions. |
| `internal/transcribe` | `Transcriber` interface + Whisper implementation. |
| `internal/describe` | `Describer` interface + GPT vision implementation. |
| `internal/ocr` | `Recognizer` interface + GPT vision OCR implementation. |
| `internal/telegram` | Telegram bot (long polling, no webhook needed). |
| `internal/whatsapp` | WhatsApp Business API webhook handler. |
| `internal/granola` | Granola meeting notes sync via MCP with OAuth. |
| `internal/watcher` | File system watcher for auto-importing Viwoods `.note` files. |
| `internal/importqueue` | Background import worker for async file processing. |
| `internal/noteparser` | Viwoods `.note` file parser. |
| `internal/opengraph` | Open Graph metadata fetcher for web clipping. |
| `internal/pdf` | PDF export renderer. |
| `internal/state` | File hashing for deduplication. |

### Web frontend

React 19 + TypeScript + Vite + Tailwind CSS v4. No component library — all components are hand-written.

| Directory | Contents |
|-----------|----------|
| `web/src/components/` | React components, one per file. |
| `web/src/api/client.ts` | API client — wraps Connect-RPC calls with offline fallback. |
| `web/src/api/types.ts` | TypeScript types mirroring proto messages (manually maintained). |
| `web/src/hooks/` | Custom React hooks. |
| `web/src/lib/` | Utilities (date helpers, offline store, sync engine). |

The Vite dev server proxies `/blackwood.v1` and `/api` to `localhost:8080`.

### Static embedding

For release builds, the web UI is embedded into the Go binary:
1. `web/dist/` is copied to `cmd/blackwood/static/`
2. `cmd/blackwood/static.go` uses `//go:embed all:static` to embed it
3. The server serves embedded files as fallback when `web/dist/` doesn't exist on disk

During development, the server serves from `web/dist/` on the filesystem if present.

## Build & Run

```sh
# Full build (web + Go)
cd web && npm ci && npm run build && cd ..
rm -rf cmd/blackwood/static && cp -r web/dist cmd/blackwood/static
go build -o blackwood ./cmd/blackwood

# Or use Makefile targets
make build          # Go binary only (no web rebuild)
make build-server   # Regenerate protos + build Go binary
make web-build      # Web UI only
make generate       # Regenerate proto code (requires buf)
make test           # go test ./...
```

The Ona automation task `build` runs the full pipeline on devcontainer start.

### Development server

The Blackwood server listens on port **8090** in the dev environment (configured in `.blackwood/config.yaml`). The devcontainer forwards this port.

To run the Vite dev server for frontend work:
```sh
cd web && npm run dev
```
Vite proxies API calls to `localhost:8080` by default. Adjust `vite.config.ts` if the Go server runs on a different port.

## Code Conventions

### Go

- **Logging**: `log/slog` with JSON handler. Use structured fields: `slog.Info("message", "key", value)`.
- **Error handling**: Wrap with `fmt.Errorf("context: %w", err)`. Return `connect.NewError(code, err)` from RPC handlers.
- **IDs**: Generated with `crypto/rand` hex encoding (see `storage.newID()`).
- **Interfaces**: Defined in the consumer package, not the provider. Small interfaces (1-2 methods): `Transcriber`, `Describer`, `Recognizer`, `EntryIndexer`.
- **Nil-safe optionals**: AI features (transcriber, indexer, describer) may be nil when no API key is configured. Always nil-check before use.
- **Index backends**: The `index.Indexer` interface abstracts over OpenAI embeddings and QMD. Consumers should accept `index.Indexer`, not `*index.Index`. QMD is enabled via config (`qmd.enabled: true`) and takes priority over OpenAI for indexing/search. RAG chat still requires an OpenAI API key for the LLM.
- **SQLite**: WAL mode, separate read/write pools. Write pool limited to 1 connection. Use `RetryOnBusy()` for write operations that may contend.
- **Context**: Thread `context.Context` through all operations. Background goroutines use the signal-notify context from `main()`.
- **No frameworks**: Standard library `net/http` with `http.ServeMux`. Connect-RPC handlers registered via generated `New*ServiceHandler()` functions.

### TypeScript / React

- **No component library** — all UI is custom with Tailwind CSS.
- **API types** in `web/src/api/types.ts` are manually maintained to match proto definitions. When adding proto fields, update both.
- **Offline support**: `offlineStore.ts` (IndexedDB via `idb`) + `syncEngine.ts`. API client functions in `client.ts` queue writes when offline.
- **Routing**: `react-router-dom` v7 with `BrowserRouter`. Routes: `/day/:date`, `/week/:weekId`, `/month/:monthId`, `/chat/:slug`, `/search`, `/clip`.
- **State**: Local component state with `useState`/`useCallback`/`useEffect`. No global state library.
- **Markdown**: `react-markdown` with `remark-gfm` and `rehype-raw`. Editing via CodeMirror.

### Proto / API

- Proto files: `proto/blackwood/v1/*.proto`
- Package: `blackwood.v1`, Go package `blackwoodv1`
- Code generation: `buf generate` (configured in `buf.gen.yaml`)
- Generated code: `gen/blackwood/v1/` — **never edit manually**
- Wire format: Connect-RPC JSON (camelCase field names)
- Streaming: Server-streaming for `Chat` RPC using Connect protocol envelopes

When adding a new RPC:
1. Define messages and service method in the appropriate `.proto` file
2. Run `make generate`
3. Implement the handler in `internal/api/`
4. Register in `cmd/blackwood/main.go`
5. Add client function in `web/src/api/client.ts`
6. Update types in `web/src/api/types.ts`

## Testing

```sh
make test           # Run all Go tests
cd web && npm run lint  # Lint frontend
```

### Patterns

- **Test helpers**: `newTestStore(t)` creates an in-memory SQLite store with `t.TempDir()` and `t.Cleanup()`.
- **Table-driven tests**: Used for parsing/validation (see `import_test.go`, `config/server_test.go`).
- **HTTP tests**: `httptest.NewRequest` + `httptest.NewRecorder` for webhook/handler tests.
- **Mock interfaces**: Small mock structs implementing the interface (e.g., `mockEmbeddingClient` in `index_test.go`).
- **No test framework**: Standard `testing` package only. No testify, no gomock.
- **Environment isolation**: Tests use `t.Setenv()` to override env vars, `t.TempDir()` for file system.

### CI

GitHub Actions (`.github/workflows/ci.yml`):
- Builds web UI, copies to `cmd/blackwood/static/`, builds Go binary, runs `make test`
- Runs `golangci-lint` separately

## Commit Conventions

Commits use a mix of styles. Recent commits tend toward imperative, descriptive summaries without conventional-commit prefixes. Older commits use `feat:`, `fix:`, `docs:`, `chore:` prefixes.

Follow the most recent pattern: **imperative summary describing the change**.

Examples from history:
- `Eliminate SQLITE_BUSY errors with WAL, write serialization, and retry`
- `Add user timezone and color theme preferences`
- `Fix collapsible list item alignment and nested h2 spacing`

## Adding a New Feature

### New integration (e.g., a new message source)

1. Create `internal/<name>/` with a small interface + implementation
2. Add config fields to `internal/config/server.go` (`ServerConfig` struct + `Resolve()`)
3. Wire up in `cmd/blackwood/main.go` — follow the Telegram/WhatsApp pattern: check config, create dependencies, start goroutine
4. Add tests in `internal/<name>/<name>_test.go`
5. Update `blackwood.example.yaml` with commented-out config block
6. Update `README.md` with setup instructions

### New API endpoint

**Connect-RPC (structured data)**:
1. Add proto messages + RPC to `proto/blackwood/v1/`
2. `make generate`
3. Implement handler in `internal/api/`
4. Register in `main.go`
5. Add frontend client + types

**Plain HTTP (simple endpoints)**:
1. Add handler function in `internal/api/` (see `clip.go`, `search.go`, `location.go`)
2. Register with `srv.Handle("METHOD /api/path", handler)` in `main.go`

### New frontend component

1. Create `web/src/components/ComponentName.tsx`
2. Add route in `web/src/App.tsx` if it's a page
3. Use Tailwind classes matching existing component styles
4. CSS variables for theming: `bg-background`, `text-foreground`, `bg-card`, `border-border`, `text-muted-foreground`

## Storage

Daily notes on disk: `<data_dir>/notes/YYYY/MM/DD/index.md`
Attachments alongside: `<data_dir>/notes/YYYY/MM/DD/<filename>`
SQLite database: `<data_dir>/blackwood.db`

Schema is in `internal/storage/schema.sql`. Migrations are applied via `CREATE TABLE IF NOT EXISTS` — additive only. When adding tables or columns, append to `schema.sql`.

## Release

GoReleaser (`.goreleaser.yaml`):
1. Builds web UI, copies to `cmd/blackwood/static/`
2. Cross-compiles for linux/darwin × amd64/arm64
3. Sets version via ldflags: `-X main.Version={{.Version}}`
4. Triggered by pushing a `v*` tag

## Common Pitfalls

- **Forgetting to rebuild static files**: The Go binary embeds `cmd/blackwood/static/`. After web changes, run `rm -rf cmd/blackwood/static && cp -r web/dist cmd/blackwood/static` before `go build`.
- **Proto/types drift**: `web/src/api/types.ts` is manually maintained. After proto changes, update it to match.
- **SQLite busy errors**: All writes go through the single-connection write pool. Use `RetryOnBusy()` when accessing the write DB from outside the storage package.
- **Nil AI services**: Transcriber, describer, indexer, and RAG engine are all nil when no OpenAI API key is configured. Guard with nil checks.
- **buf.gen.yaml paths**: Plugin paths in `buf.gen.yaml` reference the maintainer's local Go bin. `buf generate` requires `protoc-gen-go` and `protoc-gen-connect-go` in your PATH.
