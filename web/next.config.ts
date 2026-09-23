import type { NextConfig } from "next";

// NullGate 3.0 web — API routing strategy:
//  - API_INTERNAL_URL set (Railway production): /api/* and /sub/* are proxied
//    server-side to the Go api service (cookies stay same-origin).
//  - Not set (sandbox preview / local dev): requests fall through to the
//    built-in demo route handlers under src/app/api (in-memory demo data).
const API = process.env.API_INTERNAL_URL || "";

const nextConfig: NextConfig = {
  output: "standalone",
  reactStrictMode: false,
  typescript: { ignoreBuildErrors: true },
  ...(API
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
