"use client";

import { useMemo, useRef, useState } from "react";
import {
  BadgeCheck, CalendarClock, Download, Infinity as InfinityIcon, Link2, Pencil,
  Plus, QrCode, Search, Trash2, UserRoundPlus, Waypoints,
} from "lucide-react";
import type { ClientRow, InboundInfo, PanelSettings, ProtocolKey, ShareLink } from "@/lib/panel/types";
import { PROTOCOL_KEYS, PROTOCOL_LABELS } from "@/lib/panel/types";
import { api, downloadClientJSON, subURL } from "@/lib/panel/api";
import { bytesFmt, expireLabel, jalali, linkHost, linkLabel, pct } from "@/lib/panel/format";
import { Badge, CopyBtn, Field, Modal, QRImage, Spinner, useToast } from "./bits";

type Draft = {
  name: string; quota_gb: string; expire_days: string; note: string;
  protocols: Record<string, boolean>;
  protoMode: "default" | "custom";
};

const emptyDraft = (): Draft => ({
  name: "", quota_gb: "", expire_days: "", note: "",
  protoMode: "default",
  protocols: Object.fromEntries(PROTOCOL_KEYS.map((k) => [k, true])),
});

// Persian (۰-۹) and Arabic-Indic (٠-٩) digits → ASCII: Number() rejects them,
// so "۵۰" in the quota field used to become NaN → 0 (= unlimited) and "۳۰"
// days used to silently WIPE the expiry on edit.
const toEnDigits = (s: string) =>
  s.replace(/[۰-۹]/g, (d) => String("۰۱۲۳۴۵۶۷۸۹".indexOf(d)))
   .replace(/[٠-٩]/g, (d) => String("٠١٢٣٤٥٦٧٨٩".indexOf(d)));

/** "۵۰" | "50" | "50.5" → 50 | 50.5; "" → null (blank); garbage → null */
const parseNum = (s: string): number | null => {
  const t = toEnDigits(s.trim()).replace(/,/g, "");
  if (t === "") return null;
  const n = Number(t);
  return Number.isFinite(n) ? n : null;
};

export default function UsersView({
  clients, reload, busy, customs, settings,
}: {
  clients: ClientRow[];
  reload: () => Promise<unknown>;
  busy: boolean;
  customs: InboundInfo[] | null;
  settings: PanelSettings | null;
}) {
  const toast = useToast();
  const [q, setQ] = useState("");
  const [form, setForm] = useState<{ open: boolean; edit: ClientRow | null }>({ open: false, edit: null });
  const [linksOf, setLinksOf] = useState<ClientRow | null>(null);
  const [links, setLinks] = useState<ShareLink[] | null>(null);
  const [del, setDel] = useState<ClientRow | null>(null);
  const linksReqId = useRef(0);

  const filtered = useMemo(
    () => clients.filter((c) => c.name.toLowerCase().includes(q.trim().toLowerCase())),
    [clients, q],
  );

  const openLinks = async (c: ClientRow) => {
    setLinksOf(c);
    setLinks(null);
    // guard against a fetch race: clicking user A then user B quickly must
    // never render A's links under B's modal
    const reqId = ++linksReqId.current;
    try {
      const r = await api.links(c.id);
      if (reqId !== linksReqId.current) return; // a newer request superseded this one
      setLinks(r.links);
    } catch (e) {
      if (reqId !== linksReqId.current) return;
      toast("err", e instanceof Error ? e.message : "خطا در دریافت لینک‌ها");
      setLinks([]);
    }
  };

  const doDelete = async (c: ClientRow) => {
    try {
      await api.deleteClient(c.id);
      toast("ok", `کاربر «${c.name}» حذف شد`);
      setDel(null);
      await reload();
    } catch (e) {
      toast("err", e instanceof Error ? e.message : "خطا در حذف");
    }
  };

  return (
    <div className="space-y-4">
      <div className="flex items-center justify-between gap-3 flex-wrap">
        <div>
          <h1 className="text-xl font-bold gold-text">کاربران</h1>
          <p className="text-[11.5px] text-mu mt-1">{clients.length.toLocaleString("fa-IR")} کاربر ثبت‌شده در PostgreSQL</p>
        </div>
        <div className="flex items-center gap-2 flex-1 sm:flex-none min-w-[220px]">
          <div className="relative flex-1 sm:w-56">
            <input className="ng-input !py-2 !pr-9" placeholder="جستجوی کاربر…" value={q} onChange={(e) => setQ(e.target.value)} />
            <Search className="absolute right-3 top-1/2 -translate-y-1/2 size-4 text-mu/60" />
          </div>
          <button className="ng-btn ng-btn-gold" onClick={() => setForm({ open: true, edit: null })}>
            <Plus className="size-4" /> کاربر جدید
          </button>
        </div>
      </div>

      <div className="ng-card overflow-hidden">
        <div className="overflow-x-auto">
          <table className="w-full text-sm min-w-[860px]">
            <thead>
              <tr className="text-[11px] text-mu border-b border-line">
                <th className="text-right font-medium px-4 py-3">کاربر</th>
                <th className="text-right font-medium px-4 py-3">وضعیت</th>
                <th className="text-right font-medium px-4 py-3 w-[240px]">مصرف حجم</th>
                <th className="text-right font-medium px-4 py-3">سقف</th>
                <th className="text-right font-medium px-4 py-3">انقضا</th>
                <th className="text-right font-medium px-4 py-3">پروتکل‌ها</th>
                <th className="text-left font-medium px-4 py-3">اقدامات</th>
              </tr>
            </thead>
            <tbody>
              {busy && clients.length === 0 ? (
                <tr><td colSpan={7} className="py-16 text-center"><Spinner /></td></tr>
              ) : filtered.length === 0 ? (
                <tr><td colSpan={7} className="py-14 text-center text-mu text-[13px]">کاربری یافت نشد — اولین کاربر را بسازید</td></tr>
              ) : (
                filtered.map((c) => {
                  const used = c.up + c.down;
                  const p = c.usage_pct ?? pct(used, c.quota);
                  return (
                    <tr key={c.id} className="border-b border-line/60 last:border-0 hover:bg-white/[.02] transition">
                      <td className="px-4 py-3">
                        <div className="flex items-center gap-2.5">
                          <span className={`size-8 rounded-full grid place-items-center text-[12px] font-bold text-black bg-gradient-to-b from-gold2 to-goldd`}>
                            {c.name.slice(0, 1).toUpperCase()}
                          </span>
                          <div className="min-w-0">
                            <p className="font-semibold text-[13px] truncate">{c.name}</p>
                            {c.note ? <p className="text-[10.5px] text-mu truncate max-w-[160px]">{c.note}</p> : null}
                          </div>
                        </div>
                      </td>
                      <td className="px-4 py-3">
                        {c.status === "active" ? (
                          <Badge tone="ok"><span className="size-1.5 rounded-full bg-ok live-dot" /> فعال</Badge>
                        ) : c.status === "quota" ? (
                          <Badge tone="warn">اتمام حجم</Badge>
                        ) : (
                          <Badge tone="bad">منقضی شده</Badge>
                        )}
                      </td>
                      <td className="px-4 py-3">
                        <div className="flex items-center gap-2 mb-1.5" dir="ltr">
                          <span className="mono text-[11px] text-tx">{bytesFmt(used)}</span>
                          {c.quota > 0 ? (
                            <span className="mono text-[10px] text-mu">/ {bytesFmt(c.quota)}</span>
                          ) : (
                            <span className="mono text-[10px] text-mu">/ نامحدود</span>
                          )}
                        </div>
                        <div className="h-1.5 rounded-full bg-white/6 overflow-hidden" role="progressbar" aria-valuenow={p} aria-valuemin={0} aria-valuemax={100}>
                          {c.quota > 0 ? (
                            <div
                              className={`h-full rounded-full transition-all ${p >= 100 ? "bg-bad" : p >= 80 ? "bg-warn" : "bg-gradient-to-l from-gold to-gold2"}`}
                              style={{ width: `${Math.max(2, p)}%` }}
                            />
                          ) : (
                            <div className="h-full w-[3px] rounded-full bg-gradient-to-l from-gold to-gold2/40" />
                          )}
                        </div>
                      </td>
                      <td className="px-4 py-3">
                        {c.quota > 0 ? (
                          <span className="mono text-[11.5px]" dir="ltr">{bytesFmt(c.quota)}</span>
                        ) : (
                          <span className="inline-flex items-center gap-1 text-[11.5px] text-gtx"><InfinityIcon className="size-3.5" /> نامحدود</span>
                        )}
                      </td>
                      <td className="px-4 py-3">
                        <div className="text-[11.5px]">{jalali(c.expire_at)}</div>
                        <div className={`text-[10px] ${c.status === "expired" ? "text-bad" : "text-mu"}`}>{expireLabel(c.expire_at)}</div>
                      </td>
                      <td className="px-4 py-3">
                        {/* null/[] = follows the server's global protocols (incl. ALL
                            custom inbounds) — showing "no chips" here used to
                            trick admins into thinking the user had nothing enabled */}
                        {(c.protocols ?? []).length === 0 ? (
                          <span className="rounded-md bg-gold/10 border border-gold/25 px-1.5 py-0.5 text-[9.5px] text-gtx">پیش‌فرض سرور</span>
                        ) : (
                          <div className="flex flex-wrap gap-1 max-w-[150px]">
                            {(c.protocols ?? []).slice(0, 3).map((pk) => (
                              <span key={pk} className="rounded-md bg-white/5 border border-line px-1.5 py-0.5 text-[9.5px] text-mu" dir="ltr">
                                {protoChipLabel(pk, customs)}
                              </span>
                            ))}
                            {(c.protocols ?? []).length > 3 && (
                              <span className="rounded-md bg-gold/10 border border-gold/25 px-1.5 py-0.5 text-[9.5px] text-gtx">
                                +{((c.protocols ?? []).length - 3).toLocaleString("fa-IR")}
                              </span>
                            )}
                          </div>
                        )}
                      </td>
                      <td className="px-4 py-3">
                        <div className="flex items-center justify-end gap-1.5">
                          <IconBtn title="لینک‌ها و QR" onClick={() => openLinks(c)}><Link2 className="size-4" /></IconBtn>
                          <IconBtn title="ویرایش" onClick={() => setForm({ open: true, edit: c })}><Pencil className="size-4" /></IconBtn>
                          <IconBtn title="حذف" danger onClick={() => setDel(c)}><Trash2 className="size-4" /></IconBtn>
                        </div>
                      </td>
                    </tr>
                  );
                })
              )}
            </tbody>
          </table>
        </div>
      </div>

      {/* create / edit modal */}
      <UserFormModal
        open={form.open}
        edit={form.edit}
        customs={customs}
        globalOn={settings?.protocols ?? null}
        onClose={() => setForm({ open: false, edit: null })}
        onSaved={async (msg) => { setForm({ open: false, edit: null }); toast("ok", msg); await reload(); }}
      />

      {/* links modal */}
      <Modal open={!!linksOf} onClose={() => setLinksOf(null)} title={`کانفیگ‌های ${linksOf?.name ?? ""}`} wide>
        {linksOf && (
          <div className="space-y-4">
            <div className="ng-card-gold p-4 flex flex-col sm:flex-row items-start sm:items-center gap-3">
              <div className="flex-1 min-w-0">
                <p className="text-[11px] text-mu mb-1 flex items-center gap-1.5"><QrCode className="size-3.5" /> لینک اشتراک (همه پروتکل‌ها + به‌روزرسانی خودکار)</p>
                <code className="mono text-[11px] break-all block text-gtx" dir="ltr">{subURL(linksOf.sub_token)}</code>
              </div>
              <div className="flex gap-2 shrink-0">
                <CopyBtn text={subURL(linksOf.sub_token)} label="کپی" />
              </div>
            </div>

            {links === null ? (
              <div className="py-10 text-center"><Spinner /></div>
            ) : (
              <ul className="space-y-2 max-h-72 overflow-y-auto pl-1">
                {links.map((l) => (
                  <li key={l.key + l.url} className="ng-card !rounded-xl px-3 py-2.5 flex items-center gap-2">
                    <div className="min-w-0 flex-1">
                      <p className="text-[11px] font-semibold text-gtx">{l.label || linkLabel(l.url)}</p>
                      <p className="mono text-[10px] text-mu truncate" dir="ltr">{linkHost(l.url)} · {l.url.split("://")[0]}</p>
                    </div>
                    <CopyBtn text={l.url} />
                  </li>
                ))}
                {links.length === 0 && <li className="text-mu text-[13px] py-6 text-center">لینکی تولید نشد — پروتکل‌های این کاربر را بررسی کنید</li>}
              </ul>
            )}

            <div className="flex flex-wrap items-center gap-2.5 pt-1">
              <button className="ng-btn ng-btn-ghost" onClick={() => downloadClientJSON(linksOf.id, linksOf.name)}>
                <Download className="size-4" /> دانلود JSON کلاینت
              </button>
              {links && links[0] ? <QRImage text={links[0].url} size={132} /> : null}
              {links && links[0] ? (
                <p className="text-[10.5px] text-mu leading-relaxed max-w-[220px]">
                  QR لینک اول ({PROTOCOL_LABELS[(links[0].key as ProtocolKey)] ?? links[0].key}) — در v2rayNG «Scan QR code» را بزنید
                </p>
              ) : null}
            </div>
          </div>
        )}
      </Modal>

      {/* delete confirm */}
      <Modal open={!!del} onClose={() => setDel(null)} title="حذف کاربر">
        <p className="text-[13.5px] leading-relaxed">
          آیا از حذف <b className="text-gtx">{del?.name}</b> مطمئن هستید؟ همه کانفیگ‌های او بی‌درنگ غیرفعال می‌شوند و این عمل قابل بازگشت نیست.
        </p>
        <div className="mt-5 flex gap-2.5 justify-end">
          <button className="ng-btn ng-btn-ghost" onClick={() => setDel(null)}>انصراف</button>
          <button className="ng-btn ng-btn-danger" onClick={() => del && doDelete(del)}>
            <Trash2 className="size-4" /> حذف قطعی
          </button>
        </div>
      </Modal>
    </div>
  );
}

function IconBtn({ children, title, onClick, danger }: { children: React.ReactNode; title: string; onClick: () => void; danger?: boolean }) {
  return (
    <button
      onClick={onClick} title={title} aria-label={title}
      className={`p-2 rounded-lg border border-line bg-white/[.03] transition hover:bg-white/[.07] ${danger ? "text-bad/80 hover:text-bad hover:border-bad/40" : "text-mu hover:text-gtx hover:border-gold/40"}`}
    >
      {children}
    </button>
  );
}

/** table-chip label: builtin protocol keys shorten; custom inbound tags show
 *  the inbound's display name (falling back to the raw tag) */
function protoChipLabel(pk: string, customs: InboundInfo[] | null): string {
  if ((PROTOCOL_KEYS as readonly string[]).includes(pk)) {
    return pk.replace("vless-", "");
  }
  const ib = customs?.find((c) => c.tag === pk);
  return ib?.name ?? pk;
}

function UserFormModal({
  open, edit, customs, globalOn, onClose, onSaved,
}: {
  open: boolean; edit: ClientRow | null;
  customs: InboundInfo[] | null;
  globalOn: Record<string, boolean> | null;
  onClose: () => void; onSaved: (msg: string) => Promise<void>;
}) {
  const toast = useToast();
  const [d, setD] = useState<Draft>(emptyDraft);
  const [initial, setInitial] = useState<string>("");
  const [busy, setBusy] = useState(false);

  // reset whenever target changes
  const key = `${open}-${edit?.id ?? "new"}`;
  if (key !== initial) {
    setInitial(key);
    if (open) {
      const saved = edit?.protocols ?? [];
      setD(edit ? {
        name: edit.name, quota_gb: edit.quota ? String(Math.round((edit.quota / (1 << 30)) * 100) / 100) : "", expire_days: "", note: edit.note ?? "",
        // [] = "follow server defaults" — offer that mode explicitly instead of
        // showing an all-unchecked form that SAVES the opposite of what it shows
        protoMode: saved.length === 0 ? "default" : "custom",
        protocols: Object.fromEntries([
          ...PROTOCOL_KEYS.map((k) => [k, saved.includes(k)]),
          ...(customs ?? []).map((ib) => [ib.tag, saved.includes(ib.tag)]),
        ]),
      } : emptyDraft());
    }
  }

  const submit = async () => {
    // numeric parse FIRST: Persian digits (۵۰/۳۰) used to become NaN → 0, which
    // meant unlimited quota and a silently WIPED expiry on edit
    const q = parseNum(d.quota_gb);
    if (q === null && d.quota_gb.trim() !== "") {
      toast("err", "سقف حجم باید عدد باشد (مثلاً 50 یا ۵۰)");
      return;
    }
    if (q !== null && (q < 0 || q > 1048576)) {
      toast("err", "سقف حجم باید بین 0 تا 1048576 گیگابایت باشد");
      return;
    }
    const ed = parseNum(d.expire_days);
    if (ed === null && d.expire_days.trim() !== "") {
      toast("err", "روزهای انقضا باید عدد صحیح باشد (مثلاً 30 یا ۳۰)");
      return;
    }
    if (ed !== null && (!Number.isInteger(ed) || ed < 0 || ed > 36500)) {
      toast("err", "روزهای انقضا باید عدد صحیح بین 0 تا 36500 باشد");
      return;
    }

    setBusy(true);
    const body: {
      name: string; quota_gb: number; expire_days?: number;
      protocols?: string[]; note: string;
    } = {
      name: d.name.trim(),
      // quota 0 == unlimited is explicit user intent (prefilled with the
      // current quota, blanking it means "make unlimited")
      quota_gb: q ?? 0,
      note: d.note.trim(),
    };
    if (d.protoMode === "custom") {
      const sel = [
        ...PROTOCOL_KEYS.filter((k) => d.protocols[k]),
        ...(customs ?? []).map((ib) => ib.tag).filter((t) => d.protocols[t]),
      ];
      if (sel.length === 0) {
        // an empty list means "follow global" on the server — the exact OPPOSITE
        // of the all-unchecked UI, so block it instead of saving a lie
        toast("err", "حداقل یک پروتکل انتخاب کنید — یا حالت «پیش‌فرض سرور» را برگردانید");
        setBusy(false);
        return;
      }
      body.protocols = sel;
    }
    // "default" mode omits protocols entirely → null → follow global
    if (!edit || d.expire_days.trim() !== "") {
      // CRITICAL: in edit mode a blank expire field means "keep the current
      // expiry" — sending 0 here used to silently NULL expire_at on every
      // rename/note save (the Go handler treats <= 0 as "clear").
      body.expire_days = ed ?? 0;
    }
    try {
      if (edit) {
        await api.patchClient(edit.id, body);
        await onSaved(`کاربر «${body.name}» به‌روزرسانی شد`);
      } else {
        await api.createClient(body);
        await onSaved(`کاربر «${body.name}» ساخته شد`);
      }
    } catch (e) {
      toast("err", e instanceof Error ? e.message : "خطا در ذخیره");
    } finally {
      setBusy(false);
    }
  };

  return (
    <Modal open={open} onClose={onClose} title={edit ? `ویرایش ${edit.name}` : "کاربر جدید"}>
      <div className="space-y-4">
        <Field label="نام کاربر">
          <input className="ng-input" placeholder="مثلاً mobin_x" value={d.name} onChange={(e) => setD({ ...d, name: e.target.value })} />
        </Field>
        <div className="grid grid-cols-2 gap-3">
          <Field label="سقف حجم (GB)" hint="خالی یا صفر = نامحدود">
            <input className="ng-input" dir="ltr" inputMode="decimal" placeholder="50" value={d.quota_gb} onChange={(e) => setD({ ...d, quota_gb: e.target.value })} />
          </Field>
          <Field label={edit ? "تمدید (روز از امروز)" : "انقضا (روز)" } hint={edit ? "خالی = بدون تغییر انقضای فعلی" : "خالی یا صفر = بدون انقضا"}>
            <input className="ng-input" dir="ltr" inputMode="numeric" placeholder="30" value={d.expire_days} onChange={(e) => setD({ ...d, expire_days: e.target.value })} />
          </Field>
        </div>
        <Field label="یادداشت">
          <input className="ng-input" placeholder="اختیاری — مثلاً «گوشی خودم»" value={d.note} onChange={(e) => setD({ ...d, note: e.target.value })} />
        </Field>
        <div>
          <p className="text-xs text-mu font-medium mb-2 flex items-center gap-1.5"><BadgeCheck className="size-3.5" /> پروتکل‌های این کاربر</p>
          <div className="grid grid-cols-2 gap-2 mb-2.5">
            <button
              type="button"
              onClick={() => setD({ ...d, protoMode: "default" })}
              className={`rounded-xl border px-3 py-2 text-[12px] font-medium transition ${
                d.protoMode === "default"
                  ? "border-gold/40 bg-gold/10 text-gtx"
                  : "border-line bg-white/[.03] text-mu hover:border-gold/25"
              }`}
            >
              پیش‌فرض سرور
              <span className="block text-[9.5px] font-normal opacity-70 mt-0.5">
                {globalOn
                  ? `${Object.values(globalOn).filter(Boolean).length.toLocaleString("fa-IR")} پروتکل فعال + همه اینباندهای سفارشی`
                  : "همه پروتکل‌های فعال سرور"}
              </span>
            </button>
            <button
              type="button"
              onClick={() => setD({ ...d, protoMode: "custom" })}
              className={`rounded-xl border px-3 py-2 text-[12px] font-medium transition ${
                d.protoMode === "custom"
                  ? "border-gold/40 bg-gold/10 text-gtx"
                  : "border-line bg-white/[.03] text-mu hover:border-gold/25"
              }`}
            >
              انتخاب دستی
              <span className="block text-[9.5px] font-normal opacity-70 mt-0.5">فقط پروتکل‌های تیک‌خورده</span>
            </button>
          </div>
          {d.protoMode === "custom" ? (
            <div className="grid grid-cols-2 gap-2">
              {PROTOCOL_KEYS.map((k) => (
                <label key={k} className="flex items-center gap-2 rounded-xl border border-line bg-white/[.03] px-3 py-2.5 cursor-pointer hover:border-gold/30 transition">
                  <input type="checkbox" className="ng-check size-4" checked={!!d.protocols[k]} onChange={(e) => setD({ ...d, protocols: { ...d.protocols, [k]: e.target.checked } })} />
                  <span className="text-[12px] font-medium">{PROTOCOL_LABELS[k]}</span>
                </label>
              ))}
              {(customs ?? []).map((ib) => (
                <label key={ib.tag} className="flex items-center gap-2 rounded-xl border border-line bg-white/[.03] px-3 py-2.5 cursor-pointer hover:border-gold/30 transition">
                  <input type="checkbox" className="ng-check size-4" checked={!!d.protocols[ib.tag]} onChange={(e) => setD({ ...d, protocols: { ...d.protocols, [ib.tag]: e.target.checked } })} />
                  <span className="text-[12px] font-medium flex items-center gap-1.5 min-w-0">
                    <Waypoints className="size-3.5 shrink-0 text-mu/70" />
                    <span className="truncate">{ib.name}</span>
                    <span className="text-[9px] text-mu/60 shrink-0" dir="ltr">{ib.protocol}/{ib.network}</span>
                  </span>
                </label>
              ))}
            </div>
          ) : (
            <p className="text-[10.5px] text-mu leading-relaxed bg-white/[.02] border border-line rounded-xl px-3 py-2.5">
              این کاربر از تنظیمات کلی سرور پیروی می‌کند؛ هر تغییری در تنظیمات پروتکل‌ها یا اینباندهای سفارشی، بی‌درنگ روی او هم اعمال می‌شود.
            </p>
          )}
        </div>
        <div className="flex gap-2.5 justify-end pt-1">
          <button className="ng-btn ng-btn-ghost" onClick={onClose} type="button">انصراف</button>
          <button
            className="ng-btn ng-btn-gold" disabled={busy || !d.name.trim()}
            onClick={submit} type="button"
          >
            {busy ? <Spinner className="size-4 !border-t-transparent" /> : <UserRoundPlus className="size-4" />}
            {edit ? "ذخیره تغییرات" : "ساخت کاربر"}
          </button>
        </div>
        {edit ? (
          <p className="text-[10.5px] text-mu flex items-center gap-1.5"><CalendarClock className="size-3.5" /> انقضای فعلی: {expireLabel(edit.expire_at)} — برای تمدید، روزهای باقی‌مانده جدید را وارد کنید</p>
        ) : null}
      </div>
    </Modal>
  );
}
