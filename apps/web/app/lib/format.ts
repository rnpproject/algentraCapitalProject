const LOCALE = "id-ID";
const EMPTY = "—";

export type Tone = "pos" | "neg" | "warn" | "info" | "muted" | "";

export function formatNumber(value: number | string | null | undefined, min = 0, max = 2): string {
  if (value === null || value === undefined || value === "") return EMPTY;
  const parsed = Number(value);
  if (!Number.isFinite(parsed)) return EMPTY;
  return parsed.toLocaleString(LOCALE, { minimumFractionDigits: min, maximumFractionDigits: Math.max(min, max) });
}

export const formatInteger = (value: number | string | null | undefined) => formatNumber(value, 0, 0);
export const formatPrice = (value: number | string | null | undefined) => formatNumber(value, 2, 5);
export const formatLots = (value: number | string | null | undefined) => formatNumber(value, 2, 3);

export function formatMoney(value: number | string | null | undefined, currency?: string | null): string {
  const text = formatNumber(value, 2, 2);
  return text === EMPTY || !currency ? text : `${text} ${currency}`;
}

export function numberTone(value: number | string | null | undefined): Tone {
  const parsed = Number(value);
  if (value === null || value === undefined || !Number.isFinite(parsed) || parsed === 0) return "";
  return parsed > 0 ? "pos" : "neg";
}

export function toneClass(tone: Tone): string {
  return tone ? `tone-${tone}` : "";
}
