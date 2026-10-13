"use client";

import { FormEvent, useEffect, useState } from "react";
import { apiRequest } from "../lib/api";

type TelegramStatus = { configured?: boolean; connected?: boolean; state?: string; account_name?: string | null };

export function TelegramPanel() {
  const [status, setStatus] = useState<TelegramStatus | null>(null);
  const [phone, setPhone] = useState("");
  const [code, setCode] = useState("");
  const [password, setPassword] = useState("");
  const [notice, setNotice] = useState("");
  const [error, setError] = useState("");
  async function refresh() {
    try { setStatus(await apiRequest<TelegramStatus>("telegram/status")); }
    catch (reason) { setError(reason instanceof Error ? reason.message : "Status tidak dapat dimuat."); }
  }
  useEffect(() => { void refresh(); }, []);
  async function sendCode(event: FormEvent) {
    event.preventDefault(); setError(""); setNotice("");
    try { await apiRequest("telegram/send-code", { method: "POST", body: JSON.stringify({ phone }) }); setNotice("Kode diminta dari Telegram."); }
    catch (reason) { setError(reason instanceof Error ? reason.message : "Gagal meminta kode."); }
  }
  async function verify(event: FormEvent, endpoint: string, value: Record<string, string>) {
    event.preventDefault(); setError(""); setNotice("");
    try { const result = await apiRequest<{ status?: string }>(`telegram/${endpoint}`, { method: "POST", body: JSON.stringify(value) }); setNotice(result.status ?? "Telegram diperbarui."); await refresh(); }
    catch (reason) { setError(reason instanceof Error ? reason.message : "Verifikasi gagal."); }
  }
  return <section className="telegram-page">
    <header className="telegram-heading">
      <div>
        <span className="eyebrow">Koneksi Telegram</span>
        <h1>Pengaturan Telegram</h1>
        <p className="muted">Hubungkan akun Telegram untuk mengelola sumber sinyal.</p>
      </div>
      <p className="telegram-status" aria-live="polite">
        <span>Status</span>
        <strong>{status?.state ?? "Memuat"}</strong>
        <span>{status?.account_name ?? "Belum terhubung"}</span>
      </p>
    </header>
    {error && <p className="error telegram-message" role="alert">{error}</p>}
    {notice && <p className="success telegram-message" role="status">{notice}</p>}
    <div className="telegram-layout">
      <section className="telegram-card telegram-login-card">
        <div className="telegram-card-heading">
          <h2>Hubungkan akun</h2>
          <p className="muted">Masuk dengan nomor telepon, lalu verifikasi kode yang dikirim Telegram.</p>
        </div>
        <div className="telegram-login-forms">
          <form onSubmit={sendCode}>
            <label className="field">
              Nomor telepon (format internasional)
              <input autoComplete="tel" inputMode="tel" placeholder="+628123456789" value={phone} onChange={(event) => setPhone(event.target.value)} />
            </label>
            <button className="button primary" type="submit">Minta kode Telegram</button>
          </form>
          <form onSubmit={(event) => void verify(event, "verify-code", { code })}>
            <label className="field">
              Kode Telegram
              <input autoComplete="one-time-code" inputMode="numeric" value={code} onChange={(event) => setCode(event.target.value)} />
            </label>
            <button className="button" type="submit">Verifikasi kode</button>
          </form>
        </div>
      </section>
      <section className="telegram-card telegram-twofa-card">
        <div className="telegram-card-heading">
          <h2>Verifikasi 2FA</h2>
          <p className="muted">Jika akun Anda memakai kata sandi verifikasi dua langkah, masukkan di sini.</p>
        </div>
        <form onSubmit={(event) => void verify(event, "verify-2fa", { password })}>
          <label className="field">
            Kata sandi 2FA
            <input autoComplete="current-password" type="password" value={password} onChange={(event) => setPassword(event.target.value)} />
          </label>
          <button className="button" type="submit">Verifikasi 2FA</button>
        </form>
      </section>
    </div>
  </section>;
}
