import type { NextConfig } from "next";

const nextConfig: NextConfig = {
  poweredByHeader: false,
  reactStrictMode: true,
  async rewrites() {
    return {
      beforeFiles: [
        {
          source: "/health",
          destination: "http://127.0.0.1:8000/health",
        },
        {
          source: "/ready",
          destination: "http://127.0.0.1:8000/ready",
        },
        {
          source: "/api/core/downloads/:path*",
          destination: "http://127.0.0.1:8000/downloads/:path*",
        },
        {
          source: "/api/core/:path*",
          destination: "http://127.0.0.1:8000/v1/:path*",
        },
      ],
      afterFiles: [
        {
          source: "/api/:path*",
          destination: "http://127.0.0.1:8000/api/:path*",
        },
      ],
    };
  },
  async headers() {
    return [{
      source: "/:path*",
      headers: [
        { key: "X-Content-Type-Options", value: "nosniff" },
        { key: "Referrer-Policy", value: "strict-origin-when-cross-origin" },
        { key: "X-Frame-Options", value: "DENY" },
        { key: "Permissions-Policy", value: "camera=(), microphone=(), geolocation=()" }
      ]
    }, {
      // Aset brand-kit & musik jarang berubah: cache di browser/CDN agar tidak diunduh ulang tiap kunjungan.
      source: "/brand-kit/:path*",
      headers: [{ key: "Cache-Control", value: "public, max-age=86400, stale-while-revalidate=604800" }]
    }, {
      source: "/musik.mp3",
      headers: [{ key: "Cache-Control", value: "public, max-age=86400, stale-while-revalidate=604800" }]
    }];
  }
};

export default nextConfig;
