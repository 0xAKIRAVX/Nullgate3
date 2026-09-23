"use client";

import { useEffect, useRef, useState } from "react";
import {
  Download, FileJson, Power, Save, ScrollText, ShieldCheck,
} from "lucide-react";
import type { PanelSettings } from "@/lib/panel/types";
import { PROTOCOL_KEYS, PROTOCOL_LABELS } from "@/lib/panel/types";
import { api } from "@/lib/panel/api";
import { Badge, Field, Spinner, useToast } from "./bits";

export default function SettingsView({
  settings, reload,
}: {
  settings: PanelSettings | null;
  reload: () => Promise<void>;
}) {
  const toast = useToast();
  const [form, setForm] = useState<PanelSettings | null>(settings);
  const [busy, setBusy] = useState(false);
  const [logs, setLogs] = useState<string[] | null>(null);
  const [logBusy, setLogBusy] = useState(false);
  const logBox = useRef<HTMLPreElement>(null);

  useEffect(() => setForm(settings), [settings]);

  const save = async () => {
    if (!form) return;
    setBusy(true);
    try {
      await api.putSettings({
        address: form.address,
        reality_sni: form.reality_sni,
        cfg_fmt: form.cfg_fmt,
        protocols: form.protocols,
      });
      toast("ok", "تنظیمات ذخیره شد — لینک‌های جدید از الان اعمال می‌شوند");
      await reload();
    } catch (e) {
      toast("err", e instanceof Error ? e.message : "خطا در ذخیره");
    } finally {
      setBusy(false);
    }
  };

  const restart = async () => {
    setBusy(true);
    try {
      await api.restart();
      toast("ok", "Xray با کانفیگ جدید ری‌استارت شد");
    } catch (e) {
      toast("err", e instanceof Error ? e.message : "خطا در ری‌استارت");
    } finally {
      setBusy(false);
    }
  };

  const loadLogs = async () => {
    setLogBusy(true);
    try {
      const r = await api.logs();
      setLogs(r.lines);
      requestAnimationFrame(() => {
        if (logBox.current) logBox.current.scrollTop = logBox.current.scrollHeight;
      });
    } catch (e) {
      toast("err", e instanceof Error ? e.message : "خطا در دریافت لاگ");
    } finally {
      setLogBusy(false);
    }
  };

  if (!form) return <div className="py-16 text-center"><Spinner /></div>;

  return (
    <div className="space-y-4">
      <div>
        <h1 className="text-xl font-bold gold-text">تنظیمات</h1>
        <p className="text-[11.5px] text-mu mt-1">این مقادیر در PostgreSQL ذخیره می‌شوند و با Redeploy تغییر نمی‌کنند</p>
      </div>

      <div className="grid lg:grid-cols-2 gap-3 sm:gap-4">
        <div className="ng-card p-5 space-y-4">
          <h2 className="font-bold text-[15px] flex items-center gap-2">
            <ShieldCheck className="size-4.5 text-gtx" /> اتصال و Reality
          </h2>
          <Field label="آدرس سرور (اختیاری)" hint="خالی = دامنه‌ی درخواست به‌صورت خودکار استفاده می‌شود. برای Railway دامنه سرویس api را بگذارید.">
            <input
              className="ng-input" dir="ltr" placeholder="nullgate-api.up.railway.app"
              value={form.address} onChange={(e) => setForm({ ...form, address: e.target.value })}
            />
          </Field>
          <Field label="SNI پیش‌فرض Reality" hint="دامنه‌ای که handshake TLS به آن جعل می‌شود — پیشنهادی: www.samsung.com (سازگار با REALITY)">
            <input
              className="ng-input" dir="ltr" placeholder="www.samsung.com"
              value={form.reality_sni} onChange={(e) => setForm({ ...form, reality_sni: e.target.value })}
            />
          </Field>
          <Field label="قالب نام کانفیگ" hint="متغیرها: {name} {label} — مثلاً {name}-{label}">
            <input
              className="ng-input" dir="ltr" placeholder="{name}-{label}"
              value={form.cfg_fmt} onChange={(e) => setForm({ ...form, cfg_fmt: e.target.value })}
            />
          </Field>
          <div className="flex items-center justify-between ng-card !rounded-xl px-3.5 py-3">
            <div>
              <p className="text-[12.5px] font-semibold">عمر نشست ورود</p>
              <p className="text-[10.5px] text-mu mt-0.5">از متغیر محیطی SESSION_HOURS خوانده می‌شود</p>
            </div>
            <Badge tone="gold">{(form.session_hours ?? 24).toLocaleString("fa-IR")} ساعت</Badge>
          </div>
        </div>

        <div className="ng-card p-5">
          <h2 className="font-bold text-[15px]">پروتکل‌های سرور</h2>
          <p className="text-[11px] text-mu mt-0.5 mb-4">پروتکل‌های خاموش از لینک همه کاربران حذف می‌شوند (اشتراک‌ها خودکار به‌روز می‌شوند)</p>
          <div className="space-y-2">
            {PROTOCOL_KEYS.map((k) => {
              const on = form.protocols?.[k] !== false;
              return (
                <label key={k} className={`flex items-center justify-between gap-3 rounded-xl border px-3.5 py-3 cursor-pointer transition ${on ? "border-gold/25 bg-gold/6" : "border-line bg-white/[.02]"}`}>
                  <span>
                    <span className="text-[13px] font-semibold">{PROTOCOL_LABELS[k]}</span>
                    <span className="block text-[10px] text-mu mono mt-0.5" dir="ltr">{k}</span>
                  </span>
                  <input
                    type="checkbox" className="ng-check size-4.5"
                    checked={on}
                    onChange={(e) => setForm({ ...form, protocols: { ...form.protocols, [k]: e.target.checked } })}
                  />
                </label>
              );
            })}
          </div>
        </div>
      </div>

      <div className="flex flex-wrap gap-2.5">
        <button className="ng-btn ng-btn-gold" disabled={busy} onClick={save}>
          {busy ? <Spinner className="size-4" /> : <Save className="size-4" />} ذخیره تنظیمات
        </button>
        <button className="ng-btn ng-btn-ghost" disabled={busy} onClick={restart}>
          <Power className="size-4" /> ری‌استارت Xray
        </button>
        <a className="ng-btn ng-btn-ghost" href="/api/server-config" download="xray-server.json">
          <FileJson className="size-4" /> دانلود کانفیگ سرور
        </a>
      </div>

      <div className="ng-card p-5">
        <div className="flex items-center justify-between gap-3 flex-wrap mb-3">
          <h2 className="font-bold text-[15px] flex items-center gap-2"><ScrollText className="size-4.5 text-gtx" /> لاگ زنده Xray</h2>
          <button className="ng-btn ng-btn-ghost !py-1.5 !px-3 !text-xs" disabled={logBusy} onClick={loadLogs}>
            {logBusy ? <Spinner className="size-3.5" /> : <Download className="size-3.5 rotate-180" />} دریافت آخرین ۱۵۰ خط
          </button>
        </div>
        <pre
          ref={logBox}
          className="mono text-[10.5px] leading-relaxed bg-black/45 border border-line rounded-xl p-3.5 max-h-72 overflow-y-auto whitespace-pre-wrap"
          dir="ltr"
        >
          {logs === null ? "برای مشاهده، دکمه دریافت را بزنید…" : logs.join("\n") || "— خالی —"}
        </pre>
      </div>
    </div>
  );
}
