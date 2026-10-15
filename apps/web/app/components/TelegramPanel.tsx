"use client";

import { useEffect, useState } from "react";
import { apiRequest } from "../lib/api";

type TelegramStatus = {
  connected?: boolean;
  account_name?: string | null;
  state?: string;
  configured?: boolean;
};

type TelegramGroup = {
  chat_id: string | number;
  name: string;
  enabled?: boolean;
  alias?: string | null;
};

function errorMessage(reason: unknown): string {
  return reason instanceof Error ? reason.message : "Permintaan tidak berhasil.";
}

export function TelegramPanel() {
  const [statusText, setStatusText] = useState("Status: memeriksa...");
  const [notice, setNotice] = useState("");
  const [phone, setPhone] = useState("");
  const [code, setCode] = useState("");
  const [password, setPassword] = useState("");
  const [codeStep, setCodeStep] = useState(false);
  const [passwordStep, setPasswordStep] = useState(false);
  const [groups, setGroups] = useState<TelegramGroup[]>([]);
  const [groupsLoaded, setGroupsLoaded] = useState(false);
  const [saving, setSaving] = useState(false);

  async function loadGroups() {
    try {
      const result = await apiRequest<TelegramGroup[]>("telegram/groups");
      setGroups(Array.isArray(result) ? result : []);
      setGroupsLoaded(true);
    } catch (reason) {
      setNotice(errorMessage(reason));
    }
  }

  async function refreshStatus() {
    try {
      const status = await apiRequest<TelegramStatus>("telegram/status");
      setStatusText(
        status.connected
          ? `Status: connected — ${status.account_name || "akun Telegram"}`
          : `Status: ${status.state}${status.configured ? "" : " (API ID/HASH belum diatur pada environment backend)"}`,
      );
      if (status.connected) await loadGroups();
    } catch (reason) {
      setNotice(errorMessage(reason));
    }
  }

  useEffect(() => {
    void refreshStatus();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  async function sendCode() {
    try {
      const result = await apiRequest<{ phone?: string }>("telegram/send-code", {
        method: "POST",
        body: JSON.stringify({ phone }),
      });
      setCodeStep(true);
      setNotice(`Kode dikirim ke ${result.phone}.`);
    } catch (reason) {
      setNotice(errorMessage(reason));
    }
  }

  async function verifyCode() {
    try {
      const result = await apiRequest<{ requires_2fa?: boolean }>("telegram/verify-code", {
        method: "POST",
        body: JSON.stringify({ code }),
      });
      if (result.requires_2fa) setPasswordStep(true);
      else await refreshStatus();
      setNotice(result.requires_2fa ? "Masukkan password 2FA Telegram." : "Login Telegram berhasil.");
    } catch (reason) {
      setNotice(errorMessage(reason));
    }
  }

  async function verifyPassword() {
    try {
      await apiRequest("telegram/verify-2fa", {
        method: "POST",
        body: JSON.stringify({ password }),
      });
      setPassword("");
      await refreshStatus();
      setNotice("Login Telegram berhasil.");
    } catch (reason) {
      setNotice(errorMessage(reason));
    }
  }

  async function reconnect() {
    try {
      await apiRequest("telegram/reconnect", { method: "POST" });
      await refreshStatus();
    } catch (reason) {
      setNotice(errorMessage(reason));
    }
  }

  async function logoutTelegram() {
    try {
      await apiRequest("telegram/logout", { method: "POST" });
      await refreshStatus();
      setNotice("Telegram logout.");
    } catch (reason) {
      setNotice(errorMessage(reason));
    }
  }

  function toggleGroup(chatId: string | number, enabled: boolean) {
    setGroups((prev) => prev.map((group) => (group.chat_id === chatId ? { ...group, enabled } : group)));
  }

  function renameGroup(chatId: string | number, alias: string) {
    setGroups((prev) => prev.map((group) => (group.chat_id === chatId ? { ...group, alias } : group)));
  }

  async function saveGroups() {
    if (saving) return;
    setSaving(true);
    try {
      const rows = groups.map((group) => ({
        chat_id: group.chat_id,
        enabled: Boolean(group.enabled),
        alias: group.alias || null,
      }));
      const result = await apiRequest<{ disabled?: number }>("telegram/groups/select", {
        method: "POST",
        body: JSON.stringify({ groups: rows }),
      });
      await loadGroups();
      setNotice(
        result.disabled
          ? `Whitelist tersimpan; ${result.disabled} sumber lama dinonaktifkan.`
          : "Whitelist grup tersimpan.",
      );
    } catch (reason) {
      setNotice(errorMessage(reason));
    } finally {
      setSaving(false);
    }
  }

  return (
    <main className="page-shell dashboard-page telegram-page" id="main-content">
      <div className="page-heading">
        <div>
          <span className="eyebrow">Signal sources</span>
          <h1>Telegram Setup</h1>
          <p className="page-description">
            Hubungkan akun sumber, pilih grup yang boleh dibaca, dan beri alias agar sinyal mudah dilacak.
          </p>
        </div>
        <p id="status" className="heading-aside">
          {statusText}
        </p>
      </div>
      <section id="login">
        <h2>Hubungkan akun</h2>
        <p className="section-help">
          Masuk dengan nomor Telegram dalam format internasional. Kode login dan 2FA hanya dipakai untuk
          menghubungkan session lokal.
        </p>
        <div className="telegram-connect-fields">
          <label htmlFor="phone">
            Nomor telepon (format internasional)
            <small className="field-help">Kode login Telegram akan dikirim ke aplikasi Telegram akun ini.</small>
          </label>
          <input
            id="phone"
            type="tel"
            className="phone-input"
            autoComplete="tel"
            placeholder="+628123456789"
            pattern="\+[1-9][0-9]{7,14}"
            title="Masukkan nomor internasional, contoh +628123456789."
            required
            value={phone}
            onChange={(event) => setPhone(event.target.value)}
          />
          <button id="send-code" type="button" onClick={() => void sendCode()}>
            Kirim kode Telegram
          </button>
          <div id="code-step" hidden={!codeStep}>
            <label htmlFor="code">
              Kode Telegram
              <small className="field-help">Masukkan kode yang diterima untuk menyelesaikan login.</small>
            </label>
            <input
              id="code"
              autoComplete="one-time-code"
              inputMode="numeric"
              required={codeStep}
              value={code}
              onChange={(event) => setCode(event.target.value)}
            />
            <button id="verify-code" type="button" onClick={() => void verifyCode()}>
              Verifikasi kode
            </button>
          </div>
          <div id="password-step" hidden={!passwordStep}>
            <label htmlFor="password">
              Password 2FA
              <small className="field-help">Isi hanya jika verifikasi dua langkah aktif pada akun.</small>
            </label>
            <input
              id="password"
              type="password"
              autoComplete="current-password"
            required={passwordStep}
              value={password}
              onChange={(event) => setPassword(event.target.value)}
            />
            <button id="verify-password" type="button" onClick={() => void verifyPassword()}>
              Verifikasi 2FA
            </button>
          </div>
        </div>
        <div className="telegram-session-actions">
          <p className="section-help">
            Reconnect memulihkan koneksi; Logout Telegram mencabut session lokal; Muat grup mengambil daftar grup
            terbaru.
          </p>
          <button id="reconnect" className="secondary" type="button" onClick={() => void reconnect()}>
            Reconnect
          </button>
          <button id="telegram-logout" className="secondary" type="button" onClick={() => void logoutTelegram()}>
            Logout Telegram
          </button>
          <button id="refresh-groups" className="secondary" type="button" onClick={() => void loadGroups()}>
            Muat grup
          </button>
        </div>
      </section>
      <p id="notice" role="status">
        {notice}
      </p>
      <section>
        <h2>Grup dan channel</h2>
        <p className="section-help">
          Centang sumber sinyal yang dipercaya. Alias hanya mengubah nama tampilan, bukan ID grup Telegram.
        </p>
        <div id="groups" className="telegram-groups">
          {groupsLoaded ? (
            groups.length ? (
              <table>
                <thead>
                  <tr><th scope="col">Pilih</th><th scope="col">Grup atau channel</th><th scope="col">Alias</th></tr>
                </thead>
                <tbody>
                  {groups.map((group) => (
                    <tr key={String(group.chat_id)}>
                      <td>
                        <input
                          type="checkbox"
                          aria-label={`Aktifkan sumber ${group.name}`}
                          data-chat-id={String(group.chat_id)}
                          checked={Boolean(group.enabled)}
                          onChange={(event) => toggleGroup(group.chat_id, event.target.checked)}
                        />
                      </td>
                      <td>{group.name}</td>
                      <td>
                        <input
                          className="alias"
                          aria-label={`Alias untuk ${group.name}`}
                          placeholder="Alias (opsional)"
                          data-alias-for={String(group.chat_id)}
                          value={group.alias || ""}
                          onChange={(event) => renameGroup(group.chat_id, event.target.value)}
                        />
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            ) : (
              "Tidak ada grup atau channel."
            )
          ) : (
            "Login untuk memuat daftar grup."
          )}
        </div>
        <button id="save-groups" type="button" disabled={saving} aria-busy={saving} onClick={() => void saveGroups()}>
          {saving ? "Menyimpan…" : "Simpan whitelist"}
        </button>
      </section>
    </main>
  );
}
