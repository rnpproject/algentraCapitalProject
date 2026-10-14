function readError(data: unknown): string {
  return typeof data === "object" && data !== null && "detail" in data && typeof data.detail === "string"
    ? data.detail
    : "Permintaan tidak berhasil.";
}

function csrfHeaders(): Headers {
  const headers = new Headers();
  if (typeof window !== "undefined") {
    const csrf = sessionStorage.getItem("algentra-csrf");
    if (csrf) headers.set("x-csrf-token", csrf);
  }
  return headers;
}

export async function apiRequest<T>(path: string, init: RequestInit = {}): Promise<T> {
  const headers = new Headers(init.headers);
  const method = (init.method ?? "GET").toUpperCase();
  if (init.body && !headers.has("content-type")) headers.set("content-type", "application/json");
  if (!new Set(["GET", "HEAD", "OPTIONS"]).has(method)) {
    const csrf = csrfHeaders().get("x-csrf-token");
    if (csrf) headers.set("x-csrf-token", csrf);
  }
  const response = await fetch(`/api/core/${path}`, { ...init, method, headers, cache: "no-store" });
  const data: unknown = await response.json().catch(() => ({}));
  if (!response.ok) throw new Error(readError(data));
  return data as T;
}

export async function apiUpload<T>(path: string, form: FormData): Promise<T> {
  const headers = csrfHeaders();
  const response = await fetch(`/api/core/${path}`, { method: "POST", headers, body: form, cache: "no-store" });
  const data: unknown = await response.json().catch(() => ({}));
  if (!response.ok) throw new Error(readError(data));
  return data as T;
}
