import type { NextConfig } from "next";

const nextConfig: NextConfig = {
  poweredByHeader: false,
  // Web asli disajikan oleh core-api (template Jinja); Next hanya meneruskan semua request.
  async rewrites() {
    const origin = (process.env.CORE_API_ORIGIN ?? "http://127.0.0.1:8000").replace(/\/$/, "");
    return { beforeFiles: [{ source: "/:path*", destination: `${origin}/:path*` }], afterFiles: [], fallback: [] };
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
    }];
  }
};

export default nextConfig;
