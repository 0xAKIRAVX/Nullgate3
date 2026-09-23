import { buildLinks, store } from "@/lib/panel/demo";

// public subscription endpoint — mirrors the Go /sub/{token} behavior
export async function GET(req: Request, ctx: { params: Promise<{ token: string }> }) {
  const { token } = await ctx.params;
  const c = store().clients.find((x) => x.sub_token === token);
  if (!c) return new Response("not found", { status: 404 });
  const links = buildLinks(c);
  const body = Buffer.from(links.join("\n"), "utf8").toString("base64");
  const info = [`upload=${c.up}`, `download=${c.down}`];
  if (c.quota > 0) info.push(`total=${c.quota}`);
  if (c.expire_at) info.push(`expire=${Math.floor(new Date(c.expire_at).getTime() / 1000)}`);
  return new Response(body, {
    headers: {
      "Content-Type": "text/plain; charset=utf-8",
      "Profile-Title": `base64:${Buffer.from(c.name).toString("base64")}`,
      "Profile-Update-Interval": "12",
      "Subscription-Userinfo": info.join("; "),
    },
  });
}

export const dynamic = "force-dynamic";
