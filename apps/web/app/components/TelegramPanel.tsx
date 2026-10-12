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
  return <section className="panel section"><span className="eyebrow">Koneksi Telegram</span><h2>Pengaturan Telegram</h2>
    <p className="muted">Status: {status?.state ?? "Memuat"} · {status?.account_name ?? "Belum terhubung"}</p>
    {error && <p className="error" role="alert">{error}</p>}{notice && <p className="success" role="status">{notice}</p>}
    <form onSubmit={sendCode}><label className="field">Nomor telepon<input value={phone} onChange={(event) => setPhone(event.target.value)} /></label><button className="button" type="submit">Minta kode</button></form>
    <form onSubmit={(event) => void verify(event, "verify-code", { code })}><label className="field">Kode Telegram<input value={code} onChange={(event) => setCode(event.target.value)} /></label><button className="button" type="submit">Verifikasi kode</button></form>
    <form onSubmit={(event) => void verify(event, "verify-2fa", { password })}><label className="field">Kata sandi 2FA<input type="password" value={password} onChange={(event) => setPassword(event.target.value)} /></label><button className="button" type="submit">Verifikasi 2FA</button></form>
  </section>;
}
