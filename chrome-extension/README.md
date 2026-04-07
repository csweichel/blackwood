# Blackwood Web Clipper

Chrome extension to clip web pages into Blackwood daily notes.

## Install

1. Open `chrome://extensions/`
2. Enable **Developer mode** (top right)
3. Click **Load unpacked** and select this `chrome-extension/` directory

## Configure

Click the extension icon → gear icon → set your Blackwood server URL (e.g. `http://localhost:8080`).

The URL is stored in Chrome sync storage and follows your Chrome profile across devices.

## Authentication

If your Blackwood instance has TOTP authentication enabled, the extension will prompt you to log in when you first try to clip. Enter your 6-digit authenticator code in the login view. The session token is stored in Chrome sync storage and lasts 30 days.

You can log out from the settings view (gear icon → Logout).

If a clip fails with a 401, the extension automatically clears the stale token and shows the login view.

## Usage

Three ways to clip:

- **Popup**: Click the extension icon → **Clip this page**
- **Context menu**: Right-click on a page or link → **Clip to Blackwood**
- **Keyboard shortcut**: `Alt+Shift+B` (configurable in `chrome://extensions/shortcuts`)

Clips are appended to the **Links** section of today's daily note. The server fetches Open Graph metadata (title, description, image) for the clipped URL.
