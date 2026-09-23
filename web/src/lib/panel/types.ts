// ── NullGate 3.0 API types (mirror of the Go api structs) ──────────────

export const PROTOCOL_KEYS = [
  "vless-reality",
  "vless-ws",
  "vless-xhttp",
  "vless-hu",
  "vmess-ws",
  "trojan-ws",
] as const;

export type ProtocolKey = (typeof PROTOCOL_KEYS)[number];

export const PROTOCOL_LABELS: Record<ProtocolKey, string> = {
  "vless-reality": "Reality (TCP)",
  "vless-ws": "VLESS WS",
  "vless-xhttp": "VLESS XHTTP",
  "vless-hu": "VLESS HTTPUpgrade",
  "vmess-ws": "VMess WS",
  "trojan-ws": "Trojan WS",
};

export interface ClientRow {
  id: string;
  name: string;
  quota: number; // bytes, 0 = unlimited
  expire_at: string | null; // RFC3339 or null
  protocols: string[];
  note: string;
  sub_token: string;
  created_at: string;
  up: number;
  down: number;
  status: "active" | "quota" | "expired";
  usage_pct: number | null;
}

export interface PanelState {
  version: string;
  protocols: Record<string, boolean>;
  reality: { pub?: string; sid?: string; sni?: string };
  ports: { app: number; api: number };
  tcp: { host: string; port: string; ready: boolean };
  clients: { total: number; active: number };
  traffic: { up: number; down: number };
  xray: { running: boolean; restarts: number; since?: unknown };
}

export interface PanelSettings {
  protocols: Record<string, boolean>;
  reality_sni: string;
  session_hours: number;
  cfg_fmt: string;
  address: string;
}

export interface InboundInfo {
  tag: string;
  name: string;
  protocol: string;
  network: string;
  security: string;
  port?: number;
  lport?: number;
  path?: string;
  svc?: string;
  sni?: string;
  builtin: boolean;
}

export interface LiveMsg {
  type: "traffic";
  up: number;
  down: number;
  xray: { running: boolean; restarts: number; since?: unknown };
}

/** one shareable config — mirror of xray.Link in the Go engine */
export interface ShareLink {
  key: string;
  label: string;
  url: string;
}
