// ── formatting helpers (Persian labels, Jalali dates, byte sizes) ──────

export function bytesFmt(n: number, digits = true): string {
  if (!isFinite(n)) return "—";
  if (n <= 0) return digits ? "0" : "0 B";
  const units = ["B", "KB", "MB", "GB", "TB", "PB"];
  const i = Math.min(Math.floor(Math.log(n) / Math.log(1024)), units.length - 1);
  const v = n / Math.pow(1024, i);
  const s = v >= 100 || i === 0 ? v.toFixed(0) : v >= 10 ? v.toFixed(1) : v.toFixed(2);
  return `${s} ${units[i]}`;
}
// keep zero consistent with other values ("0 B"), not unit-less "0"
export function bytesFmt0(n: number): string {
  return n <= 0 ? "0 B" : bytesFmt(n);
}

export function gbFmt(gb: number): string {
  return gb > 0 ? `${gb.toLocaleString("en-US")} GB` : "نامحدود";
}

export function jalali(iso: string | null | undefined): string {
  if (!iso) return "—";
  const d = new Date(iso);
  if (isNaN(d.getTime())) return "—";
  return new Intl.DateTimeFormat("fa-IR", {
    year: "numeric", month: "2-digit", day: "2-digit",
  }).format(d);
}

export function daysLeft(iso: string | null | undefined): number | null {
  if (!iso) return null;
  const d = new Date(iso).getTime();
  if (isNaN(d)) return null;
  // calendar-day difference, not 24h buckets: expiring tonight at 23:30 must
  // read as "امروز", not "۱ روز مانده" — and tomorrow's date is "فردا".
  const startOfDay = (t: number) => {
    const x = new Date(t);
    x.setHours(0, 0, 0, 0);
    return x.getTime();
  };
  return Math.round((startOfDay(d) - startOfDay(Date.now())) / 86400000);
}

export function expireLabel(iso: string | null | undefined): string {
  if (!iso) return "نامحدود";
  const t = new Date(iso).getTime();
  if (isNaN(t)) return "—";
  if (t <= Date.now()) return "منقضی شده";
  const n = daysLeft(iso);
  if (n === null) return "—";
  if (n <= 0) return "امروز";
  if (n === 1) return "فردا";
  return `${n.toLocaleString("fa-IR")} روز مانده`;
}

export function pct(used: number, total: number): number {
  if (!total || total <= 0) return 0;
  return Math.min(100, Math.round((used / total) * 1000) / 10);
}

export function uptimeLabel(since: unknown): string {
  const t = typeof since === "number" ? since : 0;
  // Go atomic.Value may marshal unixnano or rfc3339 — handle both
  let ms = 0;
  if (t > 1e15) ms = t / 1e6; // unix nano
  else if (t > 1e11) ms = t; // unix milli
  else if (t > 1e8) ms = t * 1000; // unix sec
  if (!ms) return "—";
  const s = Math.max(0, Math.floor((Date.now() - ms) / 1000));
  const h = Math.floor(s / 3600), m = Math.floor((s % 3600) / 60);
  if (h > 0) return `${h.toLocaleString("fa-IR")} ساعت و ${m.toLocaleString("fa-IR")} دقیقه`;
  return `${m.toLocaleString("fa-IR")} دقیقه`;
}

/** extracts host:port from a vless/vmess/trojan URI for display */
export function linkHost(uri: string): string {
  try {
    if (uri.startsWith("vmess://")) {
      const j = JSON.parse(atob(uri.slice(8)));
      return `${j.add}:${j.port}`;
    }
    // share links look like scheme://uuid@host:443?query#frag — the original
    // regex required a "/" after the port, so every real link matched nothing
    const m = uri.match(/^[a-z]+:\/\/[^@/]+@([^:/?#]+):(\d+)(?:[/?#]|$)/);
    return m ? `${m[1]}:${m[2]}` : "—";
  } catch {
    return "—";
  }
}

/** human label of a share link (vmess carries it inside base64 JSON) */
export function linkLabel(uri: string): string {
  try {
    if (uri.startsWith("vmess://")) {
      return String(JSON.parse(atob(uri.slice(8))).ps || "VMess");
    }
    const h = uri.match(/#([^#]*)$/);
    return h ? decodeURIComponent(h[1]) : "کانفیگ";
  } catch {
    return "کانفیگ";
  }
}
