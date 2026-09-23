"use client";

import { useState } from "react";
import { Globe, Layers, Plus, ShieldCheck, Trash2, TriangleAlert, Waypoints } from "lucide-react";
import type { InboundInfo } from "@/lib/panel/types";
import { api } from "@/lib/panel/api";
import { Badge, Field, Modal, Spinner, useToast } from "./bits";

const NETS = ["ws", "xhttp", "httpupgrade", "grpc", "tcp"] as const;
const PROTOS = ["vless", "vmess", "trojan"] as const;

export default function InboundsView({
  data, reload,
}: {
  data: { builtin: InboundInfo[]; custom: InboundInfo[] } | null;
  reload: () => Promise<void>;
}) {
  const toast = useToast();
  const [open, setOpen] = useState(false);
  const [del, setDel] = useState<InboundInfo | null>(null);
  const [busy, setBusy] = useState(false);

  const doDelete = async (ib: InboundInfo) => {
    setBusy(true);
    try {
      await api.deleteInbound(ib.tag);
      toast("ok", `اینباند «${ib.name}» حذف شد`);
      setDel(null);
      await reload();
    } catch (e) {
      toast("err", e instanceof Error ? e.message : "خطا در حذف");
    } finally {
      setBusy(false);
    }
  };

  return (
    <div className="space-y-4">
      <div className="flex items-center justify-between gap-3 flex-wrap">
        <div>
          <h1 className="text-xl font-bold gold-text">اینباندها</h1>
          <p className="text-[11.5px] text-mu mt-1">۶ اینباند داخلی همیشه فعال + اینباندهای سفارشی روی دامنه و پورت‌های پنل</p>
        </div>
        <button className="ng-btn ng-btn-gold" onClick={() => setOpen(true)}>
          <Plus className="size-4" /> اینباند سفارشی
        </button>
      </div>

      {!data ? (
        <div className="py-16 text-center"><Spinner /></div>
      ) : (
        <div className="grid md:grid-cols-2 xl:grid-cols-3 gap-3">
          {data.builtin.map((ib) => (
            <div key={ib.tag} className="ng-card-gold p-4">
              <div className="flex items-start justify-between gap-2">
                <div className="flex items-center gap-2.5 min-w-0">
                  <span className="size-10 rounded-xl bg-gold/12 border border-gold/25 grid place-items-center shrink-0">
                    <ShieldCheck className="size-5 text-gtx" />
                  </span>
                  <div className="min-w-0">
                    <p className="font-bold text-[13.5px] truncate">{ib.name}</p>
                    <p className="text-[10.5px] text-mu mono" dir="ltr">{ib.protocol}/{ib.network}</p>
                  </div>
                </div>
                <Badge tone="gold">داخلی</Badge>
              </div>
              <dl className="mt-3 space-y-1.5 text-[11px]">
                <Row k="مسیر" v={ib.path ?? "—"} mono />
                {ib.port ? <Row k="پورت" v={String(ib.port)} mono /> : null}
              </dl>
            </div>
          ))}
          {data.custom.map((ib) => (
            <div key={ib.tag} className="ng-card p-4">
              <div className="flex items-start justify-between gap-2">
                <div className="flex items-center gap-2.5 min-w-0">
                  <span className="size-10 rounded-xl bg-white/4 border border-line grid place-items-center shrink-0">
                    <Waypoints className="size-5 text-mu" />
                  </span>
                  <div className="min-w-0">
                    <p className="font-bold text-[13.5px] truncate">{ib.name}</p>
                    <p className="text-[10.5px] text-mu mono" dir="ltr">{ib.protocol}/{ib.network} + {ib.security}</p>
                  </div>
                </div>
                <button
                  className="p-1.5 rounded-lg text-bad/70 hover:text-bad hover:bg-bad/10 transition" title="حذف اینباند"
                  onClick={() => setDel(ib)}
                >
                  <Trash2 className="size-4" />
                </button>
              </div>
              <dl className="mt-3 space-y-1.5 text-[11px]">
                <Row k="مسیر / سرویس" v={ib.path || ib.svc || "—"} mono />
                <Row k="پورت" v={ib.port ? String(ib.port) : "خودکار (TLS دامنه)"} mono />
                {ib.sni ? <Row k="SNI" v={ib.sni} mono /> : null}
              </dl>
              {ib.security === "reality" && ib.reachable === false && (
                <div className="mt-3 rounded-lg border border-bad/30 bg-bad/8 p-2.5 text-[10.5px] leading-relaxed text-bad flex gap-2">
                  <TriangleAlert className="size-4 shrink-0 mt-0.5" />
                  <span>
                    روی Railway بدون TCP Proxy از بیرون در دسترس نیست. یک TCP Proxy جدید روی پورت داخلی <b className="mono">{ib.port}</b> بسازید و متغیرهای <code className="mono">TCP2_HOST</code> / <code className="mono">TCP2_PORT</code> را با همان آدرس پر کنید.
                  </span>
                </div>
              )}
            </div>
          ))}
          {data.custom.length === 0 && (
            <div className="ng-card p-6 md:col-span-2 xl:col-span-1 flex flex-col items-center justify-center text-center gap-2 min-h-[140px]">
              <Layers className="size-6 text-mu/50" />
              <p className="text-[12.5px] text-mu">اینباند سفارشی ندارید</p>
              <p className="text-[10.5px] text-mu/70">برای مسیر دوم روی ۴۴۳ یا پروتکل ترکیبی جدید بسازید</p>
            </div>
          )}
        </div>
      )}

      <CreateInboundModal
        open={open}
        onClose={() => setOpen(false)}
        onSaved={async (msg) => { setOpen(false); toast("ok", msg); await reload(); }}
      />

      <Modal open={!!del} onClose={() => setDel(null)} title="حذف اینباند سفارشی">
        <p className="text-[13.5px] leading-relaxed">
          اینباند <b className="text-gtx">{del?.name}</b> حذف شود؟ کانفیگ‌های متصل به آن با ری‌استارت بعدی Xray از دسترس خارج می‌شوند.
        </p>
        <div className="mt-5 flex gap-2.5 justify-end">
          <button className="ng-btn ng-btn-ghost" onClick={() => setDel(null)}>انصراف</button>
          <button className="ng-btn ng-btn-danger" disabled={busy} onClick={() => del && doDelete(del)}>
            {busy ? <Spinner className="size-4" /> : <Trash2 className="size-4" />} حذف
          </button>
        </div>
      </Modal>
    </div>
  );
}

function Row({ k, v, mono }: { k: string; v: string; mono?: boolean }) {
  return (
    <div className="flex items-center justify-between gap-2">
      <dt className="text-mu">{k}</dt>
      <dd className={mono ? "mono text-[10.5px] truncate max-w-[170px]" : "text-[11px]"} dir={mono ? "ltr" : "auto"}>{v}</dd>
    </div>
  );
}

function CreateInboundModal({
  open, onClose, onSaved,
}: {
  open: boolean; onClose: () => void; onSaved: (msg: string) => Promise<void>;
}) {
  const toast = useToast();
  const [busy, setBusy] = useState(false);
  const [d, setD] = useState({
    name: "", protocol: "vless", network: "ws", security: "tls", port: "", path: "", sni: "",
  });

  const submit = async () => {
    setBusy(true);
    try {
      await api.createInbound({
        name: d.name.trim(),
        protocol: d.protocol,
        network: d.network,
        security: d.security,
        port: d.security === "reality" ? Number(d.port) : undefined,
        path: d.path || undefined,
        sni: d.security === "reality" ? d.sni || undefined : undefined,
      });
      await onSaved(`اینباند «${d.name.trim()}» ساخته شد و Xray ری‌استارت شد`);
      setD({ name: "", protocol: "vless", network: "ws", security: "tls", port: "", path: "", sni: "" });
    } catch (e) {
      toast("err", e instanceof Error ? e.message : "خطا در ساخت اینباند");
    } finally {
      setBusy(false);
    }
  };

  return (
    <Modal open={open} onClose={onClose} title="اینباند سفارشی جدید">
      <div className="space-y-4">
        <Field label="نام نمایشی">
          <input className="ng-input" placeholder="مثلاً «مسیر ۴۴۳»" value={d.name} onChange={(e) => setD({ ...d, name: e.target.value })} />
        </Field>
        <div className="grid grid-cols-3 gap-3">
          <Field label="پروتکل">
            <select className="ng-input" value={d.protocol} onChange={(e) => setD({ ...d, protocol: e.target.value })}>
              {PROTOS.map((p) => <option key={p} value={p} className="bg-card">{p.toUpperCase()}</option>)}
            </select>
          </Field>
          <Field label="ترنسپورت">
            <select className="ng-input" value={d.network} onChange={(e) => setD({ ...d, network: e.target.value })}>
              {NETS.map((n) => <option key={n} value={n} className="bg-card">{n}</option>)}
            </select>
          </Field>
          <Field label="امنیت">
            <select className="ng-input" value={d.security} onChange={(e) => setD({ ...d, security: e.target.value })}>
              <option value="tls" className="bg-card">TLS</option>
              <option value="reality" className="bg-card">Reality</option>
            </select>
          </Field>
        </div>

        {d.security === "reality" ? (
          <>
            <div className="ng-card-gold p-3 text-[11px] text-gtx leading-relaxed flex gap-2">
              <ShieldCheck className="size-4 shrink-0 mt-0.5" />
              <span>
                Reality فقط با VLESS و ترنسپورت TCP/XHTTP/gRPC کار می‌کند. عددی که در «پورت عمومی» می‌نویسید همان پورت داخلی Xray است؛ برای دسترسی از بیرون باید برایش یک TCP Proxy جدا بسازید و آدرسش را در <code className="mono">TCP2_HOST</code> / <code className="mono">TCP2_PORT</code> بگذارید. لینک‌ها خودکار درست می‌شوند.
              </span>
            </div>
            <div className="grid grid-cols-2 gap-3">
              <Field label="پورت عمومی" hint="۱۰۲۴ تا ۶۵۵۳۵ — بدون تداخل">
                <input className="ng-input" dir="ltr" inputMode="numeric" placeholder="443" value={d.port} onChange={(e) => setD({ ...d, port: e.target.value })} />
              </Field>
              <Field label="SNI" hint="خالی = از تنظیمات پنل">
                <input className="ng-input" dir="ltr" placeholder="www.samsung.com" value={d.sni} onChange={(e) => setD({ ...d, sni: e.target.value })} />
              </Field>
            </div>
          </>
        ) : (
          <Field label="مسیر (Path)" hint="برای ws/xhttp/httpupgrade — grpc = Service name؛ خالی = تصادفی امن">
            <input className="ng-input" dir="ltr" placeholder="/mypath" value={d.path} onChange={(e) => setD({ ...d, path: e.target.value })} />
          </Field>
        )}

        <p className="text-[10.5px] text-mu flex items-center gap-1.5">
          <Globe className="size-3.5" /> مسیرهای رزرو‌شده: <code className="mono">/panel</code>، <code className="mono">/api</code>، <code className="mono">/sub</code> و مسیرهای اینباندهای داخلی
        </p>

        <div className="flex gap-2.5 justify-end pt-1">
          <button className="ng-btn ng-btn-ghost" onClick={onClose} type="button">انصراف</button>
          <button className="ng-btn ng-btn-gold" disabled={busy || !d.name.trim()} onClick={submit} type="button">
            {busy ? <Spinner className="size-4" /> : <Plus className="size-4" />} ساخت و ری‌استارت
          </button>
        </div>
      </div>
    </Modal>
  );
}
