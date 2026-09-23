// ── REST client — same-origin, cookie auth (ng_session) ────────────────

async function req<T>(path: string, init?: RequestInit): Promise<T> {
  const res = await fetch(path, {
    credentials: "include",
    ...init,
    headers: { "Content-Type": "application/json", ...(init?.headers || {}) },
  });
  let body: unknown = null;
  try {
    body = await res.json();
  } catch {
    /* non-json (downloads etc.) */
  }
  if (!res.ok) {
    const msg =
      (body as { error?: string } | null)?.error ||
      (res.status === 401 ? "نشست منقضی شده — دوباره وارد شوید" : `خطای ${res.status}`);
    throw new Error(msg);
  }
  return body as T;
}

export const api = {
  health: () => req<{ ok: boolean; version: string; demo?: boolean }>("/api/health"),
  setupStatus: () => req<{ needs_setup: boolean }>("/api/setup/status"),
  setup: (username: string, password: string) =>
    req<{ ok: true }>("/api/setup", { method: "POST", body: JSON.stringify({ username, password }) }),
  login: (username: string, password: string) =>
    req<{ ok: true }>("/api/login", { method: "POST", body: JSON.stringify({ username, password }) }),
  logout: () => req<{ ok: true }>("/api/logout", { method: "POST" }),
  me: () => req<{ username: string; role: string }>("/api/me"),
  state: () => req<import("./types").PanelState>("/api/state"),
  settings: () => req<import("./types").PanelSettings>("/api/settings"),
  putSettings: (p: Partial<import("./types").PanelSettings>) =>
    req<import("./types").PanelSettings>("/api/settings", { method: "PUT", body: JSON.stringify(p) }),
  clients: () => req<import("./types").ClientRow[]>("/api/clients"),
  createClient: (b: { name: string; quota_gb: number; expire_days: number; protocols: string[]; note: string }) =>
    req<import("./types").ClientRow>("/api/clients", { method: "POST", body: JSON.stringify(b) }),
  patchClient: (id: string, b: Partial<{ name: string; quota_gb: number; expire_days: number; protocols: string[]; note: string }>) =>
    req<import("./types").ClientRow>(`/api/clients/${id}`, { method: "PATCH", body: JSON.stringify(b) }),
  deleteClient: (id: string) => req<{ ok: true }>(`/api/clients/${id}`, { method: "DELETE" }),
  links: (id: string) => req<{ links: import("./types").ShareLink[] }>(`/api/clients/${id}/links`),
  inbounds: () => req<{ builtin: import("./types").InboundInfo[]; custom: import("./types").InboundInfo[] }>("/api/inbounds"),
  createInbound: (b: { name: string; protocol: string; network: string; security: string; port?: number; path?: string; sni?: string }) =>
    req<{ ok: true }>("/api/inbounds", { method: "POST", body: JSON.stringify(b) }),
  deleteInbound: (tag: string) => req<{ ok: true }>(`/api/inbounds/${tag}`, { method: "DELETE" }),
  logs: () => req<{ lines: string[] }>("/api/logs"),
  restart: () => req<{ ok: true }>("/api/restart", { method: "POST" }),
};

export function subURL(token: string): string {
  if (typeof window === "undefined") return `/sub/${token}`;
  return `${window.location.origin}/sub/${token}`;
}

export function downloadClientJSON(id: string, name: string) {
  const a = document.createElement("a");
  a.href = `/api/clients/${id}/config`;
  a.download = `nullgate-${name}.json`;
  document.body.appendChild(a);
  a.click();
  a.remove();
}
