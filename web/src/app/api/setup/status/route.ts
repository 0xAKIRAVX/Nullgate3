export async function GET() {
  return Response.json({ needs_setup: false });
}

export const dynamic = "force-dynamic";
