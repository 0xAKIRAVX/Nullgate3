"use client";

import { useState } from "react";
import { Eye, EyeOff, Loader2, LogIn, ShieldCheck, UserRoundPlus } from "lucide-react";
import { api } from "@/lib/panel/api";
import { useToast } from "./bits";

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
        backgroundImage: "linear-gradient(rgba(6,6,8,.55), rgba(8,8,10,.82)), url(/login-bg.jpg)",
        backgroundSize: "cover",
        backgroundPosition: "center",
      }}
    >
      <div className="absolute top-6 right-6 text-right fade-up">
        <p className="text-[13px] text-gtx/90 font-semibold">برای آزادی</p>
        <p className="text-2xl font-bold gold-text leading-snug">یک قدم جلوتر</p>
        <p className="text-[11px] text-mu mt-1">اتصال امن، بدون محدودیت</p>
      </div>

      <div className="relative w-full max-w-[400px] fade-up">
        <div className="ng-card bg-black/55 backdrop-blur-xl border-gold/15 shadow-2xl px-7 py-9 sm:px-9">
          <div className="flex flex-col items-center text-center">
            { }
            <img src="/nullgate-logo.png" alt="NullGate" className="size-16 drop-shadow-[0_4px_16px_rgba(217,180,74,.35)]" />
            <h1 className="mt-3 text-xl font-bold">
              {mode === "setup" ? "راه‌اندازی اولیه پنل" : "ورود به پنل کاربری"}
            </h1>
            <p className="mt-1.5 text-xs text-mu leading-relaxed">
              {mode === "setup"
                ? "اولین حساب مدیر را بسازید — این فرم فقط یک بار نمایش داده می‌شود"
                : "به دشبورد مدیریت خوش آمدید"}
            </p>
          </div>

          <form onSubmit={submit} className="mt-7 space-y-3.5">
            <div className="relative">
              <input
                className="ng-input pl-10" dir="ltr" autoComplete="username" required
                placeholder="نام کاربری" value={u} onChange={(e) => setU(e.target.value)} minLength={3}
              />
              <UserRoundPlus className="absolute left-3 top-1/2 -translate-y-1/2 size-4 text-mu/60" />
            </div>
            <div className="relative">
              <input
                className="ng-input pl-10" dir="ltr" type={show ? "text" : "password"} autoComplete={mode === "setup" ? "new-password" : "current-password"}
                required placeholder="رمز عبور" value={p} onChange={(e) => setP(e.target.value)} minLength={6}
              />
              <button type="button" onClick={() => setShow((s) => !s)} aria-label="نمایش رمز"
                className="absolute left-3 top-1/2 -translate-y-1/2 text-mu/60 hover:text-tx transition">
                {show ? <EyeOff className="size-4" /> : <Eye className="size-4" />}
              </button>
            </div>
            {mode === "setup" && (
              <input
                className="ng-input" dir="ltr" type={show ? "text" : "password"}
                required placeholder="تکرار رمز عبور" value={p2} onChange={(e) => setP2(e.target.value)} minLength={6}
              />
            )}

            <button type="submit" disabled={busy || u.length < 3 || p.length < 6} className="ng-btn ng-btn-gold w-full !py-3 !text-[15px] mt-1">
              {busy ? <Loader2 className="size-4.5 spin" /> : mode === "setup" ? <ShieldCheck className="size-4.5" /> : <LogIn className="size-4.5" />}
              {mode === "setup" ? "ساخت حساب مدیر" : "ورود"}
            </button>
          </form>

          {mode === "login" && (
            <div className="mt-6 flex flex-wrap items-center justify-center gap-2 text-[10.5px] text-mu">
              <span className="rounded-full border border-gold/25 bg-gold/8 px-2.5 py-1 text-gtx">Next.js ۱۶</span>
              <span className="rounded-full border border-gold/25 bg-gold/8 px-2.5 py-1 text-gtx">Go + Xray</span>
              <span className="rounded-full border border-gold/25 bg-gold/8 px-2.5 py-1 text-gtx">PostgreSQL</span>
              <span className="rounded-full border border-gold/25 bg-gold/8 px-2.5 py-1 text-gtx">زنده WebSocket</span>
            </div>
          )}
        </div>
      </div>

      <p className="relative mt-8 text-center text-sm font-bold gold-text fade-up">قدرت در اتصال است</p>
      <p className="relative text-center text-[11px] text-mu mt-1">ایران، همیشیه متصل</p>
    </main>
  );
}
