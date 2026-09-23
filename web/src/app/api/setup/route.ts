export async function POST() {
  return Response.json({ ok: false, error: "setup already completed" }, { status: 403 });
}
