import { demoAuthed, json, store, unauthed } from "@/lib/panel/demo";

export async function GET(req: Request) {
  if (!demoAuthed(req)) return unauthed();
  const s = store();
  return json({
    builtin: [
      { tag: "vless-reality", name: "Reality TCP", protocol: "vless", network: "tcp", security: "reality", port: 9000, builtin: true },
      { tag: "vless-ws", name: "VLESS WS", protocol: "vless", network: "ws", security: "none", path: "/ws", builtin: true },
      { tag: "vless-xhttp", name: "VLESS XHTTP", protocol: "vless", network: "xhttp", security: "none", path: "/xhttp", builtin: true },
      { tag: "vless-hu", name: "VLESS HTTPUpgrade", protocol: "vless", network: "httpupgrade", security: "none", path: "/hu", builtin: true },
      { tag: "vmess-ws", name: "VMess WS", protocol: "vmess", network: "ws", security: "none", path: "/vmess", builtin: true },
      { tag: "trojan-ws", name: "Trojan WS", protocol: "trojan", network: "ws", security: "none", path: "/trojan", builtin: true },
    ],
    custom: s.inbounds.map((i) => ({ ...i, builtin: false })),
  });
}

export async function POST(req: Request) {
  if (!demoAuthed(req)) return unauthed();
  const b = (await req.json().catch(() => ({}))) as {
    name?: string; protocol?: string; network?: string; security?: string; port?: number; path?: string; sni?: string;
  };
  const name = (b.name || "").trim();
  if (!name) return json({ ok: false, error: "نام اینباند را وارد کنید" }, 400);
  if (b.security === "reality" && (b.protocol !== "vless" || !["tcp", "xhttp", "grpc"].includes(b.network || ""))) {
    return json({ ok: false, error: "Reality فقط با VLESS و ترنسپورت TCP یا XHTTP یا gRPC کار می‌کند" }, 400);
  }
  const s = store();
  s.inbounds.push({
    tag: "ib-" + Math.random().toString(16).slice(2, 10),
    name,
    protocol: b.protocol || "vless",
    network: b.network || "ws",
    security: b.security || "tls",
    port: b.port,
    path: b.path,
    sni: b.sni,
  });
  s.restarts += 1;
  return json({ ok: true });
}

export const dynamic = "force-dynamic";
