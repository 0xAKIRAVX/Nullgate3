import { NextResponse } from "next/server";

// health — demo build signals the UI so it can show a "دمو" badge
export async function GET() {
  return NextResponse.json({
    ok: true,
    version: "3.0.0-alpha.3",
    db: true,
    redis: true,
    demo: true,
  });
}
