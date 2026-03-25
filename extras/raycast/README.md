# Blackwood Raycast Extension

Interact with your [Blackwood](https://github.com/csweichel/blackwood) daily notes from Raycast.

## Commands

| Command | Description |
|---------|-------------|
| **Log Note** | Add a text note to today's daily note |
| **Clip URL** | Clip a URL from the clipboard (fetches OpenGraph metadata) |
| **Ask Question** | Ask a question against your notes using RAG |
| **Ingest Image** | Add an image file to today's daily note |
| **Voice Record** | Record a voice memo, transcribe it, and add to today's note |

## Setup

1. Install dependencies:
   ```bash
   cd extras/raycast && npm install
   ```

2. Open Raycast and configure the **Server URL** preference to point at your Blackwood instance (default: `http://localhost:8080`).

3. For voice recording, install `sox`:
   ```bash
   brew install sox
   ```

## Development

```bash
npm run dev    # Start Raycast development mode
npm run build  # Build the extension
npm run lint   # Run linter
```

## Icon

Place a 512×512 PNG icon at `assets/icon.png`. The extension ships without one by default.
