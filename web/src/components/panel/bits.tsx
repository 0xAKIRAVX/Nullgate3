"use client";

import { createContext, useCallback, useContext, useEffect, useRef, useState } from "react";
import QRCode from "qrcode";
import { Check, Copy, X } from "lucide-react";

// ── toasts ────────────────────────────────────────────────────────────
type ToastKind = "ok" | "err" | "info";
interface Toast { id: number; kind: ToastKind; text: string }
const ToastCtx = createContext<(kind: ToastKind, text: string) => void>(() => {});
export const useToast = () => useContext(ToastCtx);

export function ToastHost({ children }: { children: React.ReactNode }) {
  const [list, setList] = useState<Toast[]>([]);
  const seq = useRef(1);
  const push = useCallback((kind: ToastKind, text: string) => {
    const id = seq.current++;
    setList((l) => [...l, { id, kind, text }]);
    setTimeout(() => setList((l) => l.filter((t) => t.id !== id)), 3600);
  }, []);
  return (
    <ToastCtx.Provider value={push}>
      {children}
      <div className="fixed top-4 left-1/2 -translate-x-1/2 z-[90] flex flex-col gap-2 w-[min(92vw,380px)]">
        {list.map((t) => (
          <div
            key={t.id}
            className={`toast-in ng-card px-4 py-2.5 text-sm flex items-center gap-2 shadow-xl backdrop-blur-md ${
              t.kind === "ok"
                ? "border-ok/40 text-ok"
                : t.kind === "err"
                  ? "border-bad/40 text-bad"
                  : "border-gold/40 text-gtx"
            }`}
            role="status"
          >
            <span className="size-2 rounded-full bg-current live-dot" />
            {t.text}
          </div>
        ))}
      </div>
    </ToastCtx.Provider>
  );
}

// ── modal ─────────────────────────────────────────────────────────────
export function Modal({
  open, onClose, title, children, wide,
}: {
  open: boolean; onClose: () => void; title: string; children: React.ReactNode; wide?: boolean;
}) {
  useEffect(() => {
    if (!open) return;
    const onKey = (e: KeyboardEvent) => e.key === "Escape" && onClose();
    window.addEventListener("keydown", onKey);
    document.body.style.overflow = "hidden";
    return () => {
      window.removeEventListener("keydown", onKey);
      document.body.style.overflow = "";
    };
  }, [open, onClose]);
  if (!open) return null;
  return (
    <div className="fixed inset-0 z-[80] flex items-end sm:items-center justify-center p-0 sm:p-6" role="dialog" aria-modal="true">
      <div className="absolute inset-0 bg-black/70 backdrop-blur-sm" onClick={onClose} />
      <div
        className={`relative ng-card bg-card w-full ${wide ? "sm:max-w-2xl" : "sm:max-w-md"} max-h-[92vh] overflow-y-auto rounded-t-2xl sm:rounded-2xl fade-up shadow-2xl`}
      >
        <div className="sticky top-0 z-10 flex items-center justify-between gap-3 px-5 py-4 bg-card/95 backdrop-blur border-b border-line rounded-t-2xl">
          <h3 className="font-bold text-[15px] gold-text">{title}</h3>
          <button onClick={onClose} aria-label="بستن" className="p-1.5 rounded-lg hover:bg-white/5 text-mu hover:text-tx transition">
            <X className="size-4.5" />
          </button>
        </div>
        <div className="p-5">{children}</div>
      </div>
    </div>
  );
}

// ── small building blocks ─────────────────────────────────────────────
export function Badge({ tone, children }: { tone: "ok" | "bad" | "warn" | "gold" | "mu"; children: React.ReactNode }) {
  const map = {
    ok: "bg-ok/12 text-ok border-ok/30",
    bad: "bg-bad/12 text-bad border-bad/30",
    warn: "bg-warn/12 text-warn border-warn/30",
    gold: "bg-gold/12 text-gtx border-gold/30",
    mu: "bg-white/5 text-mu border-line",
  } as const;
  return (
    <span className={`inline-flex items-center gap-1.5 rounded-full border px-2.5 py-0.5 text-[11px] font-medium ${map[tone]}`}>
      {children}
    </span>
  );
}

export function Spinner({ className = "size-5" }: { className?: string }) {
  return (
    <span
      className={`spin inline-block rounded-full border-2 border-gold/30 border-t-gold ${className}`}
      role="progressbar" aria-label="در حال بارگذاری"
    />
  );
}

export function CopyBtn({ text, label, className = "" }: { text: string; label?: string; className?: string }) {
  const [done, setDone] = useState(false);
  const toast = useToast();
  return (
    <button
      onClick={async () => {
        try {
          await navigator.clipboard.writeText(text);
        } catch {
          const ta = document.createElement("textarea");
          ta.value = text; document.body.appendChild(ta); ta.select();
          document.execCommand("copy"); ta.remove();
        }
        setDone(true);
        toast("ok", "کپی شد");
        setTimeout(() => setDone(false), 1500);
      }}
      className={`ng-btn ng-btn-ghost !px-2.5 !py-1.5 !text-xs ${className}`}
      title="کپی"
      type="button"
    >
      {done ? <Check className="size-3.5 text-ok" /> : <Copy className="size-3.5" />}
      {label}
    </button>
  );
}

export function QRImage({ text, size = 168 }: { text: string; size?: number }) {
  const [url, setUrl] = useState<string>("");
  useEffect(() => {
    let dead = false;
    QRCode.toDataURL(text, { width: 512, margin: 1, color: { dark: "#0a0a0b", light: "#f2f2f4" } })
      .then((u) => !dead && setUrl(u))
      .catch(() => {});
    return () => { dead = true; };
  }, [text]);
  if (!url) return <div className="rounded-xl bg-white/5 shimmer" style={{ width: size, height: size }} />;
   
  return <img src={url} alt="QR کانفیگ" width={size} height={size} className="rounded-xl" />;
}

/** tiny sparkline for live traffic deltas */
export function Sparkline({ data, w = 220, h = 44 }: { data: number[]; w?: number; h?: number }) {
  if (data.length < 2) return <div className="h-[44px]" />;
  const max = Math.max(...data, 1);
  const step = w / (data.length - 1);
  const pts = data.map((v, i) => `${(i * step).toFixed(1)},${(h - 3 - (v / max) * (h - 8)).toFixed(1)}`);
  return (
    <svg viewBox={`0 0 ${w} ${h}`} className="w-full h-11 overflow-visible" aria-hidden>
      <defs>
        <linearGradient id="ngspark" x1="0" y1="0" x2="0" y2="1">
          <stop offset="0%" stopColor="#d9b44a" stopOpacity=".35" />
          <stop offset="100%" stopColor="#d9b44a" stopOpacity="0" />
        </linearGradient>
      </defs>
      <polygon points={`0,${h} ${pts.join(" ")} ${w},${h}`} fill="url(#ngspark)" />
      <polyline points={pts.join(" ")} fill="none" stroke="#d9b44a" strokeWidth="1.6" strokeLinejoin="round" strokeLinecap="round" />
    </svg>
  );
}

export function Field({ label, children, hint }: { label: string; children: React.ReactNode; hint?: string }) {
  return (
    <label className="block space-y-1.5">
      <span className="text-xs text-mu font-medium">{label}</span>
      {children}
      {hint ? <span className="block text-[10.5px] text-mu/70 leading-relaxed">{hint}</span> : null}
    </label>
  );
}
