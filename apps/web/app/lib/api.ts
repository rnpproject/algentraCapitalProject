export async function apiRequest<T>(path: string, init: RequestInit = {}): Promise<T> {
  const headers = new Headers(init.headers);
  const method = (init.method ?? "GET").toUpperCase();
  if (init.body && !headers.has("content-type")) headers.set("content-type", "application/json");
  if (!new Set(["GET", "HEAD", "OPTIONS"]).has(method) && typeof window !== "undefined") {
    const csrf = sessionStorage.getItem("algentra-csrf");
    if (csrf) headers.set("x-csrf-token", csrf);
  }
  const response = await fetch(`/api/core/${path}`, { ...init, method, headers, cache: "no-store" });
  const data: unknown = await response.json().catch(() => ({}));
  if (!response.ok) {
    const detail = typeof data === "object" && data !== null && "detail" in data && typeof data.detail === "string"
      ? data.detail
      : "Permintaan tidak berhasil.";
    throw new Error(detail);
  }
  return data as T;
}
