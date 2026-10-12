"use client";

import Link from "next/link";
import { useRouter } from "next/navigation";
import { FormEvent, useState } from "react";

type AuthMode = "client" | "admin" | "register";

async function post(path: string, values: Record<string, string | boolean>) {
  const response = await fetch(`/api/core/${path}`, {
    method: "POST",
    headers: { "content-type": "application/json" },
    body: JSON.stringify(values),
    cache: "no-store"
  });
  const data = await response.json().catch(() => ({}));
  if (!response.ok) throw new Error(typeof data.detail === "string" ? data.detail : "Permintaan tidak berhasil.");
  return data as Record<string, unknown>;
}

export function AuthForm({ mode }: { mode: AuthMode }) {
  const router = useRouter();
  const [email, setEmail] = useState("");
  const [password, setPassword] = useState("");
  const [code, setCode] = useState("");
  const [verifying, setVerifying] = useState(false);
  const [busy, setBusy] = useState(false);
  const [message, setMessage] = useState("");
  const [error, setError] = useState("");
  const isAdmin = mode === "admin";

  async function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    setBusy(true);
    setError("");
    setMessage("");
    try {
      if (mode === "register" && !verifying) {
        await post("auth/register", { email, password });
        setVerifying(true);
        setMessage("Kode verifikasi dikirim jika pendaftaran tersedia untuk email ini.");
        return;
      }
      if (mode === "register") {
        const result = await post("auth/verify-email", { email, code });
        if (typeof result.csrf_token === "string") sessionStorage.setItem("algentra-csrf", result.csrf_token);
        setMessage("Email berhasil diverifikasi. Silakan masuk.");
        router.push("/login");
        return;
      }
      const result = await post(isAdmin ? "auth/login" : "auth/client-login", isAdmin
        ? { username: email, password }
        : { email, password, remember_me: false });
      if (typeof result.csrf_token === "string") sessionStorage.setItem("algentra-csrf", result.csrf_token);
      router.push(isAdmin ? "/dashboard" : "/account");
      router.refresh();
    } catch (reason) {
      setError(reason instanceof Error ? reason.message : "Permintaan tidak berhasil.");
    } finally {
      setBusy(false);
    }
  }

  const title = mode === "register" ? (verifying ? "Verifikasi email" : "Buat akun") : isAdmin ? "Login admin" : "Masuk ke portal";
  return (
    <div className="shell form-wrap">
      <section className="panel form-card">
        <span className="eyebrow">Algentra Capital</span>
        <h1>{title}</h1>
        <p className="muted">{mode === "register" ? "Buat akses portal menggunakan alamat email Anda." : "Akses portal dan pengaturan akun Anda."}</p>
        <form onSubmit={submit}>
          {!verifying && <label className="field">{isAdmin ? "Username" : "Email"}
            <input autoComplete={isAdmin ? "username" : "email"} required type={isAdmin ? "text" : "email"} value={email} onChange={(event) => setEmail(event.target.value)} />
          </label>}
          {mode === "register" && verifying && <label className="field">Email
            <input autoComplete="email" required type="email" value={email} onChange={(event) => setEmail(event.target.value)} />
          </label>}
          {verifying
            ? <label className="field">Kode 6 digit<input inputMode="numeric" maxLength={6} required value={code} onChange={(event) => setCode(event.target.value)} /></label>
            : <label className="field">Kata sandi<input autoComplete={isAdmin ? "current-password" : mode === "register" ? "new-password" : "current-password"} minLength={mode === "register" ? 12 : 1} required type="password" value={password} onChange={(event) => setPassword(event.target.value)} /></label>}
          {error && <p className="error" role="alert">{error}</p>}
          {message && <p className="success" role="status">{message}</p>}
          <button className="button primary" disabled={busy} type="submit">{busy ? "Memproses…" : verifying ? "Verifikasi" : mode === "register" ? "Kirim kode" : "Masuk"}</button>
        </form>
        {!isAdmin && <p className="muted">
          {mode === "register" ? <>Sudah punya akun? <Link className="text-link" href="/login">Masuk</Link></> : <>Belum terdaftar? <Link className="text-link" href="/register">Buat akun</Link> · <Link className="text-link" href="/forgot-password">Reset kata sandi</Link></>}
        </p>}
      </section>
    </div>
  );
}
