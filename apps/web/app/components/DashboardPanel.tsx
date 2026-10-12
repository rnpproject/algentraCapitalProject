"use client";

import Link from "next/link";
import { useEffect, useState } from "react";
import { apiRequest } from "../lib/api";

type DashboardStats = {
  signals_total?: number; signals_executed?: number; signals_pending?: number; signals_rejected?: number;
  active_clients?: number; ea_online?: boolean; telegram_connected?: boolean;
};

export function DashboardPanel() {
  const [stats, setStats] = useState<DashboardStats | null>(null);
  const [error, setError] = useState("");
  useEffect(() => {
    apiRequest<DashboardStats>("dashboard/stats").then(setStats).catch((reason: unknown) => {
      setError(reason instanceof Error ? reason.message : "Gagal memuat dashboard.");
    });
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
  </section>;
}
