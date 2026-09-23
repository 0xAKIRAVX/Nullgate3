import { demoAuthed, json, store, unauthed } from "@/lib/panel/demo";

export async function GET(req: Request) {
  if (!demoAuthed(req)) return unauthed();
  return json(store().panel);
}

export async function PUT(req: Request) {
  if (!demoAuthed(req)) return unauthed();
  const body = (await req.json().catch(() => ({}))) as Record<string, unknown>;
  const s = store();
  if (body.protocols && typeof body.protocols === "object") {
    for (const [k, v] of Object.entries(body.protocols as Record<string, boolean>)) {
      if (typeof v === "boolean") s.panel.protocols[k] = v;
    }
  }
  if (typeof body.reality_sni === "string" && body.reality_sni.trim()) s.panel.reality_sni = body.reality_sni.trim();
  if (typeof body.cfg_fmt === "string") s.panel.cfg_fmt = body.cfg_fmt;
  if (typeof body.address === "string") s.panel.address = body.address;
  return json(s.panel);
}

export const dynamic = "force-dynamic";
