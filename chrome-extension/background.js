const DEFAULT_SERVER = "http://localhost:8080";

// Create context menu on install
chrome.runtime.onInstalled.addListener(() => {
  chrome.contextMenus.create({
    id: "blackwood-clip",
    title: "Clip to Blackwood",
    contexts: ["page", "link"],
  });
});

async function getServerUrl() {
  const result = await chrome.storage.sync.get("serverUrl");
  return result.serverUrl || DEFAULT_SERVER;
}

async function getToken() {
  const result = await chrome.storage.sync.get("authToken");
  return result.authToken || "";
}

async function clipUrl(url, tab) {
  const serverUrl = await getServerUrl();
  const token = await getToken();

  const headers = { "Content-Type": "application/json" };
  if (token) {
    headers["Authorization"] = `Bearer ${token}`;
  }

  try {
    const resp = await fetch(`${serverUrl}/api/clip`, {
      method: "POST",
      headers,
      body: JSON.stringify({ url }),
    });

    if (resp.status === 401) {
      // Token expired or missing — show lock badge
      chrome.action.setBadgeText({ text: "🔒", tabId: tab.id });
      chrome.action.setBadgeBackgroundColor({ color: "#f39c12", tabId: tab.id });
      setTimeout(() => {
        chrome.action.setBadgeText({ text: "", tabId: tab.id });
      }, 3000);
      return;
    }

    if (!resp.ok) {
      const text = await resp.text();
      throw new Error(text || `Server error (${resp.status})`);
    }

    const data = await resp.json();
    chrome.action.setBadgeText({ text: "✓", tabId: tab.id });
    chrome.action.setBadgeBackgroundColor({ color: "#27ae60", tabId: tab.id });
    setTimeout(() => {
      chrome.action.setBadgeText({ text: "", tabId: tab.id });
    }, 2000);
  } catch (err) {
    chrome.action.setBadgeText({ text: "!", tabId: tab.id });
    chrome.action.setBadgeBackgroundColor({ color: "#c0392b", tabId: tab.id });
    setTimeout(() => {
      chrome.action.setBadgeText({ text: "", tabId: tab.id });
    }, 3000);
  }
}

// Context menu handler
chrome.contextMenus.onClicked.addListener((info, tab) => {
  if (info.menuItemId !== "blackwood-clip") return;

  // If right-clicked on a link, clip the link URL; otherwise clip the page URL
  const url = info.linkUrl || info.pageUrl;
  if (url) {
    clipUrl(url, tab);
  }
});

// Keyboard shortcut handler
chrome.commands.onCommand.addListener(async (command) => {
  if (command !== "clip-page") return;

  const [tab] = await chrome.tabs.query({ active: true, currentWindow: true });
  if (tab?.url && (tab.url.startsWith("http://") || tab.url.startsWith("https://"))) {
    clipUrl(tab.url, tab);
  }
});
