# NullGate 3.0 🛰️

نسل سوم پنل مدیریت پروکسی شخصی — بازنویسی کامل با استک جدید (گزینه 🅰️):

| سرویس | تکنولوژی | نقش |
|---|---|---|
| `api` | Go 1.26 + Xray-core | مغز پنل: REST API، موتور Xray، WebSocket آمار زنده، ساب‌اسکریپشن |
| `web` | Next.js 16 (standalone) | رابط کاربری مشکی-طلایی، فارسی RTL |
| `postgres` | PostgreSQL 16 | منبع حقیقت: کاربرها، کلیدها، آمار |
| `redis` | Redis 7 | جلسه‌های ادمین + محدودسازی نرخ |

## چرا 3.0؟

- **کلیدهای Reality دیگر پاک نمی‌شوند** — همه‌چیز در PostgreSQL است، نه فایل `state.json`؛ Redeploy بدون خطر.
- **معماری چندسرویسی** — Xray و رابط کاربری دیگر توی یک فرآیند جنگ ندارند.
- **آمار دقیق** — مصرف ترافیک از statsAPI خود Xray جمع و دوره‌ای در دیتابیس ذخیره می‌شود.
- **همه‌چیز واقعی تست شده** — زنجیره کامل مرورگر → Next.js → Go → PostgreSQL → Xray → اینترنت (فاز ۴).

## پروتکل‌های پشتیبانی‌شده

| # | پروتکل | ترنسپورت | مسیر عمومی |
|---|---|---|---|
| 1 | VLESS Reality | TCP (Vision) | TCP Proxy (پورت ۹۰۰۰ داخلی) |
| 2 | VLESS XHTTP | HTTPS | `/xhttp` |
| 3 | VLESS WebSocket | TLS | `/ws` |
| 4 | VLESS HTTPUpgrade | TLS | `/hu` |
| 5 | VMess | TLS WebSocket | `/vmess` |
| 6 | Trojan | TLS WebSocket | `/trojan` |

پروتکل‌های ۲ تا ۶ روی دامنه HTTPS خود Railway اجرا می‌شوند (پورت ۴۴۳)؛ Reality مسیر اختصاصی TCP Proxy دارد.

## ساختار ریپو

```
nullgate-api/
├── Dockerfile              # سرویس api (Go + Xray پیش‌نصب‌شده)
├── docker-compose.yml      # تست محلی ۴ سرویسه
├── RAILWAY_GUIDE.md        # راهنمای قدم‌به‌قدم دیپلوی Railway (فارسی)
├── cmd/api/                # نقطه ورود بک‌اند
├── internal/
│   ├── api/                # REST + WebSocket + پروکسی ساب
│   ├── auth/               # bcrypt + جلسه (Redis/حافظه)
│   ├── config/             # متغیرهای محیطی
│   ├── db/                 # اتصال + مایگریشن PostgreSQL
│   ├── store/              # لایه داده (کاربر/تنظیمات/Reality)
│   └── xray/               # موتور: ساخت کانفیگ، supervisor، لینک‌ها، آمار
├── e2e/                    # تست‌های انتها-به-انتها (Xray واقعی + PG واقعی)
└── web/                    # فرانت Next.js (Dockerfile خودش را دارد)
    └── src/app/            # داشبورد، کاربرها، لینک‌ها، inbounds، تنظیمات
```

## اجرای محلی (docker compose)

```bash
docker compose up --build
# پنل:        http://localhost:3000
# اولین بازدید → صفحه Setup → ساخت ادمین
# Reality:     127.0.0.1:9000  (جانشین TCP Proxy در تست محلی)
```

## متغیرهای محیطی سرویس api

| متغیر | پیش‌فرض | توضیح |
|---|---|---|
| `PORT` | `8080` | پورت HTTP داخلی |
| `DATABASE_URL` | — (الزامی) | رشته اتصال PostgreSQL |
| `REDIS_URL` | — | اختیاری؛ نبودن → جلسه در حافظه |
| `SECRET` | — | امضای توکن اشتراک |
| `REALITY_SNI` | `www.samsung.com` | دامنه پوشاننده Reality |
| `TCP_APP_PORT` | `9000` | پورت inbound Reality داخل کانتینر (هدف TCP Proxy) |
| `TCP_HOST` / `TCP_PORT` | — | دامنه/پورت عمومی TCP Proxy (در لینک‌ها ظاهر می‌شود) |
| `XRAY_BIN` | `/app/xray/xray` | مسیر باینری Xray (در داکر پیش‌نصب است) |
| `XRAY_VERSION` | `v26.3.27` | نسخه پین‌شده Xray-core |
| `COLLECT_INTERVAL` | `30` | فاصله جمع‌آوری آمار (ثانیه) |
| `COOKIE_INSECURE` | خالی | فقط تست محلی http |

## متغیرهای سرویس web

| متغیر | توضیح |
|---|---|
| `API_INTERNAL_URL` | آدرس داخلی سرویس api (مثل `http://api.railway.internal:8080`)؛ ست نشود → حالت دمو با داده درون‌حافظه‌ای |
| `PORT` | پیش‌فرض `3000` |

## تست

```bash
go test ./...           # یونیت + E2E (Postgres/Redis embedded + Xray واقعی)
cd web && bun run build # بیلد فرانت
```
