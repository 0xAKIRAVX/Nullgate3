import { demoAuthed, store, unauthed } from "@/lib/panel/demo";

export async function GET(req: Request) {
  if (!demoAuthed(req)) return unauthed();
  return Response.json({ lines: store().logs });
}

export const dynamic = "force-dynamic";
