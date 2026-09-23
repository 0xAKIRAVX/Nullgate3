import type { NextConfig } from "next";

// NullGate 3.0 web — three build modes:
//
// 1. STATIC_EXPORT=1 (Railway api service, single-service deploy):
//    `next build` emits a fully static out/ that the Go binary embeds via
//    go:embed (internal/webui). The demo route handlers under src/app/api and
//    src/app/sub are removed before the build (they are request-dependent and
//    cannot be exported) — the Go API answers the same same-origin paths.
//
// 2. API_INTERNAL_URL set (separate web service): /api/* and /sub/* are
//    proxied server-side to the Go api service (cookies stay same-origin).
//
// 3. Neither (local dev): requests fall through to the built-in demo route
//    handlers under src/app/api (in-memory demo data).
const STATIC = process.env.STATIC_EXPORT === "1";
const API = process.env.API_INTERNAL_URL || "";

const nextConfig: NextConfig = {
  ...(STATIC ? { output: "export" as const } : { output: "standalone" as const }),
  reactStrictMode: false,
  typescript: { ignoreBuildErrors: true },
  images: { unoptimized: true },
  ...(API && !STATIC
    ? {
        async rewrites() {
          // beforeFiles → the Go api always wins over the built-in demo handlers
          return {
            beforeFiles: [
              { source: "/api/:path*", destination: `${API}/api/:path*` },
              { source: "/sub/:path*", destination: `${API}/sub/:path*` },
            ],
          };
        },
      }
    : {}),
};

export default nextConfig;
