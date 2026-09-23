import { demoAuthed, unauthed } from "@/lib/panel/demo";

const CFG = {
  log: { loglevel: "warning" },
  inbounds: [
    { tag: "vless-reality", listen: "0.0.0.0", port: 9000, protocol: "vless", settings: {}, streamSettings: { network: "tcp", security: "reality" } },
    { tag: "vless-ws", listen: "127.0.0.1", port: 10001, protocol: "vless" },
    { tag: "vless-xhttp", listen: "127.0.0.1", port: 10002, protocol: "vless" },
    { tag: "vless-hu", listen: "127.0.0.1", port: 10006, protocol: "vless" },
    { tag: "vmess-ws", listen: "127.0.0.1", port: 10004, protocol: "vmess" },
    { tag: "trojan-ws", listen: "127.0.0.1", port: 10005, protocol: "trojan" },
  ],
  outbounds: [{ tag: "direct", protocol: "freedom" }],
};

export async function GET(req: Request) {
  if (!demoAuthed(req)) return unauthed();
  return new Response(JSON.stringify(CFG, null, 2), {
    headers: {
      "Content-Type": "application/json; charset=utf-8",
      "Content-Disposition": 'attachment; filename="xray-server.json"',
    },
  });
}

export const dynamic = "force-dynamic";
