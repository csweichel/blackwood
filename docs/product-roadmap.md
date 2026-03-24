# Blackwood — Product Roadmap

## What Blackwood is

A local-first daily notes app. One markdown document per day. Content flows in from the web UI, Telegram, WhatsApp, file watchers, and a web clipper. Everything is searchable through semantic embeddings and a RAG chat interface. Runs on your machine, no cloud dependency.

## What exists today

- Markdown daily notes with structured `# Notes` / `# Links` sections
- Voice memos (Whisper transcription), photos (vision LLM description), handwriting OCR (Viwoods)
- Telegram and WhatsApp integrations
- Open Graph link previews (Telegram + web clipper)
- RAG chat with source citations
- Offline-first web UI with service worker + IndexedDB sync
- Client-side routing with bookmarkable URLs
- PDF export, collapsible sections, keyboard shortcuts
- Optional HTTPS/TLS

## Proposed features

### 1. Daily digest / summary

**What:** At the end of each day (or on demand), generate a short summary of the day's notes using the LLM. Store it as a `# Summary` section at the top of the daily note.

**Why:** Scanning a full day of notes is slow. A 2-3 sentence summary makes it easy to recall what happened on any given day, especially when browsing the calendar weeks later.

**Implementation:**
- New `POST /api/daily-notes/{date}/summarize` endpoint
- Calls the LLM with the day's full content, asks for a concise summary
- Inserts/replaces the `# Summary` section at the top of the note
- Optional: auto-generate at midnight via a background goroutine
- Frontend: "Summarize" button in the daily note header

---

### 2. Weekly and monthly views

**What:** Aggregate views that show summaries across multiple days. A weekly view shows 7 days of summaries. A monthly view shows the calendar with summary snippets.

**Why:** The calendar currently shows dots for days with content. Summaries would make it useful for reviewing what happened over a period without clicking into each day.

**Implementation:**
- New routes: `/week/2025-W03`, `/month/2025-01`
- Backend: `ListDailyNotes` already supports date ranges — add a summary field to the response
- Frontend: new components that render a timeline of daily summaries
- Depends on daily digest (#1) for the summary content

---

### 3. Tags and backlinks

**What:** Support `#tag` syntax in notes. Clicking a tag shows all days that mention it. Support `[[2025-01-15]]` wikilinks to other daily notes.

**Why:** Daily notes accumulate topics over time. Tags let you trace a thread (e.g., `#project-x`) across days without relying on full-text search.

**Implementation:**
- Parse `#tag` tokens during note save, store in a `tags` table
- New endpoint: `GET /api/tags` (list all), `GET /api/tags/{tag}` (list dates)
- Frontend: tag cloud component, clickable tags in rendered markdown
- Wikilinks (`[[2025-01-15]]`) already partially supported via `remarkWikilinks` — wire them to router navigation

---

### 4. Templates

**What:** Configurable templates for new daily notes. Instead of starting with empty `# Notes` / `# Links`, users can define a structure like:

```markdown
# Morning
# Work
# Personal
# Links
```

**Why:** Different people organize their days differently. A fixed structure helps build a journaling habit.

**Implementation:**
- New config field: `server.note_template` (path to a markdown file)
- `GetOrCreateDailyNote` reads the template when creating a new note
- `AppendToSection` already handles arbitrary section names
- Setup wizard: "Choose a note template" step

---

### 5. Search page

**What:** A dedicated search view with full-text and semantic search. Type a query, see matching entries across all days with highlighted snippets and dates.

**Why:** The RAG chat is powerful but conversational. Sometimes you just want to find "that thing I wrote about X" without starting a chat.

**Implementation:**
- New route: `/search?q=...`
- Backend: the semantic index already exists (`internal/index/`). Add a `Search` RPC that returns ranked results with snippets.
- Frontend: search input in the header (always visible), results page with date-grouped entries
- Keyboard shortcut: `Cmd+K` to focus search

---

### 6. Image gallery per day

**What:** A grid view of all photos and images for a given day, viewable as a lightbox.

**Why:** Photos are currently inline in the markdown. When you take many photos in a day, scrolling through markdown to find one is tedious.

**Implementation:**
- Frontend component that queries entries with `type=PHOTO` for the current date
- Renders a thumbnail grid below the note or as a tab
- Click to open full-size in a lightbox overlay
- No backend changes needed — attachments are already served via `/api/attachments/{id}`

---

### 7. Email integration

**What:** A dedicated email address (or IMAP polling) that captures incoming emails as daily note entries.

**Why:** Email is still how many things arrive — receipts, confirmations, newsletters. Forwarding an email to Blackwood captures it alongside the day's other notes.

**Implementation:**
- New `internal/email/` package with IMAP polling
- Parse email body (prefer plain text, fall back to HTML→text)
- Attachments → stored as entry attachments
- Links in the email body → extracted and added to `# Links`
- Config: `email.imap_server`, `email.username`, `email.password_file`, `email.poll_interval`

---

### 8. Location tagging

**What:** Optionally tag entries with a location. Show a map view of where notes were created.

**Why:** Travel notes and daily logs are more meaningful with location context. "What did I do in Berlin?" becomes answerable.

**Implementation:**
- Add optional `latitude`/`longitude` fields to `Entry` metadata
- Telegram messages already include location data when shared — extract it
- Web UI: use the Geolocation API to tag entries created in the browser
- New route: `/map` with a Leaflet/Mapbox view showing entry pins grouped by day

---

### 9. Shared notes / collaboration

**What:** Share a daily note via a public read-only URL. Optionally allow another Blackwood user to contribute to a shared day.

**Why:** Couples, families, or small teams might want a shared daily log (e.g., a travel journal).

**Implementation:**
- New `shares` table: `note_id`, `token`, `permissions` (read/write), `expires_at`
- `GET /shared/{token}` serves a read-only rendered view (no auth required)
- Write access: the shared token allows `CreateEntry` and `UpdateDailyNoteContent` scoped to that note
- Frontend: "Share" button in the note header, generates a link

---

### 10. Native apps (iOS / macOS)

**What:** Native apps that talk to the Blackwood server API. Quick capture from the share sheet, notifications, widgets.

**Why:** The web UI works on mobile but lacks native integration — share sheet, widgets, notifications, Siri shortcuts.

**Implementation:**
- Swift app using the existing Connect-RPC API
- iOS share extension for quick capture (text, URLs, photos)
- macOS menu bar app for quick text entry
- Push notifications when a daily digest is ready
- The API is already designed for this — all operations go through Connect-RPC

---

## Priority suggestion

| Priority | Feature | Effort | Impact |
|----------|---------|--------|--------|
| P0 | Search page (#5) | Medium | High — most requested missing feature |
| P0 | Daily digest (#1) | Low | High — makes the calendar useful |
| P1 | Tags and backlinks (#3) | Medium | High — connects notes across days |
| P1 | Weekly/monthly views (#2) | Medium | Medium — depends on digests |
| P1 | Templates (#4) | Low | Medium — personalization |
| P2 | Image gallery (#6) | Low | Medium — quality of life |
| P2 | Email integration (#7) | Medium | Medium — new input channel |
| P3 | Location tagging (#8) | Medium | Low-Medium — niche but compelling |
| P3 | Shared notes (#9) | High | Low — most users are single-user |
| P3 | Native apps (#10) | High | High — but web UI covers most cases |
