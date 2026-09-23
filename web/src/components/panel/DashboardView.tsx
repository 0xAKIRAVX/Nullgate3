"use client";

import { useEffect, useMemo, useRef, useState } from "react";
import {
  Activity, ArrowDownToLine, ArrowUpFromLine, Cpu, Database, Globe, KeyRound,
  Server, ShieldCheck, Users2, Wifi, Zap,
} from "lucide-react";
import type { ClientRow, PanelState } from "@/lib/panel/types";
import { PROTOCOL_KEYS, PROTOCOL_LABELS } from "@/lib/panel/types";
import type { LiveState } from "@/lib/panel/useLive";
import { bytesFmt, jalali, uptimeLabel } from "@/lib/panel/format";
import { Badge, CopyBtn, Sparkline } from "./bits";

export default function DashboardView({
  st, live, clients, onGoUsers,
}: {
  st: PanelState;
  live: LiveState;
  clients: ClientRow[];
  onGoUsers: () => void;
}) {
  const total = live.up + live.down;
  // live delta samples → sparkline (state so renders stay pure)
  const [spark, setSpark] = useState<number[]>([]);
  const last = useRef<number>(total);
  const prevMode = useRef(live.mode);
  useEffect(() => {
    const modeChanged = prevMode.current !== live.mode;
    prevMode.current = live.mode;
    const d = Math.max(0, total - last.current);
    last.current = total;
    // a pure mode flip (init → ws/poll) carries no new sample — pushing the
    // 0 delta used to dent the graph once on every connection
    if (live.mode !== "init" && !modeChanged) {
      setSpark((s) => [...s.slice(-39), d]);
    }
  }, [total, live.mode]);

  const recent = useMemo(() => clients.slice(0, 5), [clients]);
  const protoOn = PROTOCOL_KEYS.filter((k) => st.protocols?.[k] !== false);

  return (
    <div className="space-y-5">
      {/* hero */}
      <section
        className="relative overflow-hidden rounded-[18px] border border-line min-h-[190px] flex items-end"
        style={{ backgroundImage: "linear-gradient(to left, rgba(6,6,8,.86) 8%, rgba(6,6,8,.25) 60%), url(/hero-bg.jpg)", backgroundSize: "cover", backgroundPosition: "center" }}
      >
        <div className="relative z-10 p-5 sm:p-6 w-full">
          <p className="text-[11px] text-mu mb-1">آزادی در دستان یوست</p>
          <div className="flex flex-wrap items-center gap-x-3 gap-y-2">
            <h1 className="text-2xl sm:text-[27px] font-bold gold-text leading-tight">داشبورد NullGate ۳.۰</h1>
            <span className={`inline-flex items-center gap-1.5 rounded-full border px-2.5 py-1 text-[10.5px] font-semibold ${live.mode === "ws" ? "border-ok/40 bg-ok/10 text-ok" : "border-gold/40 bg-gold/10 text-gtx"}`}>
              <span className="size-1.5 rounded-full bg-current live-dot" />
              {live.mode === "ws" ? "زنده (WebSocket)" : live.mode === "poll" ? "زنده (Polling)" : "…"}
            </span>
          </div>
          <p className="text-[11.5px] text-mu mt-1.5">اتصال امن، بدون محدودیت — استک جدید: Next.js + Go + PostgreSQL + Redis</p>
          <div className="mt-3 flex flex-wrap gap-2 text-[10.5px]">
            <span className="rounded-full border border-gold/30 bg-black/40 px-2.5 py-1 text-gtx" dir="ltr">{st.version}</span>
            <span className={`rounded-full border px-2.5 py-1 ${live.xray.running ? "border-ok/40 bg-black/40 text-ok" : "border-bad/40 bg-black/40 text-bad"}`}>
              Xray {live.xray.running ? "در حال اجرا" : "متوقف"}
            </span>
            <span className="rounded-full border border-gold/30 bg-black/40 px-2.5 py-1 text-gtx">۶ پروتکل</span>
          </div>
        </div>
      </section>

      {/* stat cards */}
      <section className="grid grid-cols-2 xl:grid-cols-4 gap-3 sm:gap-4">
        <StatCard
          icon={<Users2 className="size-5 text-gtx" />} label="کاربران"
          value={String(st.clients.total)}
          sub={`${st.clients.active.toLocaleString("fa-IR")} کاربر فعال`}
          tone="gold"
        />
        <StatCard
          icon={<Activity className="size-5 text-ok" />} label="ترافیک کل (زنده)"
          value={bytesFmt(total)}
          sub={`آپلود ${bytesFmt(live.up)} · دانلود ${bytesFmt(live.down)}`}
          tone="ok"
        />
        <StatCard
          icon={<Server className="size-5 text-cyan" />} label="هسته Xray"
          value={live.xray.running ? "سالم" : "متوقف"}
          sub={`${live.xray.restarts.toLocaleString("fa-IR")} ری‌استارت · ${uptimeLabel(st.xray.since)}`}
          tone={live.xray.running ? "ok" : "bad"}
        />
        <StatCard
          icon={<Database className="size-5 text-purple" />} label="پایگاه داده"
          value="PostgreSQL"
          sub="کلیدهای Reality ماندگار"
          tone="gold"
        />
      </section>

      {/* live traffic + reality */}
      <section className="grid lg:grid-cols-3 gap-3 sm:gap-4">
        <div className="ng-card p-5 lg:col-span-2">
          <div className="flex items-center justify-between gap-3 flex-wrap">
            <div>
              <h2 className="font-bold text-[15px]">ترافیک زنده سرور</h2>
              <p className="text-[11px] text-mu mt-0.5">نرخ انتقال بر حسب بایت بر بازه نمونه‌برداری</p>
            </div>
            <div className="flex gap-4 text-sm">
              <span className="inline-flex items-center gap-1.5 text-ok mono" dir="ltr">
                <ArrowUpFromLine className="size-3.5" /> {bytesFmt(live.up)}
              </span>
              <span className="inline-flex items-center gap-1.5 text-info mono" dir="ltr">
                <ArrowDownToLine className="size-3.5" /> {bytesFmt(live.down)}
              </span>
            </div>
          </div>
          <div className="mt-4">
            <Sparkline data={spark} />
          </div>
          <p className="mt-2 text-[10.5px] text-mu">
            منبع: {live.mode === "ws" ? "استریم WebSocket (/api/ws)" : live.mode === "poll" ? "نظرسنجی /api/state" : "…"} — آمار هر ۳۰ ثانیه در PostgreSQL ثبت می‌شود
          </p>
        </div>

        <div className="ng-card-gold p-5">
          <div className="flex items-center gap-2">
            <KeyRound className="size-4.5 text-gtx" />
            <h2 className="font-bold text-[15px]">کلید Reality</h2>
            <Badge tone="ok"><ShieldCheck className="size-3" /> ماندگار در DB</Badge>
          </div>
          <dl className="mt-4 space-y-3 text-xs">
            <div>
              <dt className="text-mu mb-1">SNI</dt>
              <dd className="flex items-center gap-2">
                <code className="mono text-[11px] bg-black/30 rounded-lg px-2 py-1 flex-1 truncate" dir="ltr">{st.reality.sni || "—"}</code>
                {st.reality.sni ? <CopyBtn text={st.reality.sni} /> : null}
              </dd>
            </div>
            <div>
              <dt className="text-mu mb-1">Public Key</dt>
              <dd className="flex items-center gap-2">
                <code className="mono text-[11px] bg-black/30 rounded-lg px-2 py-1 flex-1 truncate" dir="ltr">{st.reality.pub || "—"}</code>
                {st.reality.pub ? <CopyBtn text={st.reality.pub} /> : null}
              </dd>
            </div>
            <div>
              <dt className="text-mu mb-1">Short ID</dt>
              <dd className="flex items-center gap-2">
                <code className="mono text-[11px] bg-black/30 rounded-lg px-2 py-1 flex-1 truncate" dir="ltr">{st.reality.sid || "—"}</code>
                {st.reality.sid ? <CopyBtn text={st.reality.sid} /> : null}
              </dd>
            </div>
            <div className="border-t border-gold/15 pt-3 flex items-center justify-between">
              <dt className="text-mu">Reality TCP Proxy</dt>
              <dd>
                {st.tcp.ready ? (
                  <code className="mono text-[11px] text-gtx" dir="ltr">{st.tcp.host}:{st.tcp.port}</code>
                ) : (
                  <Badge tone="warn">تنظیم نشده</Badge>
                )}
              </dd>
            </div>
          </dl>
        </div>
      </section>

      {/* protocols + info */}
      <section className="grid lg:grid-cols-3 gap-3 sm:gap-4">
        <div className="ng-card p-5 lg:col-span-2">
          <h2 className="font-bold text-[15px]">پروتکل‌های فعال</h2>
          <p className="text-[11px] text-mu mt-0.5">هر کاربر به‌صورت پیش‌فرض لینک هر ۶ پروتکل را دریافت می‌کند</p>
          <div className="mt-4 grid grid-cols-2 sm:grid-cols-3 gap-2.5">
            {PROTOCOL_KEYS.map((k) => {
              const on = st.protocols?.[k] !== false;
              return (
                <div key={k} className={`rounded-xl border px-3 py-3 flex items-center gap-2.5 transition ${on ? "border-gold/25 bg-gold/6" : "border-line bg-white/2 opacity-45"}`}>
                  <span className={`size-2 rounded-full ${on ? "bg-ok live-dot" : "bg-mu/40"}`} />
                  <div className="min-w-0">
                    <p className="text-[12.5px] font-semibold truncate">{PROTOCOL_LABELS[k]}</p>
                    <p className="text-[10px] text-mu mono" dir="ltr">{k}</p>
                  </div>
                </div>
              );
            })}
          </div>
          <div className="mt-4 flex items-center gap-2 text-[11px] text-mu">
            <Zap className="size-3.5 text-gtx" />
            درون‌ریزی مسیرها مستقیم درون پروسه Go انجام می‌شود — بدون nginx
          </div>
        </div>

        <div className="ng-card p-5 space-y-3.5 text-xs">
          <h2 className="font-bold text-[15px] text-tx">مشخصات سیستم</h2>
          <InfoRow icon={<Server className="size-3.5 text-gtx" />} k="پورت Reality داخلی" v={String(st.ports.app)} mono />
          <InfoRow icon={<Cpu className="size-3.5 text-cyan" />} k="پورت API آمار" v={String(st.ports.api)} mono />
          <InfoRow icon={<Globe className="size-3.5 text-purple" />} k="نسخه پنل" v={st.version} mono />
          <InfoRow icon={<Wifi className="size-3.5 text-ok" />} k="انتقال آمار" v={live.mode === "ws" ? "WebSocket" : "Polling ۵ ثانیه"} />
          <div className="border-t border-line pt-3 flex items-center justify-between">
            <span className="text-mu">آخرین کاربران</span>
            <button onClick={onGoUsers} className="text-gtx hover:text-gold2 text-[11.5px] font-semibold transition">
              مشاهده همه ←
            </button>
          </div>
          <ul className="space-y-2">
            {recent.map((c) => (
              <li key={c.id} className="flex items-center justify-between gap-2">
                <span className="truncate text-[12px] font-medium">{c.name}</span>
                <span className="text-[10.5px] text-mu shrink-0">{jalali(c.created_at)}</span>
              </li>
            ))}
            {recent.length === 0 && <li className="text-mu">هنوز کاربری ثبت نشده</li>}
          </ul>
        </div>
      </section>
    </div>
  );
}

function StatCard({
  icon, label, value, sub, tone,
}: {
  icon: React.ReactNode; label: string; value: string; sub: string; tone: "gold" | "ok" | "bad";
}) {
  const ring = tone === "ok" ? "bg-ok/10 border-ok/20" : tone === "bad" ? "bg-bad/10 border-bad/20" : "bg-gold/10 border-gold/20";
  return (
    <div className="ng-card p-4 sm:p-5 flex items-center gap-3.5 hover:border-gold/25 transition">
      <span className={`size-11 rounded-xl border grid place-items-center shrink-0 ${ring}`}>{icon}</span>
      <div className="min-w-0">
        <p className="text-[11px] text-mu">{label}</p>
        <p className="text-lg sm:text-xl font-bold mt-0.5 truncate" dir="auto">{value}</p>
        <p className="text-[10.5px] text-mu mt-0.5 truncate">{sub}</p>
      </div>
    </div>
  );
}

function InfoRow({ icon, k, v, mono }: { icon: React.ReactNode; k: string; v: string; mono?: boolean }) {
  return (
    <div className="flex items-center justify-between gap-2">
      <span className="flex items-center gap-2 text-mu">{icon}{k}</span>
      <span className={mono ? "mono text-[11.5px]" : "text-[11.5px]"} dir={mono ? "ltr" : "auto"}>{v}</span>
    </div>
  );
}
