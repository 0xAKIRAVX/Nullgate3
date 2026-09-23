import { buildLinks, demoAuthed, json, store, unauthed } from "@/lib/panel/demo";

export async function GET(req: Request, ctx: { params: Promise<{ id: string }> }) {
  if (!demoAuthed(req)) return unauthed();
  const { id } = await ctx.params;
  const c = store().clients.find((x) => x.id === id);
  if (!c) return json({ ok: false, error: "client not found" }, 404);
  return json({ links: buildLinks(c) });
}

export const dynamic = "force-dynamic";
