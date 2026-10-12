"use client";

import { FormEvent, useState } from "react";
import { SiteFrame } from "../components/SiteFrame";

export default function ForgotPasswordPage() {
  const [email, setEmail] = useState("");
  const [code, setCode] = useState("");
  const [password, setPassword] = useState("");
  const [confirming, setConfirming] = useState(false);
  const [message, setMessage] = useState("");
  const [error, setError] = useState("");
  const [busy, setBusy] = useState(false);
  async function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault(); setBusy(true); setError(""); setMessage("");
    try {
      const response = await fetch(`/api/core/auth/password-reset/${confirming ? "confirm" : "request"}`, {
        method: "POST", headers: { "content-type": "application/json" },
        body: JSON.stringify(confirming ? { email, code, password } : { email }), cache: "no-store"
      });
      const data = await response.json().catch(() => ({}));
      if (!response.ok) throw new Error(typeof data.detail === "string" ? data.detail : "Permintaan tidak berhasil.");
      if (!confirming) setConfirming(true);
      setMessage(confirming ? "Kata sandi diperbarui. Silakan masuk kembali." : "Jika akun tersedia, kode reset akan dikirim ke email Anda.");
    } catch (reason) { setError(reason instanceof Error ? reason.message : "Permintaan tidak berhasil."); }
    finally { setBusy(false); }
  }
  return <SiteFrame><div className="shell form-wrap"><section className="panel form-card">
    <span className="eyebrow">Keamanan akun</span><h1>Reset kata sandi</h1>
    <p className="muted">Kami akan mengirim kode jika alamat email terhubung dengan akun.</p>
    <form onSubmit={submit}>
      <label className="field">Email<input autoComplete="email" required type="email" value={email} onChange={(event) => setEmail(event.target.value)} /></label>
      {confirming && <>
        <label className="field">Kode 6 digit<input inputMode="numeric" maxLength={6} required value={code} onChange={(event) => setCode(event.target.value)} /></label>
        <label className="field">Kata sandi baru<input autoComplete="new-password" minLength={12} required type="password" value={password} onChange={(event) => setPassword(event.target.value)} /></label>
      </>}
      {error && <p className="error" role="alert">{error}</p>}{message && <p className="success" role="status">{message}</p>}
      <button className="button primary" disabled={busy} type="submit">{busy ? "Memproses…" : confirming ? "Simpan kata sandi" : "Kirim kode"}</button>
    </form>
  </section></div></SiteFrame>;
}
