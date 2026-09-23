import { NextResponse } from "next/server";
import { DEMO_COOKIE, DEMO_PASS, DEMO_USER } from "@/lib/panel/demo";

export async function POST(req: Request) {
  const { username, password } = (await req.json().catch(() => ({}))) as {
    username?: string; password?: string;
  };
  if (username?.trim().toLowerCase() !== DEMO_USER || password !== DEMO_PASS) {
    return NextResponse.json({ ok: false, error: "wrong username or password" }, { status: 401 });
  }
  const res = NextResponse.json({ ok: true });
  res.cookies.set(DEMO_COOKIE, "1", {
    httpOnly: true, path: "/", sameSite: "lax", maxAge: 24 * 3600,
  });
  return res;
}
