import { clientJSON, demoAuthed, store, unauthed } from "@/lib/panel/demo";

export async function GET(req: Request, ctx: { params: Promise<{ id: string }> }) {
  if (!demoAuthed(req)) return unauthed();
  const { id } = await ctx.params;
  const c = store().clients.find((x) => x.id === id);
  if (!c) return Response.json({ ok: false, error: "client not found" }, { status: 404 });
  return new Response(JSON.stringify({ remark: c.name, demo: true }, null, 2), {
    headers: {
      "Content-Type": "application/json; charset=utf-8",
      "Content-Disposition": `attachment; filename=nullgate-${c.name}.json`,
    },
  });
}

export const dynamic = "force-dynamic";
