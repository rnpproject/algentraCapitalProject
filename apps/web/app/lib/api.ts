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

function beginActionButtonLoading(): () => void {
  if (typeof document === "undefined") return () => {};
  const active = document.activeElement;
  let button = active instanceof HTMLButtonElement ? active : null;
  if (!button && active instanceof HTMLElement) {
    button = active.closest("form")?.querySelector<HTMLButtonElement>(
      'button[type="submit"], button:not([type])'
    ) ?? null;
  }
  if (!button || button.hasAttribute("aria-busy")) return () => {};
  button.dataset.autoLoading = "true";
  button.setAttribute("aria-busy", "true");
  return () => {
    delete button!.dataset.autoLoading;
    button!.removeAttribute("aria-busy");
  };
}

export async function apiRequest<T>(path: string, init: RequestInit = {}): Promise<T> {
  const headers = new Headers(init.headers);
  const method = (init.method ?? "GET").toUpperCase();
  if (init.body && !headers.has("content-type")) headers.set("content-type", "application/json");
  if (!new Set(["GET", "HEAD", "OPTIONS"]).has(method)) {
    const csrf = csrfHeaders().get("x-csrf-token");
    if (csrf) headers.set("x-csrf-token", csrf);
  }
  const stopLoading = new Set(["GET", "HEAD", "OPTIONS"]).has(method)
    ? () => {}
    : beginActionButtonLoading();
  try {
    const response = await fetch(`/api/core/${path}`, { ...init, method, headers, cache: "no-store" });
    const data: unknown = await response.json().catch(() => ({}));
    if (!response.ok) throw new Error(readError(data));
    return data as T;
  } finally {
    stopLoading();
  }
}

export async function apiUpload<T>(path: string, form: FormData): Promise<T> {
  const headers = csrfHeaders();
  const stopLoading = beginActionButtonLoading();
  try {
    const response = await fetch(`/api/core/${path}`, { method: "POST", headers, body: form, cache: "no-store" });
    const data: unknown = await response.json().catch(() => ({}));
    if (!response.ok) throw new Error(readError(data));
    return data as T;
  } finally {
    stopLoading();
  }
}
