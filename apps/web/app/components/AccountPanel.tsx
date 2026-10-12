"use client";

import Link from "next/link";
import { useEffect, useState } from "react";
import { apiRequest } from "../lib/api";

type Account = { id: number; label: string; server: string; login: string; plan: string; execution_mode: string; active: boolean };

export function AccountPanel() {
  const [accounts, setAccounts] = useState<Account[]>([]);
  const [error, setError] = useState("");
  const [loading, setLoading] = useState(true);
  useEffect(() => {
    apiRequest<Account[]>("mt5/accounts").then(setAccounts).catch((reason: unknown) => {
      setError(reason instanceof Error ? reason.message : "Gagal memuat akun.");
    }).finally(() => setLoading(false));
  }, []);
  return <section className="panel section">
    <div className="actions" style={{ justifyContent: "space-between", marginTop: 0 }}><div><span className="eyebrow">Portal klien</span><h2>Akun MT5</h2></div><Link className="button" href="/transactions">Riwayat transaksi</Link></div>
    <p className="muted">Kredensial broker tidak ditampilkan pada halaman ini.</p>
    {loading && <p className="muted">Memuat akun…</p>}{error && <p className="error" role="alert">{error}</p>}
    {!loading && !error && accounts.length === 0 && <p className="muted">Belum ada akun MT5 yang terhubung.</p>}
    {accounts.length > 0 && <div className="table-wrap"><table><thead><tr><th>Label</th><th>Server</th><th>Login</th><th>Paket</th><th>Mode</th><th>Status</th></tr></thead><tbody>
      {accounts.map((account) => <tr key={account.id}><td>{account.label}</td><td>{account.server}</td><td>{account.login}</td><td>{account.plan}</td><td>{account.execution_mode}</td><td>{account.active ? "Aktif" : "Nonaktif"}</td></tr>)}
    </tbody></table></div>}
  </section>;
}
