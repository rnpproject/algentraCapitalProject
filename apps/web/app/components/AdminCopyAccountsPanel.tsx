"use client";

import { useEffect, useRef, useState } from "react";
import { createPortal } from "react-dom";
import { useRouter } from "next/navigation";
import { apiRequest } from "../lib/api";
import { AccountPerformanceDetails } from "./AccountPerformanceDetails";

type CopyAccount = {
  id: number;
  label: string;
  server: string;
  login: string;
  plan: string;
  execution_mode: string;
  owner_email: string;
};

export function AdminCopyAccountsPanel() {
  const router = useRouter();
  const [accounts, setAccounts] = useState<CopyAccount[]>([]);
  const [selected, setSelected] = useState<CopyAccount | null>(null);
  const [error, setError] = useState("");
  const [loading, setLoading] = useState(true);
  const closeRef = useRef<HTMLButtonElement>(null);

  useEffect(() => {
    let alive = true;
    async function load() {
      try {
        const session = await apiRequest<{ admin: boolean }>("auth/session");
        if (!session.admin) {
          router.replace("/admin/login");
          return;
        }
        const data = await apiRequest<{ accounts: CopyAccount[] }>("admin/copy-accounts");
        if (alive) setAccounts(data.accounts ?? []);
      } catch (reason) {
        if (!alive) return;
        if (reason instanceof Error && (reason.message.includes("admin") || reason.message.toLowerCase().includes("login"))) {
          router.replace("/admin/login");
        } else {
          setError(reason instanceof Error ? reason.message : "Daftar akun tidak tersedia.");
        }
      } finally {
        if (alive) setLoading(false);
      }
    }
    void load();
    return () => {
      alive = false;
    };
  }, [router]);

  // Modal: Escape menutup, scroll halaman dikunci, fokus ke tombol tutup.
  useEffect(() => {
    if (!selected) return;
    const onKey = (event: KeyboardEvent) => {
      if (event.key === "Escape") setSelected(null);
    };
    const previousOverflow = document.body.style.overflow;
    document.body.style.overflow = "hidden";
    document.addEventListener("keydown", onKey);
    closeRef.current?.focus();
    return () => {
      document.body.style.overflow = previousOverflow;
      document.removeEventListener("keydown", onKey);
    };
  }, [selected]);

  return (
    <main className="page-shell dashboard-page" id="main-content">
      <div className="page-heading">
        <div>
          <span className="eyebrow">ADMIN / COPY TRADING</span>
          <h1>Akun Copy Trading</h1>
          <p className="page-description">
            Pilih akun untuk melihat saldo, profit tujuh hari, dan riwayat transaksi.
          </p>
        </div>
        {!loading && !error && (
          <div className="heading-aside">
            <span className="status">{accounts.length} akun</span>
          </div>
        )}
      </div>

      {error && (
        <p className="status tone-neg" role="alert">
          {error}
        </p>
      )}

      <section className="panel">
        <div className="table-wrap">
          <table className="admin-accounts-table">
            <thead>
              <tr>
                <th className="num">ID</th>
                <th>Nama akun</th>
                <th>Pemilik</th>
                <th>Login / Server</th>
                <th aria-label="Aksi"></th>
              </tr>
            </thead>
            <tbody>
              {accounts.map((account) => (
                <tr key={account.id}>
                  <td className="num">{account.id}</td>
                  <td>{account.label}</td>
                  <td title={account.owner_email}>{account.owner_email}</td>
                  <td title={`${account.login} · ${account.server}`}>
                    {account.login} · {account.server}
                  </td>
                  <td className="admin-accounts-action">
                    <button type="button" className="secondary" onClick={() => setSelected(account)}>
                      Detail
                    </button>
                  </td>
                </tr>
              ))}
              {!accounts.length && !error && (
                <tr>
                  <td className="empty-cell" colSpan={5}>
                    {loading ? "Memverifikasi akses admin…" : "Belum ada akun copy trading."}
                  </td>
                </tr>
              )}
            </tbody>
          </table>
        </div>
      </section>

      {selected && typeof document !== "undefined" && createPortal(
        <div
          className="account-modal-backdrop"
          role="presentation"
          onMouseDown={(event) => {
            if (event.target === event.currentTarget) setSelected(null);
          }}
        >
          <section
            className="account-modal panel"
            role="dialog"
            aria-modal="true"
            aria-labelledby="copy-account-modal-title"
          >
            <button
              ref={closeRef}
              className="secondary account-modal-close"
              type="button"
              onClick={() => setSelected(null)}
              aria-label="Tutup detail"
            >
              Tutup
            </button>
            <span className="eyebrow">
              AKUN #{selected.id} · {selected.execution_mode}
            </span>
            <h2 id="copy-account-modal-title">{selected.label}</h2>
            <p className="account-modal__meta">
              {selected.owner_email} · {selected.login} · {selected.server}
            </p>
            <AccountPerformanceDetails key={selected.id} accountId={selected.id} admin alwaysOpen />
          </section>
        </div>,
        document.body
      )}
    </main>
  );
}