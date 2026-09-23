import { demoAuthed, json, unauthed } from "@/lib/panel/demo";

export async function GET(req: Request) {
  if (!demoAuthed(req)) return unauthed();
  return json({ username: "admin", role: "admin" });
}

export const dynamic = "force-dynamic";
