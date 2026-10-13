"use client";

import Link from "next/link";
import { useEffect, useState } from "react";
import { apiRequest } from "../lib/api";

type DashboardStats = {
  signals_total?: number; signals_executed?: number; signals_pending?: number; signals_rejected?: number;
  active_clients?: number; ea_online?: boolean; telegram_connected?: boolean;
};

type Mt5Live = {
  available: boolean;
  online: boolean;
  observed_at: string | null;
  account: {
    balance: number; equity: number; floating_profit: number; currency: string; trade_mode: string; observed_at: string;
  } | null;
  positions: Array<{
    ticket: string; symbol: string; action: string; lots: number; entry_price: number; sl: number | null; tp: number | null;
  }>;
  market_quotes: Array<{ symbol: string; bid: number; ask: number; time_msc: number }>;
};

function formatTime(value: string | null): string {
  return value ? new Date(value).toLocaleTimeString("id-ID") : "Belum ada data";
}

export function DashboardPanel() {
  const [stats, setStats] = useState<DashboardStats | null>(null);
  const [mt5, setMt5] = useState<Mt5Live | null>(null);
  const [error, setError] = useState("");
  const [mt5Error, setMt5Error] = useState("");
  useEffect(() => {
    let active = true;
    let statsTimer: ReturnType<typeof setTimeout> | undefined;
    let mt5Timer: ReturnType<typeof setTimeout> | undefined;

    async function refreshStats() {
      try {
        const nextStats = await apiRequest<DashboardStats>("dashboard/stats");
        if (active) {
          setStats(nextStats);
          setError("");
        }
      } catch (reason) {
        if (active) setError(reason instanceof Error ? reason.message : "Gagal memuat dashboard.");
      } finally {
        if (active) statsTimer = setTimeout(() => void refreshStats(), 10000);
      }
    }

    async function refreshMt5() {
      try {
        const live = await apiRequest<Mt5Live>("dashboard/mt5-live");
        if (active) {
          setMt5(live);
          setMt5Error("");
        }
      } catch (reason) {
        if (active) setMt5Error(reason instanceof Error ? reason.message : "Data live MT5 gagal dimuat.");
      } finally {
        if (active) mt5Timer = setTimeout(() => void refreshMt5(), 2000);
      }
    }

    void refreshStats();
    void refreshMt5();
    return () => {
      active = false;
      clearTimeout(statsTimer);
      clearTimeout(mt5Timer);
    };
  }, []);
  const metrics = [
    ["Sinyal", stats?.signals_total], ["Dieksekusi", stats?.signals_executed],
    ["Menunggu", stats?.signals_pending], ["Klien", stats?.active_clients]
  ] as const;
  return <section className="section">
    <span className="eyebrow">Workspace privat</span><h1 style={{ fontSize: 48 }}>Dashboard admin</h1>
    <div className="actions"><Link className="button" href="/telegram-setup">Pengaturan Telegram</Link><Link className="button" href="/parser-test">Uji parser</Link><Link className="button" href="/">Lihat situs</Link></div>
    {error && <p className="error" role="alert">{error}</p>}
    {stats && <div className="cards">{metrics.map(([label, value]) => <article className="card" key={label}><span className="eyebrow">{label}</span><h2>{value ?? "—"}</h2></article>)}</div>}
    {stats && <div className="panel"><p>Status EA: <strong>{stats.ea_online ? "Online" : "Offline"}</strong></p><p>Status Telegram: <strong>{stats.telegram_connected ? "Terhubung" : "Terputus"}</strong></p></div>}
    <section className="panel mt5-live-panel" aria-live="polite">
      <header className="mt5-live-heading">
        <div><span className="eyebrow">Data terminal</span><h2>MT5 live</h2></div>
        <div className="mt5-live-status">
          <strong className={mt5?.online ? "is-online" : ""}>
            {mt5?.online ? "Online" : mt5?.available ? "Data basi" : "Menunggu terminal"}
          </strong>
          <span>Update: {formatTime(mt5?.observed_at ?? null)}</span>
        </div>
      </header>
      {mt5Error && <p className="error" role="alert">{mt5Error}</p>}
      <div className="mt5-account-grid">
        {[
          ["Balance", mt5?.account ? `${mt5.account.balance} ${mt5.account.currency}` : "—"],
          ["Equity", mt5?.account ? `${mt5.account.equity} ${mt5.account.currency}` : "—"],
          ["Floating P/L", mt5?.account ? `${mt5.account.floating_profit} ${mt5.account.currency}` : "—"],
          ["Mode", mt5?.account?.trade_mode ?? "—"],
          ["Posisi terbuka", mt5?.positions.length ?? 0],
        ].map(([label, value]) => <article className="mt5-account-item" key={label}>
          <span className="muted">{label}</span><strong>{value}</strong>
        </article>)}
      </div>
      <div className="mt5-live-columns">
        <div>
          <h3>Posisi terbuka</h3>
          {mt5?.positions.length
            ? <div className="table-wrap"><table>
                <thead><tr><th>Ticket</th><th>Simbol</th><th>Arah</th><th>Lot</th><th>Entry</th><th>SL</th><th>TP</th></tr></thead>
                <tbody>{mt5.positions.map((position) => <tr key={position.ticket}>
                  <td>{position.ticket}</td><td>{position.symbol}</td><td>{position.action}</td><td>{position.lots}</td>
                  <td>{position.entry_price}</td><td>{position.sl ?? "—"}</td><td>{position.tp ?? "—"}</td>
                </tr>)}</tbody>
              </table></div>
            : <p className="muted">Tidak ada posisi terbuka.</p>}
        </div>
        <div>
          <h3>Harga pasar</h3>
          {mt5?.market_quotes.length
            ? <div className="table-wrap"><table>
                <thead><tr><th>Simbol</th><th>Bid</th><th>Ask</th><th>Update</th></tr></thead>
                <tbody>{mt5.market_quotes.map((quote) => <tr key={quote.symbol}>
                  <td>{quote.symbol}</td><td>{quote.bid}</td><td>{quote.ask}</td>
                  <td>{new Date(quote.time_msc).toLocaleTimeString("id-ID")}</td>
                </tr>)}</tbody>
              </table></div>
            : <p className="muted">Belum ada harga pasar dari terminal.</p>}
        </div>
      </div>
    </section>
  </section>;
}
