"use client";

import { useEffect, useState } from "react";
import { apiRequest } from "../lib/api";
import { formatMoney } from "../lib/format";

type Daily = { date: string; profit: number; currency?: string | null; timezone: string };
type Deal = { ticket: string; time: string; symbol: string; action: string; entry: string; volume: number; price: number; net_profit: number; currency?: string | null };
type Details = { balance?: number | null; equity?: number | null; floating_profit?: number | null; margin?: number | null; free_margin?: number | null; currency?: string | null; open_positions?: number; state_updated_at?: string | null; daily_profit: Daily[]; history: Deal[]; history_page: number; history_pages: number; history_total: number };

const wibDate = (date: string) => new Date(`${date}T00:00:00+07:00`).toLocaleDateString("id-ID", { timeZone: "Asia/Jakarta", weekday: "short", day: "2-digit", month: "short" });
const wibTime = (date?: string | null) => date ? new Date(date).toLocaleString("id-ID", { timeZone: "Asia/Jakarta" }) : "Belum ada laporan";

export function AccountPerformanceDetails({ accountId, admin = false, compact = false }: { accountId: number; admin?: boolean; compact?: boolean }) {
  const [open, setOpen] = useState(compact);
  const [details, setDetails] = useState<Details | null>(null);
  const [historyPage, setHistoryPage] = useState(1);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState("");
  useEffect(() => {
    if (!compact) return;
    let alive = true;
    const path = admin ? `admin/copy-accounts/${accountId}/details` : `mt5/accounts/${accountId}/details`;
    setLoading(true);
    apiRequest<Details>(`${path}?page=1`).then(data => { if (alive) setDetails(data); }).catch(reason => { if (alive) setError(reason instanceof Error ? reason.message : "Detail akun tidak tersedia."); }).finally(() => { if (alive) setLoading(false); });
    return () => { alive = false; };
  }, [accountId,admin,compact]);
  useEffect(() => {
    if (!open || compact || admin || typeof EventSource === "undefined") return;
    let alive = true;
    const stream = new EventSource("/api/core/mt5/stream");
    stream.addEventListener("mt5-update", event => {
      try {
        const update = JSON.parse((event as MessageEvent<string>).data) as { account_id?: number };
        if (update.account_id !== accountId) return;
        void apiRequest<Details>(`mt5/accounts/${accountId}/details?page=${historyPage}`).then(data => { if (alive) setDetails(data); }).catch(() => {});
      } catch { /* Ignore malformed stream events. */ }
    });
    return () => { alive = false; stream.close(); };
  }, [accountId,admin,compact,open,historyPage]);
  async function loadPage(page: number) {
    if (page < 1 || (details && page > details.history_pages)) return;
    setLoading(true); setError("");
    try {
      const path = admin ? `admin/copy-accounts/${accountId}/details` : `mt5/accounts/${accountId}/details`;
      const data = await apiRequest<Details>(`${path}?page=${page}`);
      setDetails(data); setHistoryPage(data.history_page);
    } catch (reason) { setError(reason instanceof Error ? reason.message : "Riwayat transaksi tidak tersedia."); }
    finally { setLoading(false); }
  }
  async function toggle() {
    const next = !open;
    setOpen(next);
    if (!next || details || loading) return;
    setLoading(true); setError("");
    try {
      const path = admin ? `admin/copy-accounts/${accountId}/details` : `mt5/accounts/${accountId}/details`;
      setDetails(await apiRequest<Details>(`${path}?page=${historyPage}`));
    } catch (reason) { setError(reason instanceof Error ? reason.message : "Detail akun tidak tersedia."); }
    finally { setLoading(false); }
  }
  return <section className={compact ? "account-detail-popup" : "account-detail-section"}>
    <button type="button" className="secondary" aria-expanded={open} onClick={() => void toggle()}>{open ? "Tutup detail" : compact ? "Detail" : "Saldo, profit & riwayat"}</button>
    {open && <div className="account-detail-content">
      {loading && <p className="muted">Memuat detail akun…</p>}
      {error && <p className="status" role="alert">{error}</p>}
      {details && <>
        <p className="muted">Data terakhir: {wibTime(details.state_updated_at)} WIB</p>
        <div className="cards account-detail-metrics">
          {[["Saldo",details.balance],["Equity",details.equity],["Profit floating",details.floating_profit],["Margin",details.margin],["Free margin",details.free_margin],["Posisi terbuka",details.open_positions]].map(([label,value]) => <article className="card" key={String(label)}><span className="eyebrow">{label}</span><b>{label === "Posisi terbuka" ? (value ?? 0) : formatMoney(typeof value === "number" ? value : null,details.currency)}</b></article>)}
        </div>
        <h3>Profit harian · WIB</h3>
        <div className="cards daily-profit-cards">{(details.daily_profit ?? []).map(day => <article className="card" key={day.date}><span className="eyebrow">{wibDate(day.date)}</span><b className={day.profit >= 0 ? "tone-pos" : "tone-neg"}>{formatMoney(day.profit,day.currency ?? details.currency)}</b></article>)}</div>
        <h3>Riwayat transaksi MT5 · {details.history_total} transaksi</h3>
        {details.history.length === 0 ? <p className="muted">Belum ada transaksi yang dilaporkan.</p> : <><div className="table-wrap"><table><thead><tr><th>Waktu (WIB)</th><th>Simbol</th><th>Arah</th><th>Volume</th><th>Harga</th><th>Profit bersih</th></tr></thead><tbody>{details.history.map(deal => <tr key={deal.ticket}><td>{wibTime(deal.time)}</td><td>{deal.symbol}</td><td>{deal.action} · {deal.entry}</td><td>{deal.volume}</td><td>{deal.price}</td><td className={deal.net_profit >= 0 ? "tone-pos" : "tone-neg"}>{formatMoney(deal.net_profit,deal.currency ?? details.currency)}</td></tr>)}</tbody></table></div><div className="toolbar account-history-pagination"><button type="button" className="secondary" disabled={historyPage <= 1 || loading} onClick={() => void loadPage(historyPage - 1)}>Sebelumnya</button><span className="muted">Halaman {historyPage} dari {details.history_pages}</span><button type="button" className="secondary" disabled={historyPage >= details.history_pages || loading} onClick={() => void loadPage(historyPage + 1)}>Berikutnya</button></div></>}
      </>}
    </div>}
  </section>;
}
