"use client";

import { useEffect, useState } from "react";
import { apiRequest } from "../lib/api";
import { formatInteger, formatMoney } from "../lib/format";

type Performance = {
  master_account?: { available?: boolean; online?: boolean; balance?: number | null; equity?: number | null; currency?: string | null; trade_mode?: string | null };
  registered_clients?: number;
  equity_curve?: Array<{ hour: string | null; equity: number; currency: string }>;
};

export function PerformancePanel() {
  const [report, setReport] = useState<Performance | null>(null);
  const [error, setError] = useState("");
  useEffect(() => { apiRequest<Performance>("public/performance").then(setReport).catch((reason: unknown) => setError(reason instanceof Error ? reason.message : "Laporan tidak tersedia.")); }, []);
  return <section className="panel section"><span className="eyebrow">Performa terlapor</span><h2>Ringkasan performa</h2>
    {error && <p className="muted">Data performa belum tersedia saat ini.</p>}
    {report?.master_account?.available
      ? <div className="cards">
          <article className="card"><span className="eyebrow">Status</span><h2 className={report.master_account.online ? "tone-pos" : "tone-neg"}>{report.master_account.online ? "Online" : "Offline"}</h2></article>
          <article className="card"><span className="eyebrow">Balance</span><h2>{formatMoney(report.master_account.balance, report.master_account.currency)}</h2></article>
          <article className="card"><span className="eyebrow">Equity</span><h2>{formatMoney(report.master_account.equity, report.master_account.currency)}</h2></article>
        </div>
      : report && <p className="muted">Belum ada laporan akun utama.</p>}
    {report?.equity_curve?.length ? <p className="muted">{formatInteger(report.equity_curve.length)} titik equity · {formatInteger(report.registered_clients ?? 0)} klien terdaftar</p> : null}
  </section>;
}
