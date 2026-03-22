# Blackwood

**Your daily notes, from everywhere.** Blackwood is a local-first daily notes app that captures text, voice memos, photos, and handwritten notes — then makes all of it searchable through AI.

Write from the web, send a WhatsApp message, import from Obsidian, or drop in a Viwoods handwritten note. Everything lands in a single markdown document per day. A semantic index built on top lets you chat with your notes using RAG.

Blackwood runs entirely on your machine. Your data stays local.

## Features

- **Markdown daily notes** — one document per day, editable in the browser with auto-save
- **Voice memos** — record audio in the web UI or send via WhatsApp; automatically transcribed via Whisper
- **Photo capture** — upload or snap photos; automatically described via GPT-4o vision
- **Viwoods handwriting** — import `.note` files from Viwoods AIPaper; pages are OCR'd and added to your daily note
- **Obsidian import** — bulk import your existing daily notes from Obsidian
- **WhatsApp integration** — text, voice messages, and photos sent to your bot appear in today's note
- **Semantic search & RAG chat** — ask questions about your notes in natural language; get answers with source citations
- **Calendar view** — monthly grid showing which days have content; click to navigate
- **Local-first** — SQLite database, runs on your machine, no cloud dependency

## Quick Start

### Prerequisites

- Go 1.25+
- Node.js 18+ (for the web UI)
- An [OpenAI API key](https://platform.openai.com/api-keys) (for transcription, vision, embeddings, and chat)

### Build

```sh
# Build the server
go build -o bin/blackwood-server ./cmd/blackwood-server

# Build the web UI
cd web && npm ci && npm run build && cd ..
```

### Run

```sh
export OPENAI_API_KEY=sk-...
./bin/blackwood-server --addr :8080 --data-dir ~/.blackwood
```

Open [http://localhost:8080](http://localhost:8080) in your browser.

### Makefile targets

```sh
make build          # Build the file watcher daemon
make build-server   # Build the API server (requires buf)
make test           # Run all tests
make web-build      # Build the web UI
make generate       # Regenerate protobuf/Connect code
```

## Architecture

Blackwood is a Go backend serving a React frontend over a single port.

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
│  Whisper (audio) · GPT-4o (vision/chat/OCR)     │
│  text-embedding-3-small (semantic index)        │
├─────────────────────────────────────────────────┤
│  Storage                                        │
│  SQLite (notes, entries, conversations)         │
│  Embeddings (cosine similarity in Go)           │
│  File system (attachments)                      │
└─────────────────────────────────────────────────┘
```

### Key packages

| Package | Purpose |
|---------|---------|
| `cmd/blackwood-server` | API server entry point |
| `cmd/blackwood` | Legacy file watcher daemon |
| `internal/storage` | SQLite storage layer |
| `internal/api` | Connect-RPC service handlers |
| `internal/rag` | RAG engine (search + LLM) |
| `internal/index` | Semantic index (embeddings + vector search) |
| `internal/transcribe` | Whisper audio transcription |
| `internal/describe` | GPT-4o photo description |
| `internal/ocr` | GPT-4o handwriting OCR |
| `internal/noteparser` | Viwoods `.note` file parser |
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

The server is configured via environment variables:

| Variable | Required | Description |
|----------|----------|-------------|
| `OPENAI_API_KEY` | Yes | OpenAI API key for all AI features |
| `OPENAI_MODEL` | No | Model for OCR (default: `gpt-4o`) |
| `OPENAI_CHAT_MODEL` | No | Model for RAG chat (default: `gpt-4o`) |

### WhatsApp (optional)

To receive messages via WhatsApp, set up a [WhatsApp Business App](https://developers.facebook.com/docs/whatsapp/cloud-api/get-started) and configure:

| Variable | Description |
|----------|-------------|
| `WHATSAPP_VERIFY_TOKEN` | Webhook verification token (you choose this) |
| `WHATSAPP_APP_SECRET` | App secret for signature verification |
| `WHATSAPP_ACCESS_TOKEN` | Permanent access token |
| `WHATSAPP_PHONE_NUMBER_ID` | Phone number ID for sending replies |

Set your webhook URL to `https://your-domain/api/webhooks/whatsapp`.

## Legacy: File Watcher Daemon

The original `blackwood` binary watches a directory for Viwoods `.note` files and processes them via configurable hooks. See `blackwood.example.yaml` for configuration. This mode is independent of the API server.

```sh
make build
./bin/blackwood --config blackwood.example.yaml
```

## Roadmap

- [ ] Telegram bot integration
- [ ] Chrome extension (web clipper)
- [ ] iOS app
- [ ] Mac app (menu bar quick capture)
- [ ] Offline sync (service worker + IndexedDB)

## License

[MIT](LICENSE)
