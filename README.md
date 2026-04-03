# Blackwood

**Your daily notes, from everywhere.** Blackwood is a local-first daily notes app that captures text, voice memos, photos, and handwritten notes — then makes all of it searchable through AI.

Write from the web, send a WhatsApp message, import from Obsidian, or drop in a Viwoods handwritten note. Everything lands in a single markdown document per day. A semantic index built on top lets you chat with your notes using RAG.

Blackwood runs entirely on your machine. Your data stays local.

## Features

- **Markdown daily notes** — one document per day (`notes/YYYY/MM/DD/index.md`), editable in the browser with auto-save
- **Voice memos** — record audio in the web UI or send via WhatsApp; transcribed via Whisper and kept as playable audio files
- **Photo capture** — upload or snap photos; described via gpt-5.2 vision and rendered inline in the daily note
- **Semantic search** — find anything across your notes with AI-powered semantic search (`Cmd+K`)
- **RAG chat** — ask questions about your notes in natural language; get answers with source citations
- **Weekly & monthly views** — see notes aggregated by week or month with AI-generated range summaries
- **Daily digest** — automatic nightly summaries; on-demand summarize for any note
- **Location tagging** — tag daily notes with your location; reverse-geocoded to show address names
- **Web clipping** — clip any web page into today's note via bookmarklet; fetches Open Graph metadata
- **iOS app** — native iOS client for capturing notes, voice memos, and photos on the go
- **Desktop app** — Electron wrapper for macOS; runs Blackwood as a native desktop application
- **Raycast extension** — quick capture daily notes from Raycast
- **Telegram bot** — send text, voice, and photos from Telegram; uses long polling so no public URL is needed
- **WhatsApp integration** — text, voice messages, and photos sent to your bot appear in today's note
- **Granola meeting notes** — automatically imports meeting notes from [Granola](https://granola.ai) via MCP every hour, including summaries, attendees, and transcripts
- **Viwoods handwriting** — import `.note` files from Viwoods AIPaper; pages are OCR'd and added to your daily note
- **Obsidian import** — bulk import your existing daily notes from Obsidian
- **TOTP authentication** — authenticator-based login to protect your notes
- **Themes & preferences** — dark, light, or system theme; timezone-aware dates; configurable per user
- **Calendar view** — monthly grid showing which days have content; click to navigate
- **Collapsible sections** — headings and nested list items are collapsible (expanded by default) for easier scanning of long notes
- **PDF export** — download any daily note as a PDF from the note header
- **Offline support** — service worker caches the app shell; entries created offline are queued and synced when the server is reachable
- **Bookmarkable URLs** — `/day/2025-01-15` for daily notes, `/chat/2025-01-15-my-question` for conversations; browser back/forward works
- **Keyboard shortcuts** — `Cmd+D` jump to today, `Cmd+/` toggle chat, `Cmd+K` search, `Cmd+T` insert timestamp, `Cmd+Enter` save edit
- **HTTPS/TLS** — optional TLS with configurable cert/key paths; plain HTTP remains the default
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

The easiest way to get started is the interactive setup command:

```sh
./blackwood setup
```

This walks you through creating directories, storing your API key, and generating a config file.

Alternatively, configure manually:

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
│              Clients                            │
│  Web UI (React) · Electron · iOS · Raycast      │
│  Calendar · Editor · Search · Chat · Week/Month │
└──────────────────────┬──────────────────────────┘
                       │ Connect-RPC
┌──────────────────────┴──────────────────────────┐
│               Go API Server                     │
│                                                 │
│  DailyNotesService · ChatService · ImportService│
│  SearchService · DigestService · AuthService    │
│  WhatsApp · Telegram · Granola Sync · Clipper   │
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
| `internal/telegram` | Telegram bot (long polling) |
| `internal/granola` | Granola meeting notes sync (periodic polling) |
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
  # tls:
  #   cert_file: /path/to/cert.pem
  #   key_file: /path/to/key.pem

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

# Telegram bot (optional)
# telegram:
#   bot_token_file: ~/.blackwood/secrets/telegram-bot-token
#   allowed_chat_ids:
#     - 123456789

# Granola meeting notes via MCP (optional)
# granola:
#   oauth_token_file: ~/.blackwood/secrets/granola-oauth-token
#   poll_interval: 1h

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
| `TELEGRAM_BOT_TOKEN` | — | Telegram bot token from @BotFather |
| `GRANOLA_OAUTH_TOKEN` | — | Granola OAuth token for MCP meeting notes sync |

### WhatsApp (optional)

To receive messages via WhatsApp, set up a [WhatsApp Business App](https://developers.facebook.com/docs/whatsapp/cloud-api/get-started) and configure the `whatsapp` section in your config file (or set the corresponding environment variables).

Set your webhook URL to `https://your-domain/api/webhooks/whatsapp`.

### Telegram (optional)

Send text, voice messages, and photos to a Telegram bot and have them appear in your daily notes. Unlike WhatsApp, the Telegram integration uses long polling — no public URL or webhook setup is needed.

#### 1. Create a bot with @BotFather

1. Open Telegram and search for [@BotFather](https://t.me/botfather).
2. Send `/newbot` and follow the prompts to choose a name and username.
3. BotFather will reply with a **bot token** like `123456:ABC-DEF1234ghIkl-zyx57W2v1u123ew11`. Copy it.

#### 2. Store the token

```sh
echo -n "123456:ABC-DEF1234ghIkl-zyx57W2v1u123ew11" > ~/.blackwood/secrets/telegram-bot-token
chmod 600 ~/.blackwood/secrets/telegram-bot-token
```

#### 3. Configure Blackwood

Add to your config file:

```yaml
telegram:
  bot_token_file: ~/.blackwood/secrets/telegram-bot-token
```

Or use an environment variable instead:

```sh
export TELEGRAM_BOT_TOKEN=123456:ABC-DEF1234ghIkl-zyx57W2v1u123ew11
```

#### 4. Authorize your chat

Start Blackwood. The server logs will show a 6-digit authorization code:

```
telegram: bot started — send this code to the bot to authorize a chat  auth_code=482910
```

Open your bot in Telegram and send that code as a message. The bot will reply with "✓ Authorized!" and your chat is now connected. The authorization is persisted in the database — you only need to do this once.

The code rotates after each use, so you can authorize multiple devices or group chats by checking the logs each time.

To disconnect a chat, send `/revoke` to the bot.

You can also pre-authorize chat IDs in the config file (these can't be revoked via `/revoke`):

```yaml
telegram:
  bot_token_file: ~/.blackwood/secrets/telegram-bot-token
  allowed_chat_ids:
    - 123456789
```

#### What the bot does

| You send | Blackwood does |
|----------|---------------|
| Text message | Adds it as a text entry in today's note |
| Voice message | Transcribes via Whisper, adds transcription as entry |
| Photo | Describes via gpt-5.2 vision, adds description as entry |

All messages are appended to the daily note with a timestamp and "Telegram" source label. Attachments (audio files, photos) are stored alongside the note.

### Granola (optional)

Automatically import meeting notes from [Granola](https://granola.ai) into your daily notes via the [Granola MCP server](https://www.granola.ai/blog/granola-mcp). The sync runs periodically (default: every hour), fetching new or updated meeting notes and writing them as entries on the day the meeting occurred.

Each imported note includes the meeting title, date, attendees, Granola's AI-enhanced notes, private notes, and transcript (paid Granola tiers).

#### 1. Log in

```sh
blackwood granola-login
```

This opens your browser for OAuth authentication with Granola and saves the token to `~/.blackwood/secrets/granola-oauth-token`.

#### 2. Configure Blackwood

Add to your config file:

```yaml
granola:
  oauth_token_file: ~/.blackwood/secrets/granola-oauth-token
  poll_interval: 1h  # optional, default is 1h
```

Or use an environment variable:

```sh
export GRANOLA_OAUTH_TOKEN=your-token
```

Granola sync auto-enables when an OAuth token is configured.

### Web Clipping (bookmarklet)

Clip any web page into today's daily note. The clipper fetches Open Graph metadata (title, description, preview image) and appends a formatted blockquote to the note.

#### Bookmarklet

Drag this to your bookmarks bar (replace the URL with your Blackwood instance):

```
javascript:void(window.open('https://your-blackwood/clip#'+encodeURIComponent(location.href)))
```

When clicked on any page, it opens Blackwood's `/clip` route which calls `POST /api/clip`, saves the card, and redirects to today's note.

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

## Keyboard shortcuts

| Shortcut | Action |
|----------|--------|
| `Cmd+D` | Jump to today's note |
| `Cmd+/` | Toggle between notes and chat |
| `Cmd+K` | Open search |
| `Cmd+T` | Insert current time (in edit mode) |
| `Cmd+Enter` | Save and exit edit mode |
| `Esc` | Exit edit mode without saving |

On Windows/Linux, use `Ctrl` instead of `Cmd`.

## Roadmap

- [x] Telegram bot integration
- [x] Offline sync (service worker + IndexedDB)
- [x] PDF export
- [x] HTTPS/TLS support
- [x] Client-side routing with bookmarkable URLs
- [x] Collapsible sections
- [x] Keyboard shortcuts
- [x] Web clipper (bookmarklet)
- [x] Semantic search
- [x] Weekly & monthly views with range summaries
- [x] Daily digest (nightly generation)
- [x] Location tagging with reverse geocoding
- [x] TOTP authentication
- [x] User preferences (timezone, color theme)
- [x] Granola meeting notes via MCP
- [x] Raycast extension
- [x] iOS app
- [x] Mac app (Electron desktop wrapper)

## License

[MIT](LICENSE)
