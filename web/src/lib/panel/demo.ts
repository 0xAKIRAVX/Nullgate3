// ── Demo backend (server-only) ──────────────────────────────────────────
// Active only when API_INTERNAL_URL is unset (sandbox preview / local dev).
// It mirrors the Go api's JSON shapes so the UI can be exercised end-to-end.
// In production the next.config rewrites /api/* + /sub/* to the real Go api.

import { NextResponse } from "next/server";

export const DEMO_COOKIE = "ng_demo";
export const DEMO_USER = "admin";
export const DEMO_PASS = "admin123";
export const DEMO_HINT = process.env.NEXT_PUBLIC_DEMO_HINT === "0" ? false : true;

interface DemoClient {
  id: string;
  name: string;
  quota: number; // bytes
  expire_at: string | null;
  protocols: string[];
  note: string;
  sub_token: string;
  created_at: string;
  up: number;
  down: number;
}

interface DemoStore {
  t0: number;
  baseUp: number;
  baseDown: number;
  restarts: number;
  panel: {
    protocols: Record<string, boolean>;
    reality_sni: string;
    session_hours: number;
    cfg_fmt: string;
    address: string;
  };
  reality: { priv: string; pub: string; sid: string };
  clients: DemoClient[];
  inbounds: { tag: string; name: string; protocol: string; network: string; security: string; port?: number; lport?: number; path?: string; svc?: string; sni?: string }[];
  logs: string[];
}

const g = globalThis as unknown as { __ngDemo?: DemoStore };

function uid(): string {
  const h = "0123456789abcdef";
  let s = "";
  for (let i = 0; i < 32; i++) s += h[Math.floor(Math.random() * 16)];
  return `${s.slice(0, 8)}-${s.slice(8, 12)}-${s.slice(12, 16)}-${s.slice(16, 20)}-${s.slice(20, 32)}`;
}

function tok(): string {
  const A = "abcdefghijklmnopqrstuvwxyz0123456789";
  let s = "";
  for (let i = 0; i < 32; i++) s += A[Math.floor(Math.random() * A.length)];
  return s;
}

export function store(): DemoStore {
  if (g.__ngDemo) return g.__ngDemo;
  const now = Date.now();
  const iso = (daysAgo: number) => new Date(now - daysAgo * 86400000).toISOString();
  const inDays = (days: number) => new Date(now + days * 86400000).toISOString();
  const ALL = ["vless-reality", "vless-ws", "vless-xhttp", "vless-hu", "vmess-ws", "trojan-ws"];
  const SOME = ["vless-reality", "vless-ws", "vmess-ws"];
  g.__ngDemo = {
    t0: now,
    baseUp: 47_312_640_000,
    baseDown: 305_884_290_000,
    restarts: 2,
    panel: {
      protocols: Object.fromEntries(ALL.map((k) => [k, true])),
      reality_sni: "www.samsung.com",
      session_hours: 24,
      cfg_fmt: "{name}-{label}",
      address: "",
    },
    reality: {
      priv: "iFhLxuH3W7f0vLmqdCJmJ8hGh0o2BkxKkRLL0eErQ1o",
      pub: "tCybsS5RR3icRi-PiwGqD2ZWitDcyeljyPF2PtZKJCY",
      sid: "a83f2c1e",
    },
    clients: [
      { id: uid(), name: "mobin_x", quota: 0, expire_at: null, protocols: ALL, note: "خودم", sub_token: tok(), created_at: iso(12), up: 41_930_000_000, down: 4_180_000_000 },
      { id: uid(), name: "nima_cloud", quota: 100 * (1 << 30), expire_at: inDays(21), protocols: ALL, note: "گوشی + لپ‌تاپ", sub_token: tok(), created_at: iso(9), up: 8_640_000_000, down: 88_920_000_000 },
      { id: uid(), name: "parisa", quota: 20 * (1 << 30), expire_at: inDays(12), protocols: SOME, note: "", sub_token: tok(), created_at: iso(7), up: 1_120_000_000, down: 5_580_000_000 },
      { id: uid(), name: "reza_net", quota: 10 * (1 << 30), expire_at: inDays(28), protocols: ALL, note: "تبلت", sub_token: tok(), created_at: iso(5), up: 310_000_000, down: 1_010_000_000 },
      { id: uid(), name: "morteza", quota: 30 * (1 << 30), expire_at: iso(3), protocols: ALL, note: "منقضی — منتظر تمدید", sub_token: tok(), created_at: iso(30), up: 28_100_000_000, down: 1_990_000_000 },
    ],
    inbounds: [
      { tag: "ib-" + tok().slice(0, 6), name: "مسیر ۴۴۳ (XHTTP)", protocol: "vless", network: "xhttp", security: "tls", path: "/cdn443" },
    ],
    logs: [
      "2026/09/23 13:40:01 [info] xray-core: Xray 26.3.27 started",
      "2026/09/23 13:40:01 [info] infra/conf: Built-in settings panel is disabled",
      "2026/09/23 13:40:02 [info] proxy/vless/inbound: received request for tcp:cp.cloudflare.com:443",
      "2026/09/23 13:40:19 [info] transport/internet/reality: accepted real connection from 5.120.x.x",
      "2026/09/23 13:41:02 [info] app/proxyman/inbound: inbound vless-ws started on 127.0.0.1:10001",
      "2026/09/23 13:41:44 [warning] transport/internet/websocket: closing connection due to timeout",
      "2026/09/23 13:42:03 [info] proxy/trojan/inbound: connection established",
      "2026/09/23 13:44:10 [info] app/stats/counter: counter 'user>>>mobin_x>>>traffic>>>uplink' updated",
      "2026/09/23 13:46:27 [info] app/proxyman/outbound: failed to process outbound traffic > connection reset by peer",
      "2026/09/23 13:48:55 [info] transport/internet/reality: accepted real connection from 31.58.x.x",
    ],
  };
  return g.__ngDemo;
}

export function demoLive() {
  const s = store();
  const mins = (Date.now() - s.t0) / 60000;
  // ~1.4 MB/s combined — grows steadily so the UI feels alive
  return {
    up: Math.round(s.baseUp + mins * 38_000_000),
    down: Math.round(s.baseDown + mins * 86_000_000),
  };
}

export function demoState() {
  const s = store();
  const { up, down } = demoLive();
  const total = s.clients.length;
  const active = s.clients.filter((c) => {
    if (c.expire_at && new Date(c.expire_at).getTime() < Date.now()) return false;
    if (c.quota > 0 && c.up + c.down >= c.quota) return false;
    return true;
  }).length;
  return {
    version: "3.0.0-alpha.3",
    protocols: s.panel.protocols,
    reality: { pub: s.reality.pub, sid: s.reality.sid, sni: s.panel.reality_sni },
    ports: { app: 9000, api: 10085 },
    tcp: { host: "nullgate-tcp.up.railway.app", port: "45833", ready: true },
    clients: { total, active },
    traffic: { up, down },
    xray: { running: true, restarts: s.restarts, since: Date.now() - 3_610_000 },
  };
}

export function clientStatus(c: DemoClient): "active" | "quota" | "expired" {
  if (c.quota > 0 && c.up + c.down >= c.quota) return "quota";
  if (c.expire_at && new Date(c.expire_at).getTime() < Date.now()) return "expired";
  return "active";
}

export function clientJSON(c: DemoClient) {
  return {
    id: c.id, name: c.name, quota: c.quota, expire_at: c.expire_at,
    protocols: c.protocols, note: c.note, sub_token: c.sub_token,
    created_at: c.created_at, up: c.up, down: c.down,
    status: clientStatus(c),
    usage_pct: c.quota > 0 ? Math.round(((c.up + c.down) / c.quota) * 1000) / 10 : null,
  };
}

/** builds realistic share links — same {key,label,url} shape as the Go engine */
export function buildLinks(c: DemoClient): { key: string; label: string; url: string }[] {
  const s = store();
  const host = s.panel.address || "nullgate-api.up.railway.app";
  const fmt = (label: string) => (s.panel.cfg_fmt || "{name}-{label}").replace("{name}", c.name).replace("{label}", label);
  const b64u = (v: string) => Buffer.from(v).toString("base64url");
  const out: { key: string; label: string; url: string }[] = [];
  const push = (key: string, label: string, on: boolean, uri: string) => { if (on) out.push({ key, label, url: uri }); };
  const on = (k: string) => c.protocols.includes(k) && s.panel.protocols[k] !== false;

  push("vless-ws", fmt("VLESS-WS"), on("vless-ws"),
    `vless://${uid().slice(0, 8)}@${host}:443?type=ws&security=tls&path=%2Fws&host=${encodeURIComponent(host)}#${encodeURIComponent(fmt("VLESS-WS"))}`);
  push("vless-xhttp", fmt("VLESS-XHTTP"), on("vless-xhttp"),
    `vless://${uid().slice(0, 8)}@${host}:443?type=xhttp&security=tls&path=%2Fxhttp&host=${encodeURIComponent(host)}&mode=auto#${encodeURIComponent(fmt("VLESS-XHTTP"))}`);
  push("vless-hu", fmt("VLESS-HTTPUpgrade"), on("vless-hu"),
    `vless://${uid().slice(0, 8)}@${host}:443?type=httpupgrade&security=tls&path=%2Fhu&host=${encodeURIComponent(host)}#${encodeURIComponent(fmt("VLESS-HTTPUpgrade"))}`);
  push("vmess-ws", fmt("VMess-WS"), on("vmess-ws"),
    `vmess://${b64u(JSON.stringify({ v: "2", ps: fmt("VMess-WS"), add: host, port: "443", id: uid(), aid: "0", scy: "auto", net: "ws", type: "none", host, path: "/vmess", tls: "tls", sni: host }))}`);
  push("trojan-ws", fmt("Trojan-WS"), on("trojan-ws"),
    `trojan://${tok().slice(0, 16)}@${host}:443?security=tls&type=ws&path=%2Ftrojan&sni=${encodeURIComponent(host)}#${encodeURIComponent(fmt("Trojan-WS"))}`);
  push("vless-reality", fmt("VLESS-Reality-TCP"), on("vless-reality"),
    `vless://${uid().slice(0, 8)}-${uid().slice(0, 4)}@${host.replace(/:\d+$/, "")}:45833?type=tcp&security=reality&pbk=${encodeURIComponent(s.reality.pub)}&fp=chrome&sni=${encodeURIComponent(s.panel.reality_sni)}&sid=${s.reality.sid}&spx=%2F&flow=xtls-rprx-vision#${encodeURIComponent(fmt("VLESS-Reality-TCP"))}`);
  return out;
}

export function demoAuthed(req: Request): boolean {
  const raw = req.headers.get("cookie") || "";
  return raw.split(";").some((p) => p.trim() === `${DEMO_COOKIE}=1`);
}

export function unauthed(): NextResponse {
  return NextResponse.json({ ok: false, error: "unauthorized" }, { status: 401 });
}

export const json = (body: unknown, status = 200) => NextResponse.json(body, { status });
