import { createHash, createHmac, randomUUID } from "node:crypto";
import type { NextRequest } from "next/server";

export const runtime = "nodejs";
export const dynamic = "force-dynamic";

const ALLOWED_PREFIXES = new Set([
  "auth", "public", "payments", "mt5", "dashboard", "settings", "parser", "cs", "telegram", "downloads"
]);
const RESPONSE_HOP_HEADERS = new Set([
  "connection", "content-encoding", "content-length", "keep-alive", "transfer-encoding"
]);
const MAX_BODY_BYTES = 6 * 1024 * 1024;

class RequestBodyTooLarge extends Error {}

async function readBodyLimited(request: NextRequest): Promise<Uint8Array> {
  const contentLength = Number(request.headers.get("content-length"));
  if (Number.isFinite(contentLength) && contentLength > MAX_BODY_BYTES) {
    throw new RequestBodyTooLarge();
  }
  if (!request.body) return new Uint8Array();

  const reader = request.body.getReader();
  const chunks: Uint8Array[] = [];
  let size = 0;
  try {
    while (true) {
      const { done, value } = await reader.read();
      if (done) break;
      size += value.byteLength;
      if (size > MAX_BODY_BYTES) {
        await reader.cancel();
        throw new RequestBodyTooLarge();
      }
      chunks.push(value);
    }
  } finally {
    reader.releaseLock();
  }

  const body = new Uint8Array(size);
  let offset = 0;
  for (const chunk of chunks) {
    body.set(chunk, offset);
    offset += chunk.byteLength;
  }
  return body;
}

type RouteContext = { params: Promise<{ path: string[] }> };

async function proxy(request: NextRequest, context: RouteContext): Promise<Response> {
  const { path } = await context.params;
  if (!path.length || path.some((segment) => !segment || segment === "." || segment === ".." || segment.includes("\\"))) {
    return Response.json({ detail: "Path API tidak valid" }, { status: 400 });
  }
  if (!ALLOWED_PREFIXES.has(path[0])) {
    return Response.json({ detail: "Endpoint tidak tersedia melalui BFF" }, { status: 404 });
  }

  const origin = process.env.CORE_API_ORIGIN?.replace(/\/+$/, "");
  const secret = process.env.BFF_HMAC_SECRET;
  const accessId = process.env.CF_ACCESS_CLIENT_ID;
  const accessSecret = process.env.CF_ACCESS_CLIENT_SECRET;
  if (!origin || !secret || (process.env.NODE_ENV === "production" && (!accessId || !accessSecret))) {
    return Response.json({ detail: "BFF belum dikonfigurasi" }, { status: 503 });
  }

  const incomingUrl = new URL(request.url);
  // Unduhan file dilayani langsung di /downloads/* (tidak memakai signature /v1).
  const isDownload = path[0] === "downloads";
  const target = isDownload
    ? `/${path.map(encodeURIComponent).join("/")}${incomingUrl.search}`
    : `/v1/${path.map(encodeURIComponent).join("/")}${incomingUrl.search}`;
  let body: Uint8Array;
  try {
    body = request.method === "GET" || request.method === "HEAD"
      ? new Uint8Array()
      : await readBodyLimited(request);
  } catch (error) {
    if (error instanceof RequestBodyTooLarge) {
      return Response.json({ detail: "Ukuran permintaan melewati batas" }, { status: 413 });
    }
    return Response.json({ detail: "Body permintaan tidak dapat dibaca" }, { status: 400 });
  }
  const timestamp = Math.floor(Date.now() / 1000).toString();
  const nonce = randomUUID();
  const bodyHash = createHash("sha256").update(body).digest("hex");
  const canonical = [timestamp, nonce, request.method.toUpperCase(), target, bodyHash].join("\n");
  const signature = createHmac("sha256", secret).update(canonical).digest("hex");
  let apiOrigin: URL;
  try {
    apiOrigin = new URL(origin);
  } catch {
    return Response.json({ detail: "CORE_API_ORIGIN tidak valid" }, { status: 503 });
  }
  if (
    apiOrigin.username || apiOrigin.password || apiOrigin.search || apiOrigin.hash || apiOrigin.pathname !== "/" ||
    (process.env.NODE_ENV === "production" && apiOrigin.protocol !== "https:")
  ) {
    return Response.json({ detail: "CORE_API_ORIGIN tidak valid" }, { status: 503 });
  }
  const headers = new Headers({
    accept: request.headers.get("accept") ?? "application/json",
    origin: apiOrigin.origin,
  });
  if (!isDownload) {
    // BFF HMAC hanya diwajibkan untuk path /v1/* di Core API.
    headers.set("x-algentra-timestamp", timestamp);
    headers.set("x-algentra-nonce", nonce);
    headers.set("x-algentra-signature", signature);
  }
  for (const name of ["content-type", "cookie", "x-csrf-token"]) {
    const value = request.headers.get(name);
    if (value) headers.set(name, value);
  }
  if (accessId && accessSecret) {
    headers.set("cf-access-client-id", accessId);
    headers.set("cf-access-client-secret", accessSecret);
  }
  let fetchBody: ArrayBuffer | undefined;
  if (body.length) {
    fetchBody = new ArrayBuffer(body.length);
    new Uint8Array(fetchBody).set(body);
  }

  let upstream: Response;
  try {
    upstream = await fetch(new URL(target, origin), {
      method: request.method,
      headers,
      body: fetchBody,
      cache: "no-store",
      redirect: "manual"
    });
  } catch {
    return Response.json({ detail: "Core API tidak dapat dijangkau" }, { status: 502 });
  }

  const responseHeaders = new Headers();
  upstream.headers.forEach((value, name) => {
    if (!RESPONSE_HOP_HEADERS.has(name.toLowerCase()) && name.toLowerCase() !== "set-cookie") {
      responseHeaders.append(name, value);
    }
  });
  for (const cookie of upstream.headers.getSetCookie()) responseHeaders.append("set-cookie", cookie);
  responseHeaders.set("cache-control", "private, no-store");
  return new Response(await upstream.arrayBuffer(), {
    status: upstream.status,
    statusText: upstream.statusText,
    headers: responseHeaders
  });
}

export const GET = proxy;
export const POST = proxy;
export const PUT = proxy;
export const PATCH = proxy;
export const DELETE = proxy;
