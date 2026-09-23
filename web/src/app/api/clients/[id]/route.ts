import { clientJSON, demoAuthed, json, store, unauthed } from "@/lib/panel/demo";

export async function PATCH(req: Request, ctx: { params: Promise<{ id: string }> }) {
  if (!demoAuthed(req)) return unauthed();
  const { id } = await ctx.params;
  const s = store();
  const c = s.clients.find((x) => x.id === id);
  if (!c) return json({ ok: false, error: "client not found" }, 404);
  const b = (await req.json().catch(() => ({}))) as {
    name?: string; quota_gb?: number; expire_days?: number; protocols?: string[]; note?: string;
  };
  if (b.name !== undefined) c.name = b.name.trim() || c.name;
  if (b.quota_gb !== undefined) c.quota = Math.max(0, Math.round(b.quota_gb * (1 << 30)));
  if (b.expire_days !== undefined) {
    c.expire_at = b.expire_days > 0
      ? new Date(Date.now() + b.expire_days * 86400000).toISOString()
      : null;
  }
  if (b.protocols) c.protocols = b.protocols;
  if (b.note !== undefined) c.note = b.note;
  return json(clientJSON(c));
}

export async function DELETE(req: Request, ctx: { params: Promise<{ id: string }> }) {
  if (!demoAuthed(req)) return unauthed();
  const { id } = await ctx.params;
  const s = store();
  const idx = s.clients.findIndex((x) => x.id === id);
  if (idx < 0) return json({ ok: false, error: "client not found" }, 404);
  s.clients.splice(idx, 1);
  return json({ ok: true });
}

export const dynamic = "force-dynamic";
