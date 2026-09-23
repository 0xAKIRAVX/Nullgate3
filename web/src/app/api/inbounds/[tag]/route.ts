import { demoAuthed, json, store, unauthed } from "@/lib/panel/demo";

export async function DELETE(req: Request, ctx: { params: Promise<{ tag: string }> }) {
  if (!demoAuthed(req)) return unauthed();
  const { tag } = await ctx.params;
  const s = store();
  const idx = s.inbounds.findIndex((i) => i.tag === tag);
  if (idx < 0) return json({ ok: false, error: "inbound not found" }, 404);
  s.inbounds.splice(idx, 1);
  s.restarts += 1;
  return json({ ok: true });
}

export const dynamic = "force-dynamic";
