// Auth API client — uses Connect-go RPC (same protocol as all other services).

const AUTH_SERVICE = "/blackwood.v1.AuthService";

// Connect-go RPC helper (duplicated from client.ts to avoid circular deps
// and to skip the 401-redirect logic for auth calls).
async function authRpc<Req, Res>(method: string, request: Req): Promise<Res> {
  const url = `${AUTH_SERVICE}/${method}`;
  const resp = await fetch(url, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(request),
  });
  if (!resp.ok) {
    const text = await resp.text();
    throw new Error(`RPC ${method} failed (${resp.status}): ${text}`);
  }
  return resp.json();
}

export interface AuthStatus {
  authenticated: boolean;
  setupRequired: boolean;
}

export interface SetupResponse {
  secret: string;
  qrCode: string; // base64-encoded PNG
}

export async function getAuthStatus(): Promise<AuthStatus> {
  return authRpc<Record<string, never>, AuthStatus>("Status", {});
}

export async function getSetupInfo(): Promise<SetupResponse> {
  return authRpc<Record<string, never>, SetupResponse>("GetSetupInfo", {});
}

export async function submitSetup(secret: string, code: string): Promise<{ ok: boolean; error?: string }> {
  return authRpc<{ secret: string; code: string }, { ok: boolean; error?: string }>("ConfirmSetup", { secret, code });
}

export async function login(code: string): Promise<{ ok: boolean; error?: string }> {
  try {
    return await authRpc<{ code: string }, { ok: boolean; error?: string }>("Login", { code });
  } catch (err) {
    if (err instanceof Error && err.message.includes("429")) {
      return { ok: false, error: "Too many attempts. Try again later." };
    }
    throw err;
  }
}

export async function logout(): Promise<void> {
  await authRpc<Record<string, never>, Record<string, never>>("Logout", {});
}
