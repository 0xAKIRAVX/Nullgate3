"use client";

import { useState } from "react";
import { Eye, EyeOff, Loader2, LogIn, ShieldCheck, Sparkles, UserRound } from "lucide-react";
import { api } from "@/lib/panel/api";
import { useToast } from "./bits";
import Logo from "./Logo";

export default function LoginView({ mode, onDone }: { mode: "login" | "setup"; onDone: (username: string) => void }) {
  const toast = useToast();
  const [u, setU] = useState("");
  const [p, setP] = useState("");
  const [p2, setP2] = useState("");
  const [show, setShow] = useState(false);
  const [busy, setBusy] = useState(false);

  const submit = async (e: React.FormEvent) => {
    e.preventDefault();
    if (mode === "setup" && p !== p2) {
      toast("err", "تکرار رمز عبور مطابقت ندارد");
      return;
    }
    setBusy(true);
    try {
      if (mode === "setup") {
        await api.setup(u.trim(), p);
        toast("ok", "حساب مدیر ساخته شد — خوش آمدید");
      } else {
        await api.login(u.trim(), p);
        toast("ok", "خوش آمدید");
      }
      onDone(u.trim());
    } catch (err) {
      toast("err", err instanceof Error ? err.message : "خطای نامشخص");
    } finally {
      setBusy(false);
    }
  };

  return (
    <main
      className="relative min-h-screen flex flex-col items-center justify-center px-4 py-10 overflow-hidden"
      style={{
        backgroundImage: "linear-gradient(rgba(5,5,8,.62), rgba(7,7,11,.86)), url(/login-bg.jpg)",
        backgroundSize: "cover",
        backgroundPosition: "center",
      }}
    >
      {/* ambient top glow */}
      <div className="pointer-events-none absolute -top-32 left-1/2 -translate-x-1/2 size-[420px] rounded-full bg-gold/10 blur-[110px]" />
      <div className="pointer-events-none absolute bottom-0 right-0 size-[260px] rounded-full bg-gold/6 blur-[90px]" />

      {/* brand slogan — top corner */}
      <div className="absolute top-6 right-6 text-right fade-up">
        <p className="text-[11px] text-gtx/70 font-medium tracking-wide">برای آزادی</p>
        <p className="text-xl font-bold gold-text leading-snug">یک قدم جلوتر</p>
        <p className="text-[10px] text-mu mt-0.5">اتصال امن، بدون محدودیت</p>
      </div>

      <div className="relative w-full max-w-[400px] fade-up">
        {/* gold gradient border wrapper */}
        <div className="rounded-[26px] p-px bg-gradient-to-b from-gold/45 via-gold/12 to-transparent shadow-[0_24px_70px_-18px_rgba(0,0,0,.85)]">
          <div className="ng-card rounded-[25px] bg-black/62 backdrop-blur-2xl px-7 py-9 sm:px-9 relative overflow-hidden">
            {/* inner top sheen */}
            <div className="pointer-events-none absolute inset-x-10 -top-px h-px bg-gradient-to-r from-transparent via-gold/70 to-transparent" />

            <div className="flex flex-col items-center text-center">
              {/* logo with orbit ring */}
              <div className="relative grid place-items-center">
                <span className="logo-ring absolute size-[104px] rounded-full border border-gold/20" />
                <span className="absolute size-[86px] rounded-full border border-gold/10" />
                <Logo className="size-[72px] animate-float drop-shadow-[0_6px_22px_rgba(217,180,74,.4)]" />
              </div>

              <h1 className="mt-4 text-[15px] font-bold tracking-[.42em] gold-text" dir="ltr">NULLGATE</h1>
              <div className="mt-2.5 flex items-center gap-2 text-mu/80">
                <span className="h-px w-9 bg-gradient-to-l from-gold/50 to-transparent" />
                <Sparkles className="size-3.5 text-gold/70" />
                <span className="h-px w-9 bg-gradient-to-r from-gold/50 to-transparent" />
              </div>
              <p className="mt-2.5 text-[13.5px] font-semibold text-tx/95">
                {mode === "setup" ? "راه‌اندازی اولیه پنل" : "ورود به پنل کاربری"}
              </p>
              <p className="mt-1.5 text-[11px] text-mu leading-relaxed max-w-[270px]">
                {mode === "setup"
                  ? "اولین حساب مدیر را بسازید — این فرم فقط یک بار نمایش داده می‌شود"
                  : "به دشبورد مدیریت خوش آمدید"}
              </p>
            </div>

            <form onSubmit={submit} className="mt-6 space-y-3.5">
              <div className="relative">
                <input
                  className="ng-input pl-10 h-11" dir="ltr" autoComplete="username" required
                  placeholder="نام کاربری" value={u} onChange={(e) => setU(e.target.value)} minLength={3}
                />
                <UserRound className="absolute left-3 top-1/2 -translate-y-1/2 size-4 text-mu/60" />
              </div>
              <div className="relative">
                <input
                  className="ng-input pl-10 h-11" dir="ltr" type={show ? "text" : "password"}
                  autoComplete={mode === "setup" ? "new-password" : "current-password"}
                  required placeholder="رمز عبور" value={p} onChange={(e) => setP(e.target.value)} minLength={6}
                />
                <button type="button" onClick={() => setShow((s) => !s)} aria-label="نمایش رمز"
                  className="absolute left-3 top-1/2 -translate-y-1/2 text-mu/60 hover:text-tx transition">
                  {show ? <EyeOff className="size-4" /> : <Eye className="size-4" />}
                </button>
              </div>
              {mode === "setup" && (
                <div className="relative">
                  <input
                    className="ng-input pl-10 h-11" dir="ltr" type={show ? "text" : "password"}
                    required placeholder="تکرار رمز عبور" value={p2} onChange={(e) => setP2(e.target.value)} minLength={6}
                  />
                  <ShieldCheck className="absolute left-3 top-1/2 -translate-y-1/2 size-4 text-mu/50" />
                </div>
              )}

              <button
                type="submit" disabled={busy || u.length < 3 || p.length < 6}
                className="ng-btn ng-btn-gold btn-sheen w-full !py-3 !text-[15px] !rounded-xl mt-1 disabled:opacity-45"
              >
                {busy ? <Loader2 className="size-4.5 spin" /> : mode === "setup" ? <ShieldCheck className="size-4.5" /> : <LogIn className="size-4.5" />}
                {mode === "setup" ? "ساخت حساب مدیر" : "ورود"}
              </button>
            </form>

            {mode === "login" && (
              <div className="mt-6 flex flex-wrap items-center justify-center gap-2 text-[10.5px] text-mu">
                <span className="rounded-full border border-gold/25 bg-gold/8 px-2.5 py-1 text-gtx">Go + Xray</span>
                <span className="rounded-full border border-gold/25 bg-gold/8 px-2.5 py-1 text-gtx">PostgreSQL</span>
                <span className="rounded-full border border-gold/25 bg-gold/8 px-2.5 py-1 text-gtx">زنده WebSocket</span>
              </div>
            )}
          </div>
        </div>
      </div>

      <p className="relative mt-8 text-center text-sm font-bold gold-text fade-up">قدرت در اتصال است</p>
      <p className="relative text-center text-[11px] text-mu mt-1">ایران، همیشه متصل</p>
    </main>
  );
}
