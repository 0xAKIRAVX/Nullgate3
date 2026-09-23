import { demoAuthed, json, store, unauthed } from "@/lib/panel/demo";

export async function POST(req: Request) {
  if (!demoAuthed(req)) return unauthed();
  const s = store();
  s.restarts += 1;
  return json({ ok: true, xray: { running: true, restarts: s.restarts, since: Date.now() } });
}

export const dynamic = "force-dynamic";
