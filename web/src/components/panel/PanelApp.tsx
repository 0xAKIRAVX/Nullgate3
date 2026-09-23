"use client";

import { useCallback, useEffect, useState } from "react";
import {
  Globe2, LayoutDashboard, LogOut, Menu, RefreshCcw, Settings, Users2, Waypoints, X,
} from "lucide-react";
import type { ClientRow, InboundInfo, PanelSettings, PanelState } from "@/lib/panel/types";
import { api } from "@/lib/panel/api";
import { useLive } from "@/lib/panel/useLive";
import { Badge, Spinner, ToastHost, useToast } from "./bits";
import LoginView from "./LoginView";
import Logo from "./Logo";
import DashboardView from "./DashboardView";
import UsersView from "./UsersView";
import InboundsView from "./InboundsView";
import SettingsView from "./SettingsView";

type View = "dashboard" | "users" | "inbounds" | "settings";
type Boot =
  | { phase: "loading" }
  | { phase: "setup" }
  | { phase: "login" }
  | { phase: "ready"; username: string };

export default function PanelApp() {
  return (
    <ToastHost>
      <Inner />
    </ToastHost>
  );
}

function Inner() {
  const toast = useToast();
  const [boot, setBoot] = useState<Boot>({ phase: "loading" });
  const [view, setView] = useState<View>("dashboard");
  const [navOpen, setNavOpen] = useState(false);
  const [st, setSt] = useState<PanelState | null>(null);
  const [clients, setClients] = useState<ClientRow[]>([]);
  const [inbounds, setInbounds] = useState<{ builtin: InboundInfo[]; custom: InboundInfo[] } | null>(null);
  const [settings, setSettings] = useState<PanelSettings | null>(null);
  const [busy, setBusy] = useState(true);
  const live = useLive(st);

  // ── boot: decide setup vs login vs panel ────────────────────────────
  const bootNow = useCallback(async () => {
    try {
      const ss = await api.setupStatus();
      if (ss.needs_setup) {
        setBoot({ phase: "setup" });
        return;
      }
      const me = await api.me();
      setBoot({ phase: "ready", username: me.username });
    } catch {
      setBoot({ phase: "login" });
    }
  }, []);

  useEffect(() => { void bootNow(); }, [bootNow]);

  // ── data loaders ────────────────────────────────────────────────────
  const loadState = useCallback(async () => {
    try { setSt(await api.state()); } catch { /* auth guard in boot */ }
  }, []);

  const loadAll = useCallback(async () => {
    setBusy(true);
    try {
      const [s, c, i, se] = await Promise.allSettled([api.state(), api.clients(), api.inbounds(), api.settings()]);
      if (s.status === "fulfilled") setSt(s.value);
      if (c.status === "fulfilled") setClients(c.value);
      if (i.status === "fulfilled") setInbounds(i.value);
      if (se.status === "fulfilled") setSettings(se.value);
    } finally {
      setBusy(false);
    }
  }, []);

  useEffect(() => {
    if (boot.phase === "ready") void loadAll();
  }, [boot.phase, loadAll]);

  const logout = async () => {
    try { await api.logout(); } catch { /* ignore */ }
    setSt(null); setClients([]); setInbounds(null); setSettings(null);
    setBoot({ phase: "login" });
  };

  if (boot.phase === "loading") {
    return (
      <main className="min-h-screen grid place-items-center">
        <div className="flex flex-col items-center gap-4">
          { }
          <Logo className="size-14 drop-shadow-[0_4px_16px_rgba(217,180,74,.35)]" />
          <Spinner className="size-6" />
        </div>
      </main>
    );
  }

  if (boot.phase === "setup" || boot.phase === "login") {
    return (
      <LoginView
        mode={boot.phase}
        onDone={async (username) => {
          setBoot({ phase: "ready", username });
        }}
      />
    );
  }

  // ── shell ───────────────────────────────────────────────────────────
  const NAV: { id: View; label: string; icon: React.ReactNode }[] = [
    { id: "dashboard", label: "داشبورد", icon: <LayoutDashboard className="size-4.5" /> },
    { id: "users", label: "کاربران", icon: <Users2 className="size-4.5" /> },
    { id: "inbounds", label: "اینباندها", icon: <Globe2 className="size-4.5" /> },
    { id: "settings", label: "تنظیمات", icon: <Settings className="size-4.5" /> },
  ];

  const NavList = ({ onNavigate }: { onNavigate?: () => void }) => (
    <nav className="flex flex-col gap-1" aria-label="ناوبری اصلی">
      {NAV.map((n) => (
        <button
          key={n.id}
          onClick={() => { setView(n.id); onNavigate?.(); }}
          aria-current={view === n.id ? "page" : undefined}
          className={`flex items-center gap-3 rounded-xl px-4 py-3 text-[13.5px] font-semibold transition ${
            view === n.id
              ? "bg-gradient-to-l from-gold/20 to-gold/5 text-gtx border border-gold/25"
              : "text-mu hover:text-tx hover:bg-white/[.04] border border-transparent"
          }`}
        >
          {n.icon}
          {n.label}
          {n.id === "users" && clients.length > 0 ? (
            <span className="mr-auto text-[10.5px] rounded-full bg-white/6 px-2 py-0.5">{clients.length.toLocaleString("fa-IR")}</span>
          ) : null}
        </button>
      ))}
    </nav>
  );

  return (
    <div className="min-h-screen flex flex-col lg:flex-row">
      {/* sidebar (desktop) */}
      <aside className="hidden lg:flex w-[248px] shrink-0 flex-col border-l border-line bg-ink/60 px-4 py-5 sticky top-0 h-screen">
        <div className="flex items-center gap-3 px-2 pb-5 border-b border-line">
          { }
          <Logo className="size-10" />
          <div>
            <p className="font-bold text-[14px] gold-text leading-none">NullGate</p>
            <p className="text-[10.5px] text-mu mt-1">پنل مدیریت — نسل ۳.۰</p>
          </div>
        </div>
        <div className="mt-5 flex-1"><NavList /></div>
        <div className="border-t border-line pt-4 space-y-2">
          <div className="flex items-center gap-2.5 px-2">
            <span className="size-8 rounded-full bg-gradient-to-b from-gold2 to-goldd grid place-items-center text-black text-[12px] font-bold">
              {boot.username.slice(0, 1).toUpperCase()}
            </span>
            <div className="min-w-0">
              <p className="text-[12.5px] font-semibold truncate" dir="ltr">{boot.username}</p>
              <p className="text-[10px] text-mu">مدیر سیستم</p>
            </div>
          </div>
          <button onClick={logout} className="ng-btn ng-btn-ghost w-full !text-[12.5px] !text-bad/90 hover:!text-bad">
            <LogOut className="size-4" /> خروج از حساب
          </button>
        </div>
      </aside>

      {/* main */}
      <div className="flex-1 min-w-0 flex flex-col">
        {/* topbar */}
        <header className="sticky top-0 z-40 flex items-center gap-3 px-4 sm:px-6 h-16 border-b border-line bg-ngbg/85 backdrop-blur-md">
          <button className="lg:hidden p-2 rounded-lg hover:bg-white/5" onClick={() => setNavOpen(true)} aria-label="باز کردن منو">
            <Menu className="size-5" />
          </button>
          { }
          <Logo className="size-8 lg:hidden" />
          <div className="flex-1 min-w-0">
            <h2 className="font-bold text-[14.5px] truncate">
              {view === "dashboard" ? "داشبورد" : view === "users" ? "مدیریت کاربران" : view === "inbounds" ? "اینباندها و مسیرها" : "تنظیمات پنل"}
            </h2>
          </div>
          <Badge tone={live.xray.running ? "ok" : "bad"}>
            <span className={`size-1.5 rounded-full bg-current ${live.xray.running ? "live-dot" : ""}`} />
            Xray {live.xray.running ? "فعال" : "خاموش"}
          </Badge>
          <button
            className="p-2 rounded-lg hover:bg-white/5 text-mu hover:text-tx transition" title="بازخوانی داده‌ها" aria-label="بازخوانی"
            onClick={async () => { await loadAll(); toast("ok", "داده‌ها بازخوانی شد"); }}
          >
            <RefreshCcw className="size-4.5" />
          </button>
          <span className="hidden sm:inline-flex size-9 rounded-full bg-gradient-to-b from-gold2 to-goldd grid place-items-center text-black text-[13px] font-bold place-items-center">
            {boot.username.slice(0, 1).toUpperCase()}
          </span>
        </header>

        <main className="flex-1 px-4 sm:px-6 py-5 pb-24 lg:pb-8 max-w-[1400px] w-full mx-auto">
          {view === "dashboard" && st ? (
            <DashboardView st={st} live={live} clients={clients} onGoUsers={() => setView("users")} />
          ) : null}
          {view === "users" ? <UsersView clients={clients} reload={loadAll} busy={busy} /> : null}
          {view === "inbounds" ? <InboundsView data={inbounds} reload={loadAll} /> : null}
          {view === "settings" ? <SettingsView settings={settings} reload={loadAll} /> : null}
          {!st && view === "dashboard" ? <div className="py-20 text-center"><Spinner className="size-7" /></div> : null}
        </main>

        <footer className="hidden lg:block border-t border-line px-6 py-3.5 text-[11px] text-mu flex justify-between items-center mt-auto">
          <span>© ۲۰۲۶ NullGate — ساخته‌شده با Next.js + Go + PostgreSQL</span>
          <span className="flex items-center gap-1.5">اتصال امن، بدون محدودیت <span className="text-gtx">◆</span></span>
        </footer>
      </div>

      {/* mobile nav drawer */}
      {navOpen ? (
        <div className="fixed inset-0 z-[70] lg:hidden" role="dialog" aria-modal="true">
          <div className="absolute inset-0 bg-black/70 backdrop-blur-sm" onClick={() => setNavOpen(false)} />
          <div className="absolute inset-y-0 right-0 w-[270px] bg-card border-l border-line p-5 fade-up">
            <div className="flex items-center justify-between mb-5">
              <div className="flex items-center gap-2.5">
                { }
                <Logo className="size-9" />
                <p className="font-bold gold-text text-[14px]">NullGate ۳.۰</p>
              </div>
              <button onClick={() => setNavOpen(false)} className="p-2 rounded-lg hover:bg-white/5" aria-label="بستن منو">
                <X className="size-4.5" />
              </button>
            </div>
            <NavList onNavigate={() => setNavOpen(false)} />
            <button onClick={logout} className="ng-btn ng-btn-ghost w-full !text-[12.5px] !text-bad/90 mt-6">
              <LogOut className="size-4" /> خروج از حساب
            </button>
          </div>
        </div>
      ) : null}

      {/* mobile bottom nav */}
      <nav className="lg:hidden fixed bottom-0 inset-x-0 z-40 border-t border-line bg-ngbg/95 backdrop-blur-md flex" aria-label="ناوبری موبایل">
        {NAV.map((n) => (
          <button
            key={n.id}
            onClick={() => setView(n.id)}
            className={`flex-1 flex flex-col items-center gap-1 py-2.5 text-[10px] font-medium transition ${view === n.id ? "text-gtx" : "text-mu"}`}
          >
            {n.icon}
            {n.label}
          </button>
        ))}
      </nav>
    </div>
  );
}
