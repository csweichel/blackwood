const DEFAULT_SERVER = "http://localhost:8080";

// --- DOM refs ---
const clipView = document.getElementById("clipView");
const loginView = document.getElementById("loginView");
const settingsView = document.getElementById("settingsView");

const settingsToggle = document.getElementById("settingsToggle");
const settingsBack = document.getElementById("settingsBack");
const loginBack = document.getElementById("loginBack");

const clipBtn = document.getElementById("clipBtn");
const clipStatus = document.getElementById("clipStatus");
const pageTitle = document.getElementById("pageTitle");
const pageUrl = document.getElementById("pageUrl");
const favicon = document.getElementById("favicon");

const totpCode = document.getElementById("totpCode");
const loginBtn = document.getElementById("loginBtn");
const loginStatus = document.getElementById("loginStatus");

const serverUrlInput = document.getElementById("serverUrl");
const saveBtn = document.getElementById("saveBtn");
const settingsStatus = document.getElementById("settingsStatus");
const authStatusEl = document.getElementById("authStatus");
const logoutBtn = document.getElementById("logoutBtn");

let currentTab = null;

// --- View management ---

function showView(view) {
  clipView.classList.remove("active");
  loginView.classList.remove("active");
  settingsView.classList.remove("active");
  view.classList.add("active");
}

settingsToggle.addEventListener("click", async () => {
  await refreshAuthStatus();
  showView(settingsView);
});
settingsBack.addEventListener("click", () => showView(clipView));
loginBack.addEventListener("click", () => showView(clipView));

// --- Storage helpers ---

async function getServerUrl() {
  const result = await chrome.storage.sync.get("serverUrl");
  return result.serverUrl || DEFAULT_SERVER;
}

async function getToken() {
  const result = await chrome.storage.sync.get("authToken");
  return result.authToken || "";
}

async function setToken(token) {
  await chrome.storage.sync.set({ authToken: token });
}

async function clearToken() {
  await chrome.storage.sync.remove("authToken");
}

// --- Auth helpers ---

function authHeaders(token) {
  const headers = { "Content-Type": "application/json" };
  if (token) {
    headers["Authorization"] = `Bearer ${token}`;
  }
  return headers;
}

async function refreshAuthStatus() {
  const token = await getToken();
  if (token) {
    authStatusEl.innerHTML = '<span class="auth-badge ok">● Authenticated</span>';
    logoutBtn.style.display = "block";
  } else {
    authStatusEl.innerHTML = '<span class="auth-badge none">○ Not logged in</span>';
    logoutBtn.style.display = "none";
  }
}

// --- Login ---

totpCode.addEventListener("input", () => {
  totpCode.value = totpCode.value.replace(/\D/g, "").slice(0, 6);
  loginBtn.disabled = totpCode.value.length !== 6;
});

loginBtn.addEventListener("click", async () => {
  loginBtn.disabled = true;
  loginBtn.textContent = "Logging in…";
  loginStatus.textContent = "";
  loginStatus.className = "status";

  const serverUrl = await getServerUrl();
  const code = totpCode.value;

  try {
    const resp = await fetch(`${serverUrl}/api/auth/token`, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ code }),
    });

    const ct = resp.headers.get("Content-Type") || "";
    if (!ct.includes("application/json")) {
      throw new Error("Server returned non-JSON response. Is the server up to date?");
    }

    const data = await resp.json();

    if (!data.ok) {
      throw new Error(data.error || "Login failed");
    }

    await setToken(data.token);
    loginStatus.textContent = "✅ Authenticated";
    loginStatus.className = "status success";
    totpCode.value = "";

    setTimeout(() => showView(clipView), 600);
  } catch (err) {
    loginStatus.textContent = err.message;
    loginStatus.className = "status error";
    loginBtn.disabled = false;
  }

  loginBtn.textContent = "Login";
});

// --- Logout ---

logoutBtn.addEventListener("click", async () => {
  await clearToken();
  await refreshAuthStatus();
  settingsStatus.textContent = "Logged out.";
  settingsStatus.className = "status success";
  setTimeout(() => { settingsStatus.textContent = ""; }, 1500);
});

// --- Settings ---

saveBtn.addEventListener("click", async () => {
  let url = serverUrlInput.value.trim().replace(/\/+$/, "");

  if (!url) {
    settingsStatus.textContent = "URL cannot be empty.";
    settingsStatus.className = "status error";
    return;
  }

  try {
    new URL(url);
  } catch {
    settingsStatus.textContent = "Invalid URL format.";
    settingsStatus.className = "status error";
    return;
  }

  await chrome.storage.sync.set({ serverUrl: url });
  settingsStatus.textContent = "Saved.";
  settingsStatus.className = "status success";
  setTimeout(() => {
    settingsStatus.textContent = "";
    showView(clipView);
  }, 800);
});

// --- Clip ---

async function clipPage() {
  if (!currentTab?.url) return;

  clipBtn.disabled = true;
  clipBtn.textContent = "Clipping…";
  clipStatus.textContent = "";
  clipStatus.className = "status";

  const serverUrl = await getServerUrl();
  const token = await getToken();

  try {
    const resp = await fetch(`${serverUrl}/api/clip`, {
      method: "POST",
      headers: authHeaders(token),
      body: JSON.stringify({ url: currentTab.url }),
    });

    if (resp.status === 401) {
      clipStatus.textContent = "Authentication required.";
      clipStatus.className = "status error";
      clipBtn.textContent = "Clip this page";
      clipBtn.disabled = false;
      await clearToken();
      setTimeout(() => showView(loginView), 800);
      return;
    }

    if (!resp.ok) {
      const text = await resp.text();
      throw new Error(text || `Server error (${resp.status})`);
    }

    const ct = resp.headers.get("Content-Type") || "";
    if (!ct.includes("application/json")) {
      throw new Error("Server returned non-JSON response. Is the server up to date?");
    }

    const data = await resp.json();
    clipStatus.textContent = `✅ Clipped to ${data.date}`;
    clipStatus.className = "status success";
    clipBtn.textContent = "Clipped!";
  } catch (err) {
    clipStatus.textContent = err.message;
    clipStatus.className = "status error";
    clipBtn.textContent = "Clip this page";
    clipBtn.disabled = false;
  }
}

clipBtn.addEventListener("click", clipPage);

// --- Init ---

async function init() {
  const savedUrl = await getServerUrl();
  serverUrlInput.value = savedUrl;

  const [tab] = await chrome.tabs.query({ active: true, currentWindow: true });
  if (!tab) {
    pageTitle.textContent = "No active tab";
    return;
  }

  currentTab = tab;
  pageTitle.textContent = tab.title || "Untitled";
  pageUrl.textContent = tab.url || "";

  if (tab.favIconUrl) {
    favicon.src = tab.favIconUrl;
    favicon.style.display = "block";
  } else {
    favicon.style.display = "none";
  }

  if (tab.url && (tab.url.startsWith("http://") || tab.url.startsWith("https://"))) {
    clipBtn.disabled = false;
  } else {
    clipBtn.disabled = true;
    clipStatus.textContent = "Cannot clip this page type.";
    clipStatus.className = "status error";
  }
}

init();
