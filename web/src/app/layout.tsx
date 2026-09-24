import type { Metadata, Viewport } from "next";
import localFont from "next/font/local";
import "./globals.css";

const vazir = localFont({
  src: [
    { path: "../../public/fonts/Vazirmatn-Regular.woff2", weight: "400", style: "normal" },
    { path: "../../public/fonts/Vazirmatn-Medium.woff2", weight: "500", style: "normal" },
    { path: "../../public/fonts/Vazirmatn-SemiBold.woff2", weight: "600", style: "normal" },
    { path: "../../public/fonts/Vazirmatn-Bold.woff2", weight: "700", style: "normal" },
  ],
  variable: "--font-vazir",
  display: "swap",
});

export const metadata: Metadata = {
  title: "NullGate 3.0 — پنل مدیریت",
  description: "پنل مدیریت پروکسی NullGate — نسخه ۳ با Next.js + Go + PostgreSQL",
  icons: { icon: { url: "/favicon.png", type: "image/png", sizes: "128x128" } },
};

export const viewport: Viewport = {
  themeColor: "#0a0a0b",
  width: "device-width",
  initialScale: 1,
  maximumScale: 5,
};

export default function RootLayout({ children }: { children: React.ReactNode }) {
  return (
    <html lang="fa" dir="rtl" className={vazir.variable}>
      <body className="min-h-screen">{children}</body>
    </html>
  );
}
