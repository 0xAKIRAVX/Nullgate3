import { clientJSON, demoAuthed, json, store, unauthed } from "@/lib/panel/demo";

function tok() {
  const A = "abcdefghijklmnopqrstuvwxyz0123456789";
  let s = "";
  for (let i = 0; i < 32; i++) s += A[Math.floor(Math.random() * A.length)];
  return s;
}
function uid() {
  return crypto.randomUUID().replaceAll("-", "");
}

export async function GET(req: Request) {
  if (!demoAuthed(req)) return unauthed();
  return json(store().clients.map(clientJSON));
}

export async function POST(req: Request) {
  if (!demoAuthed(req)) return unauthed();
  const b = (await req.json().catch(() => ({}))) as {
    name?: string; quota_gb?: number; expire_days?: number; protocols?: string[]; note?: string;
  };
  const name = (b.name || "").trim();
  if (!name) return json({ ok: false, error: "name is required" }, 400);
  const s = store();
  const now = Date.now();
  s.clients.unshift({
    id: uid(),
    name,
    quota: Math.max(0, Math.round((b.quota_gb || 0) * (1 << 30))),
    expire_at: (b.expire_days || 0) > 0 ? new Date(now + (b.expire_days as number) * 86400000).toISOString() : null,
    protocols: b.protocols?.length ? b.protocols : ["vless-reality", "vless-ws", "vless-xhttp", "vless-hu", "vmess-ws", "trojan-ws"],
    note: b.note || "",
    sub_token: tok(),
    created_at: new Date(now).toISOString(),
    up: 0,
    down: 0,
  });
  return json(clientJSON(s.clients[0]));
}

export const dynamic = "force-dynamic";
