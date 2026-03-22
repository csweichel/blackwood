# Blackwood

**Your daily notes, from everywhere.** Blackwood is a local-first daily notes app that captures text, voice memos, photos, and handwritten notes — then makes all of it searchable through AI.

Write from the web, send a WhatsApp message, import from Obsidian, or drop in a Viwoods handwritten note. Everything lands in a single markdown document per day. A semantic index built on top lets you chat with your notes using RAG.

Blackwood runs entirely on your machine. Your data stays local.

## Features

- **Markdown daily notes** — one document per day (`notes/YYYY/MM/DD/index.md`), editable in the browser with auto-save
- **Voice memos** — record audio in the web UI or send via WhatsApp; transcribed via Whisper and kept as playable audio files
- **Photo capture** — upload or snap photos; described via gpt-5.2 vision and rendered inline in the daily note
- **Viwoods handwriting** — import `.note` files from Viwoods AIPaper; pages are OCR'd and added to your daily note
- **Obsidian import** — bulk import your existing daily notes from Obsidian
- **WhatsApp integration** — text, voice messages, and photos sent to your bot appear in today's note
- **Semantic search & RAG chat** — ask questions about your notes in natural language; get answers with source citations
- **Calendar view** — monthly grid showing which days have content; click to navigate
- **Rendered markdown view** — rendered display with edit toggle for switching between reading and editing
- **File watcher** — optionally watches a directory for new Viwoods `.note` files and auto-imports them
- **Local-first** — runs on your machine, no cloud dependency

## Quick Start

### Prerequisites

- Go 1.25+
- Node.js 18+ (for the web UI)
- An [OpenAI API key](https://platform.openai.com/api-keys) (for transcription, vision, embeddings, and chat)

### Build

```sh
# Build the single binary
go build -o blackwood ./cmd/blackwood

# Build the web UI
cd web && npm ci && npm run build && cd ..
```

### Configure

Copy the example config and add your API key:

```sh
cp blackwood.example.yaml ~/.blackwood/config.yaml
mkdir -p ~/.blackwood/secrets
echo "sk-..." > ~/.blackwood/secrets/openai-api-key
```

See [`blackwood.example.yaml`](blackwood.example.yaml) for all options.

### Run

```sh
./blackwood --config ~/.blackwood/config.yaml
```

Open [http://localhost:8080](http://localhost:8080) in your browser.

CLI flags `--addr` and `--data-dir` override the corresponding config file values. Environment variables (`OPENAI_API_KEY`, etc.) work as a fallback when no config file is provided:

```sh
export OPENAI_API_KEY=sk-...
./blackwood --addr :8080 --data-dir ~/.blackwood
```

### Makefile targets

```sh
make build          # Build the blackwood binary
make build-server   # Build with protobuf regeneration (requires buf)
make test           # Run all tests
make web-build      # Build the web UI
make generate       # Regenerate protobuf/Connect code
```

## Architecture

Blackwood is a single Go binary (`blackwood`) serving a React frontend over a single port.

```
┌─────────────────────────────────────────────────┐
│                  Web UI (React)                 │
│  Calendar · Markdown Editor · Audio · Chat      │
└──────────────────────┬──────────────────────────┘
                       │ Connect-RPC
┌──────────────────────┴──────────────────────────┐
│               Go API Server                     │
│                                                 │
│  DailyNotesService · ChatService · ImportService│
│  WhatsApp Webhook · Attachment Serving          │
├─────────────────────────────────────────────────┤
│  AI Pipelines                                   │
│  Whisper (audio) · gpt-5.2 (vision/chat/OCR)   │
│  text-embedding-3-small (semantic index)        │
├─────────────────────────────────────────────────┤
│  Storage                                        │
│  Markdown files  notes/YYYY/MM/DD/index.md      │
│  Attachments     notes/YYYY/MM/DD/<file>        │
│  SQLite (entries, conversations, embeddings)    │
└─────────────────────────────────────────────────┘
```

### Key packages

| Package | Purpose |
|---------|---------|
| `cmd/blackwood` | Entry point — API server + optional file watcher |
| `internal/config` | YAML config loading with secret file and env var resolution |
| `internal/storage` | SQLite + filesystem storage (daily notes as markdown, attachments on disk) |
| `internal/api` | Connect-RPC service handlers |
| `internal/rag` | RAG engine (search + LLM) |
| `internal/index` | Semantic index (embeddings + vector search) |
| `internal/transcribe` | Whisper audio transcription |
| `internal/describe` | gpt-5.2 photo description |
| `internal/ocr` | gpt-5.2 handwriting OCR |
| `internal/noteparser` | Viwoods `.note` file parser |
| `internal/watcher` | Viwoods file watcher (polls a directory for new `.note` files) |
| `internal/whatsapp` | WhatsApp Business API webhook |
| `web/` | React + TypeScript + Vite frontend |

### API

The API uses [Connect-RPC](https://connectrpc.com/) (gRPC-compatible over HTTP/JSON). Proto definitions are in `proto/blackwood/v1/`.

| Service | RPCs |
|---------|------|
| `DailyNotesService` | `GetDailyNote`, `ListDailyNotes`, `CreateEntry`, `UpdateEntry`, `DeleteEntry`, `UpdateDailyNoteContent`, `ListDatesWithContent` |
| `ChatService` | `Chat` (streaming), `ListConversations`, `GetConversation` |
| `ImportService` | `ImportViwoods`, `ImportObsidian` |
| `HealthService` | `Check` |

## Configuration

Blackwood is configured via a YAML config file passed with `--config`. Secrets are stored in separate files referenced by path. Environment variables are used as fallback when no config file is provided.

Priority: **config file > environment variable > default**.

### Config file

```yaml
server:
  addr: ":8080"
  data_dir: ~/.blackwood

openai:
  api_key_file: ~/.blackwood/secrets/openai-api-key
  model: gpt-5.2
  chat_model: gpt-5.2
  embedding_model: text-embedding-3-small

# WhatsApp integration (optional)
# whatsapp:
#   verify_token: your-verify-token
#   app_secret_file: ~/.blackwood/secrets/whatsapp-app-secret
#   access_token_file: ~/.blackwood/secrets/whatsapp-access-token
#   phone_number_id: "123456789"

# Viwoods file watcher (optional)
# watcher:
#   watch_dir: /path/to/viwoods/notes
#   poll_interval: 30s
```

See [`blackwood.example.yaml`](blackwood.example.yaml) for the full reference.

### Environment variable fallback

When no config file is used, the following environment variables are recognized:

| Variable | Default | Description |
|----------|---------|-------------|
| `OPENAI_API_KEY` | — | OpenAI API key for all AI features |
| `OPENAI_MODEL` | `gpt-5.2` | Model for vision and OCR |
| `OPENAI_CHAT_MODEL` | same as `OPENAI_MODEL` | Model for RAG chat |
| `WHATSAPP_VERIFY_TOKEN` | — | Webhook verification token |
| `WHATSAPP_APP_SECRET` | — | App secret for signature verification |
| `WHATSAPP_ACCESS_TOKEN` | — | Permanent access token |
| `WHATSAPP_PHONE_NUMBER_ID` | — | Phone number ID for sending replies |

### WhatsApp (optional)

To receive messages via WhatsApp, set up a [WhatsApp Business App](https://developers.facebook.com/docs/whatsapp/cloud-api/get-started) and configure the `whatsapp` section in your config file (or set the corresponding environment variables).

Set your webhook URL to `https://your-domain/api/webhooks/whatsapp`.

## Storage

Daily notes are stored as markdown files on disk:

```
~/.blackwood/
├── blackwood.db              # SQLite: entries, conversations, embeddings index
├── notes/
│   └── 2025/
│       └── 01/
│           └── 15/
│               ├── index.md          # The daily note markdown
│               ├── voice-memo-a1b2.webm
│               └── photo-c3d4.jpg
└── secrets/
    └── openai-api-key
```

Attachments (photos, audio recordings) are stored alongside the daily note in the same per-day folder and embedded in the rendered markdown.

## Roadmap

- [ ] Telegram bot integration
- [ ] Chrome extension (web clipper)
- [ ] iOS app
- [ ] Mac app (menu bar quick capture)
- [ ] Offline sync (service worker + IndexedDB)

## License

[MIT](LICENSE)
