import { demoAuthed, demoState, unauthed } from "@/lib/panel/demo";

export async function GET(req: Request) {
  if (!demoAuthed(req)) return unauthed();
  return Response.json(demoState());
}

export const dynamic = "force-dynamic";
