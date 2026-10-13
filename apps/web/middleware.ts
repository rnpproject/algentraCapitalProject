import { NextRequest, NextResponse } from "next/server";

const API_UNAVAILABLE_MESSAGE = "Layanan sedang dalam pemeliharaan. Coba lagi beberapa saat lagi.";
const PROBE_TIMEOUT_MS = 2500;

function escapeHtml(value: string): string {
  return value.replace(/[&<>"']/g, (character) => {
    const entities: Record<string, string> = {
      "&": "&amp;",
      "<": "&lt;",
      ">": "&gt;",
      '"': "&quot;",
      "'": "&#39;",
    };
    return entities[character];
  });
}

function maintenanceResponse(request: NextRequest, status: number, detail: string): Response {
  const isDevelopment = process.env.NODE_ENV === "development";
  const headers = { "cache-control": "no-store", "content-type": "text/html; charset=utf-8" };

  if (request.nextUrl.pathname.startsWith("/api/")) {
    return Response.json(
      {
        detail: API_UNAVAILABLE_MESSAGE,
        ...(isDevelopment ? { debug: { upstream_status: status || null, error: detail } } : {}),
      },
      { status: 503, headers: { "cache-control": "no-store" } },
    );
  }

  const diagnostics = isDevelopment
    ? `<details open><summary>Detail error (development)</summary><pre>HTTP ${status || "network"}\n${escapeHtml(detail)}</pre></details>`
    : "";

  return new Response(
    `<!doctype html>
<html lang="id">
  <head>
    <meta charset="utf-8">
    <meta name="viewport" content="width=device-width, initial-scale=1">
    <meta name="robots" content="noindex">
    <title>Pemeliharaan | Algentra Capital</title>
    <style>
      :root{color-scheme:dark;--bg:#0b1118;--panel:#13202c;--border:#263b4d;--text:#edf4fa;--muted:#a4b5c4;--accent:#58b7f5}
      *{box-sizing:border-box}body{margin:0;min-height:100vh;display:grid;place-items:center;padding:24px;background:radial-gradient(ellipse at top,#173044 0%,var(--bg) 58%);color:var(--text);font:16px/1.6 Inter,ui-sans-serif,system-ui,-apple-system,"Segoe UI",sans-serif}
      main{width:min(100%,620px);padding:clamp(24px,6vw,48px);border:1px solid var(--border);border-radius:20px;background:rgba(19,32,44,.94);box-shadow:0 24px 80px #0006}
      .brand{color:var(--accent);font-size:12px;font-weight:800;letter-spacing:.18em;text-transform:uppercase}h1{margin:16px 0 10px;font-size:clamp(30px,7vw,44px);line-height:1.12;letter-spacing:-.04em}.muted{color:var(--muted);margin:0}
      .status{display:flex;align-items:center;gap:9px;margin-top:24px;color:var(--muted);font-size:14px}.dot{width:9px;height:9px;border-radius:50%;background:#f5b942;box-shadow:0 0 14px #f5b94288}
      a{display:inline-flex;margin-top:28px;padding:11px 17px;border-radius:10px;background:var(--accent);color:#06131d;font-weight:750;text-decoration:none}a:hover{filter:brightness(1.08)}
      details{margin-top:24px;padding:14px;border:1px solid var(--border);border-radius:10px;color:var(--muted);font-size:13px}summary{cursor:pointer;font-weight:700;color:var(--text)}pre{margin:12px 0 0;white-space:pre-wrap;overflow-wrap:anywhere;font:12px/1.55 ui-monospace,SFMono-Regular,Consolas,monospace}
    </style>
  </head>
  <body>
    <main aria-live="polite">
      <div class="brand">Algentra Capital</div>
      <h1>Kami sedang melakukan pemeliharaan</h1>
      <p class="muted">${API_UNAVAILABLE_MESSAGE}</p>
      <div class="status"><span class="dot" aria-hidden="true"></span>Core API belum dapat dijangkau.</div>
      <a href="/">Coba lagi</a>
      ${diagnostics}
    </main>
  </body>
</html>`,
    { status: 503, headers },
  );
}

export async function middleware(request: NextRequest): Promise<Response> {
  const controller = new AbortController();
  const timeout = setTimeout(() => controller.abort(), PROBE_TIMEOUT_MS);

  try {
    const origin = process.env.CORE_API_ORIGIN ?? "http://127.0.0.1:8000";
    const headers = new Headers({ accept: "application/json" });
    const accessId = process.env.CF_ACCESS_CLIENT_ID;
    const accessSecret = process.env.CF_ACCESS_CLIENT_SECRET;
    if (accessId && accessSecret) {
      headers.set("cf-access-client-id", accessId);
      headers.set("cf-access-client-secret", accessSecret);
    }

    const response = await fetch(new URL("/health", origin), {
      method: "GET",
      headers,
      cache: "no-store",
      signal: controller.signal,
    });
    if (!response.ok) {
      const body = (await response.text()).slice(0, 2000);
      return maintenanceResponse(
        request,
        response.status,
        body || `Core API health returned HTTP ${response.status}`,
      );
    }
    return NextResponse.next();
  } catch (error) {
    const detail = error instanceof Error ? `${error.name}: ${error.message}` : String(error);
    return maintenanceResponse(request, 0, detail);
  } finally {
    clearTimeout(timeout);
  }
}

export const config = {
  matcher: ["/((?!_next/static|_next/image|favicon.ico|robots.txt|sitemap.xml).*)"],
};
