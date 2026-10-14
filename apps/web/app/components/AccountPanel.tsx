"use client";

import Link from "next/link";
import { useRouter } from "next/navigation";
import { FormEvent, useEffect, useRef, useState } from "react";
import { apiRequest } from "../lib/api";
import { formatInteger, formatMoney, numberTone, toneClass } from "../lib/format";

type Plan = { name: string; vcpu: string; ram: string; disk: string; price_idr: number };
type ManagedCopy = {
  configured: boolean; login_status?: string; login_message?: string; follower_symbol?: string;
  active: boolean; status: string; status_message?: string | null; last_seen_at?: string | null;
  mode_lot?: string; ratio_lot?: number; lot_tetap?: number; max_lot_per_order?: number;
  max_lot_total?: number; max_open_positions?: number;
};
type Account = {
  id: number; label: string; server: string; login: string; plan: string; execution_mode: string;
  active: boolean; last_seen_at?: string | null; connection_status: string;
  balance?: number | null; equity?: number | null; floating_profit?: number | null;
  currency?: string | null; trade_mode?: string | null; state_updated_at?: string | null;
  managed_copy: ManagedCopy;
};
type BalanceGroup = {
  trade_mode: string; currency: string; balance: number; equity: number; floating_profit: number;
  online_accounts: number; reported_accounts: number; updated_at?: string | null;
};
type PerformanceGroup = {
  trade_mode: string; currency: string; realized_pnl: number; closed_trades: number;
  wins: number; losses: number; win_rate: number;
};
type Performance = { balance_groups: BalanceGroup[]; performance_groups: PerformanceGroup[]; updated_at?: string | null };
type CopierStatus = { configured: boolean; available: boolean; status: string; last_seen_at?: string | null };

const rupiah = (value: number | null | undefined) =>
  value === null || value === undefined
    ? "—"
    : `Rp ${Number(value).toLocaleString("id-ID", { maximumFractionDigits: 0 })}`;
const wibTime = (value?: string | null) =>
  value ? `${new Date(value).toLocaleString("id-ID", { timeZone: "Asia/Jakarta" })} WIB` : "—";
const modeLabel = (mode?: string | null) =>
  ({ real: "Live", demo: "Demo", contest: "Kompetisi" } as Record<string, string>)[mode ?? ""] ?? mode ?? "—";

const CONNECTION_LABELS: Record<string, string> = {
  ready: "Siap menyalin: EA Copy Trading dan akun utama terhubung.",
  waiting_for_source: "EA Copy Trading terhubung; menunggu pembaruan dari akun utama.",
  offline: "EA Copy Trading belum terhubung. Periksa token, ServerURL, dan Allow WebRequest.",
  trading_disabled: "Koneksi aktif, tetapi izin trading MT5 belum aktif.",
  not_real_account: "Mode akun MT5 belum dikenali.",
  disabled: "Akun Copy Trading ini dinonaktifkan.",
};
const MANAGED_STATUS_LABELS: Record<string, string> = {
  SETUP_REQUIRED: "Masukkan password broker dan atur batas copy terlebih dahulu.",
  PAUSED: "Copy trading dijeda. Tidak ada order baru yang dikirim.",
  STARTING: "Permintaan diterima. Menunggu worker copier mengambil job.",
  RUNNING: "Worker copier tersambung. Periksa terminal dan akun broker secara berkala.",
  OFFLINE: "Worker copier tidak mengirim heartbeat terbaru. Akun tidak sedang disinkronkan.",
  ERROR: "Worker copier melaporkan kendala. Periksa server dan akun broker.",
  STOPPED: "Worker copier berhenti. Periksa konfigurasi server.",
};
const LOGIN_CHECK_LABELS: Record<string, string> = {
  UNCHECKED: "Login broker belum diperiksa. Simpan kredensial untuk memulai pemeriksaan otomatis.",
  CHECKING: "Sedang memeriksa login broker dan kesiapan terminal MT5…",
  VALID: "Login broker berhasil dan terminal siap.",
  ERROR: "Login belum berhasil. Periksa nomor akun, password trading, dan server broker.",
};

function ManagedCopyForm({ account, onDirty, onSaved, onError }: {
  account: Account; onDirty: () => void; onSaved: () => void; onError: (message: string) => void;
}) {
  const profile = account.managed_copy;
  const [busy, setBusy] = useState(false);
  async function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    setBusy(true);
    onError("");
    try {
      const data = Object.fromEntries(new FormData(event.currentTarget).entries()) as Record<string, string>;
      const payload: Record<string, string | number> = { ...data };
      for (const key of ["ratio_lot", "lot_tetap", "max_lot_per_order", "max_lot_total", "max_open_positions"]) {
        payload[key] = Number(data[key]);
      }
      await apiRequest(`mt5/accounts/${account.id}/managed-copy`, { method: "PUT", body: JSON.stringify(payload) });
      onSaved();
    } catch (reason) {
      onError(reason instanceof Error ? reason.message : "Gagal menyimpan pengaturan.");
    } finally {
      setBusy(false);
    }
  }
  const num = (name: string, label: string, value: number | undefined, min: string, max: string, step: string) => (
    <label className="field" key={name}>{label}
      <input name={name} type="number" defaultValue={value} min={min} max={max} step={step} required onInput={onDirty} onChange={onDirty} />
    </label>
  );
  return (
    <form onSubmit={(event) => void submit(event)}>
      <label className="field">Password akun broker
        <input name="broker_password" type="password" autoComplete="off" maxLength={128} required
          placeholder={profile.configured ? "Masukkan ulang untuk mengganti password tersimpan" : "Password trading akun follower"}
          onInput={onDirty} onChange={onDirty} />
      </label>
      <label className="field">Nama simbol XAUUSD di broker
        <input name="follower_symbol" defaultValue={profile.follower_symbol ?? "XAUUSD"} maxLength={64}
          pattern="[A-Za-z0-9._-]+" onInput={onDirty} onChange={onDirty} />
      </label>
      <label className="field">Metode ukuran lot
        <select name="mode_lot" defaultValue={profile.mode_lot ?? "rasio"} onInput={onDirty} onChange={onDirty}>
          <option value="rasio">Rasio terhadap lot sumber</option>
          <option value="tetap">Lot tetap</option>
        </select>
      </label>
      {num("ratio_lot", "Pengali lot rasio", profile.ratio_lot ?? 1, "0.01", "5", "0.01")}
      {num("lot_tetap", "Lot tetap", profile.lot_tetap ?? 0.01, "0.01", "5", "0.01")}
      {num("max_lot_per_order", "Batas lot per order", profile.max_lot_per_order ?? 0.05, "0.01", "5", "0.01")}
      {num("max_lot_total", "Batas total lot terbuka", profile.max_lot_total ?? 0.1, "0.01", "10", "0.01")}
      {num("max_open_positions", "Maksimum posisi terbuka", profile.max_open_positions ?? 3, "1", "50", "1")}
      <button className="button primary" disabled={busy} type="submit">
        {busy ? "Menyimpan…" : profile.configured ? "Simpan pengaturan" : "Simpan kredensial dan pengaturan"}
      </button>
    </form>
  );
}

function EaInstallGuide() {
  return (
    <section className="panel section">
      <span className="eyebrow">Panduan instalasi</span>
      <h2>Pasang EA Copy Trading</h2>
      <p className="muted">Petunjuk ini berlaku untuk akun yang memilih menjalankan EA sendiri. Akun yang dikelola server tidak perlu mengikuti langkah MT5 ini di komputer client.</p>
      <div className="actions">
        <a className="button" href="/api/core/downloads/MT5FollowerCopyEA.mq5" download>Unduh source EA Copy Trading</a>
      </div>
      <ol className="muted">
        <li>Hubungkan akun Copy Trading di atas. Salin token yang muncul; token hanya terlihat saat dibuat atau dirotasi.</li>
        <li>Di MT5, pilih <strong>File → Open Data Folder</strong>, buka folder <code>MQL5/Experts</code>, lalu salin file <code>.mq5</code> yang diunduh ke sana. Buka di MetaEditor dan tekan <strong>F7</strong>.</li>
        <li>Di MT5, pilih <strong>Tools → Options → Expert Advisors</strong>, aktifkan <em>Allow WebRequest for listed URL</em>, lalu tambahkan URL Core API Anda.</li>
        <li>Dari <strong>Navigator → Expert Advisors</strong>, pasang EA di chart. Isi <code>AccountToken</code> dengan token akun, pastikan <code>ServerURL</code> benar, lalu aktifkan <strong>Algo Trading</strong>.</li>
        <li>Pastikan EA utama Algentra aktif. Atur <code>VolumeMultiplier</code> sesuai kebutuhan sebelum menjalankannya.</li>
      </ol>
      <p className="muted"><strong>HTTP -1 / ERROR 4006:</strong> EA belum mendapat jawaban dari server. Pastikan URL <code>ServerURL</code> sama dengan alamat yang diizinkan di <em>Allow WebRequest</em>. <strong>TRADING OFF:</strong> periksa Algo Trading serta izin trading terminal, akun, dan EA.</p>
      <p className="muted">Akun Copy Trading harus memakai mode <strong>hedging</strong>. Jangan bagikan token EA. Jika token hilang atau terekspos, pilih <strong>Rotasi token</strong> dan perbarui token di EA.</p>
    </section>
  );
}

function AccountCard({ account, plans, copierAvailable, onChanged, onError, onShowToken, onDirty }: {
  account: Account; plans: Record<string, Plan>; copierAvailable: boolean;
  onChanged: () => void; onError: (message: string) => void; onShowToken: (label: string, token: string) => void;
  onDirty: () => void;
}) {
  const [busy, setBusy] = useState(false);
  const plan = plans[account.plan];
  const managed = account.execution_mode === "MANAGED";
  const profile = account.managed_copy;

  async function run(label: string, fn: () => Promise<unknown>) {
    setBusy(true);
    onError("");
    try {
      await fn();
      onChanged();
    } catch (reason) {
      onError(`${label}: ${reason instanceof Error ? reason.message : "gagal."}`);
    } finally {
      setBusy(false);
    }
  }

  function editAccount() {
    const label = prompt("Nama akun", account.label);
    if (label === null) return;
    const server = prompt("Broker server", account.server);
    if (server === null) return;
    const login = prompt("Nomor login MT5", account.login);
    if (login === null) return;
    void run("Edit akun", () => apiRequest(`mt5/accounts/${account.id}`, {
      method: "PUT", body: JSON.stringify({ label, server, login }),
    }));
  }

  function toggleManaged() {
    const active = !profile.active;
    if (active && !window.confirm("Konfirmasi: job ini akan mengirim order live ke akun broker Anda. Pastikan akun demo sudah diuji, login/server benar, serta batas lot dan posisi sesuai. Lanjutkan?")) return;
    void run(active ? "Mulai copy" : "Jeda copy", () => apiRequest(`mt5/accounts/${account.id}/managed-copy/active`, {
      method: "PUT", body: JSON.stringify({ active, acknowledge_live_risk: active }),
    }));
  }

  function removeAccount() {
    if (!window.confirm(`Hapus akun ${account.label} (${account.login})?`)) return;
    void run("Hapus akun", () => apiRequest(`mt5/accounts/${account.id}`, { method: "DELETE" }));
  }

  const balanceLine = account.balance === null || account.balance === undefined
    ? <p className="muted">{managed ? "Saldo follower menunggu laporan worker." : "Saldo MT5: menunggu laporan EA."}</p>
    : <p>
      Saldo <strong>{formatMoney(account.balance, account.currency)}</strong>
      {" · "}Ekuitas <strong>{formatMoney(account.equity, account.currency)}</strong>
      {" · "}Floating <strong className={toneClass(numberTone(account.floating_profit))}>{formatMoney(account.floating_profit, account.currency)}</strong>
    </p>;

  return (
    <article className="card">
      <h3>{account.label}</h3>
      <p className="muted">
        {managed ? `Dikelola server · ${account.login} · ${account.server}` : `Akun Copy Trading · ${account.login} · ${account.server}`}
        {plan ? ` · ${plan.name}` : ` · ${account.plan}`}
      </p>
      {managed && (
        <>
          <p className="muted">
            {profile.active ? "Copy aktif" : "Copy dijeda"}
            {" · "}Worker terakhir terhubung: {profile.last_seen_at ? wibTime(profile.last_seen_at) : "belum terhubung"}
          </p>
          <p className={toneClass(profile.login_status === "VALID" ? "pos" : profile.login_status === "ERROR" ? "neg" : "warn")} role="status">
            {profile.login_message || LOGIN_CHECK_LABELS[profile.login_status ?? ""] || LOGIN_CHECK_LABELS.UNCHECKED}
          </p>
          <p className="muted" role="status">{MANAGED_STATUS_LABELS[profile.status] ?? profile.status_message ?? MANAGED_STATUS_LABELS.SETUP_REQUIRED}</p>
          <ManagedCopyForm
            account={account}
            onDirty={onDirty}
            onSaved={() => { onError(""); onChanged(); }}
            onError={onError}
          />
        </>
      )}
      {!managed && (
        <>
          {balanceLine}
          <p className="muted">Terakhir terhubung: {account.last_seen_at ? wibTime(account.last_seen_at) : "belum ada koneksi"}</p>
          <p role="status"><strong className={toneClass(account.connection_status === "ready" ? "pos" : account.connection_status === "disabled" ? "muted" : "warn")}>
            {CONNECTION_LABELS[account.connection_status] ?? account.connection_status}
          </strong></p>
        </>
      )}
      <div className="actions">
        {managed ? (
          <button className="button primary" disabled={busy || !profile.configured || (!profile.active && (!copierAvailable || profile.login_status !== "VALID"))} onClick={toggleManaged}>
            {busy ? "Memproses…" : profile.active ? "Jeda copy trading" : "Mulai copy trading (live)"}
          </button>
        ) : (
          <>
            <button className="button" disabled={busy} onClick={editAccount}>Edit akun</button>
            <button className="button" disabled={busy} onClick={() => void run(account.active ? "Nonaktifkan" : "Aktifkan", () =>
              apiRequest(`mt5/accounts/${account.id}/active`, { method: "PUT", body: JSON.stringify({ active: !account.active }) }))}>
              {account.active ? "Nonaktifkan" : "Aktifkan"}
            </button>
            <button className="button" disabled={busy} onClick={() => void (async () => {
              setBusy(true);
              onError("");
              try {
                const result = await apiRequest<{ token: string }>(`mt5/accounts/${account.id}/rotate-token`, { method: "POST" });
                onShowToken(account.label, result.token);
                onChanged();
              } catch (reason) {
                onError(`Rotasi token: ${reason instanceof Error ? reason.message : "gagal."}`);
              } finally {
                setBusy(false);
              }
            })()}>Rotasi token</button>
          </>
        )}
        <button className="button" disabled={busy} onClick={removeAccount}>Hapus akun</button>
      </div>
    </article>
  );
}

export function AccountPanel() {
  const router = useRouter();
  const [accounts, setAccounts] = useState<Account[]>([]);
  const [plans, setPlans] = useState<Record<string, Plan>>({});
  const [performance, setPerformance] = useState<Performance | null>(null);
  const [copier, setCopier] = useState<CopierStatus | null>(null);
  const [error, setError] = useState("");
  const [message, setMessage] = useState("");
  const [loading, setLoading] = useState(true);
  const [token, setToken] = useState<{ label: string; value: string } | null>(null);
  const [executionMode, setExecutionMode] = useState("EA");
  const [planCode, setPlanCode] = useState("ZERO");
  const [formBusy, setFormBusy] = useState(false);
  const dirtyRef = useRef(false);

  async function loadAll() {
    try {
      const [accountList, perf, copierStatus, planList] = await Promise.all([
        apiRequest<Account[]>("mt5/accounts"),
        apiRequest<Performance>("mt5/performance"),
        apiRequest<CopierStatus>("mt5/copier/status"),
        apiRequest<Record<string, Plan>>("mt5/plans"),
      ]);
      setAccounts(accountList);
      setPerformance(perf);
      setCopier(copierStatus);
      setPlans(planList);
      if (!copierStatus.available && executionMode === "MANAGED") setExecutionMode("EA");
      const codes = Object.keys(planList);
      if (codes.length && !planList[planCode]) setPlanCode(codes[0]);
    } catch (reason) {
      setError(reason instanceof Error ? reason.message : "Gagal memuat akun.");
    } finally {
      setLoading(false);
    }
  }

  useEffect(() => {
    void loadAll();
    const timer = setInterval(() => {
      if (!document.hidden && !dirtyRef.current) void loadAll();
    }, 5000);
    return () => clearInterval(timer);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  async function submitOrder(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    setFormBusy(true);
    setError("");
    try {
      const data = Object.fromEntries(new FormData(event.currentTarget).entries()) as Record<string, string>;
      if (!/^\d+$/.test(data.login)) throw new Error("Nomor akun MT5 hanya boleh berisi angka, tanpa spasi.");
      const order = await apiRequest<{ id: number }>("payments/orders", { method: "POST", body: JSON.stringify(data) });
      router.push(`/transactions?order=${encodeURIComponent(order.id)}`);
    } catch (reason) {
      setError(reason instanceof Error ? reason.message : "Gagal membuat pesanan.");
    } finally {
      setFormBusy(false);
    }
  }

  const planEntries = Object.entries(plans);
  const hasEaAccount = accounts.some((account) => account.execution_mode !== "MANAGED");

  return (
    <>
      {token && (
        <section className="panel section" role="status">
          <span className="eyebrow">Token EA</span>
          <h2>Token baru: {token.label}</h2>
          <p className="muted">Salin token ini sekarang, lalu tempel ke input AccountToken pada EA MT5. Token tidak ditampilkan lagi setelah panel ditutup.</p>
          <p><code>{token.value}</code></p>
          <div className="actions">
            <button className="button primary" onClick={() => void navigator.clipboard.writeText(token.value)}>Salin token</button>
            <button className="button" onClick={() => setToken(null)}>Tutup</button>
          </div>
        </section>
      )}

      <section className="panel section">
        <span className="eyebrow">Portal klien · Copy trading</span>
        <h2>Hubungkan akun Copy Trading</h2>
        <p className="muted">Pilih cara menjalankan akun, paket, dan detail akun broker. Akun dibuat setelah pembayaran disetujui. Untuk layanan dikelola server, password broker dienkripsi dan hanya dikirim ke worker copier yang terautentikasi.</p>
        <form onSubmit={(event) => void submitOrder(event)}>
          <fieldset>
            <legend className="muted">Cara menjalankan copy trading</legend>
            <label className="field">
              <span>
                <input type="radio" name="execution_mode" value="MANAGED" checked={executionMode === "MANAGED"}
                  disabled={!copier?.available} onChange={() => setExecutionMode("MANAGED")} required />
                {" "}Dikelola server {!copier?.available && <span className="muted">(belum tersedia)</span>}
              </span>
              <span className="muted">Pengaturan melalui website. MT5 berjalan di server layanan, bukan di komputer Anda.</span>
            </label>
            <label className="field">
              <span>
                <input type="radio" name="execution_mode" value="EA" checked={executionMode === "EA"}
                  onChange={() => setExecutionMode("EA")} required />
                {" "}Pasang EA sendiri
              </span>
              <span className="muted">Anda menjalankan terminal MT5 dan EA Copy Trading di perangkat/VPS sendiri.</span>
            </label>
          </fieldset>
          <p className="muted" role="status">
            {copier ? (copier.available
              ? "Worker managed copy online. Akun managed dapat didaftarkan dan dijalankan."
              : "Worker managed copy offline. Layanan managed belum dapat dipesan atau dimulai.")
              : "Memeriksa status worker…"}
          </p>
          <fieldset>
            <legend className="muted">Pilih paket layanan</legend>
            {planEntries.map(([code, plan]) => (
              <label className="field" key={code}>
                <span>
                  <input type="radio" name="plan" value={code} checked={planCode === code}
                    onChange={() => setPlanCode(code)} required />
                  {" "}<strong>{plan.name}</strong> — {plan.vcpu} · RAM {plan.ram} · Disk {plan.disk} · <strong>{rupiah(plan.price_idr)}</strong>
                </span>
              </label>
            ))}
          </fieldset>
          <p className="muted">Total mengikuti harga paket yang dipilih. Pembayaran sementara dilakukan manual dan diperiksa admin. Jangan transfer sebelum melihat instruksi pada pesanan Anda.</p>
          <label className="field">Nama akun
            <input name="label" maxLength={80} placeholder="Copy Trading 1" required />
          </label>
          <label className="field">Server broker
            <input name="server" maxLength={128} placeholder="BrokerName-Live" required />
          </label>
          <label className="field">Nomor akun broker
            <input name="login" maxLength={32} inputMode="numeric" placeholder="12345678" required />
            <span className="muted">Masukkan nomor login MT5 berupa angka saja, tanpa spasi.</span>
          </label>
          {error && <p className="error" role="alert">{error}</p>}
          {message && <p className="success" role="status">{message}</p>}
          <button className="button primary" disabled={formBusy} type="submit">
            {formBusy ? "Memproses…" : "Lanjutkan ke pembayaran"}
          </button>
        </form>
        <div className="actions">
          <Link className="button" href="/transactions">Buka riwayat transaksi dan nota</Link>
        </div>
      </section>

      <section className="panel section">
        <span className="eyebrow">Ringkasan</span>
        <h2>Saldo dan performa akun</h2>
        <p className="muted">Worker managed melaporkan saldo, ekuitas, posisi terbuka, dan transaksi follower ke portal.</p>
        {loading && <p className="muted">Memuat…</p>}
        {performance && (
          <>
            <div className="cards">
              {performance.balance_groups.map((group, index) => (
                <article className="card" key={`${group.trade_mode}-${group.currency}-${index}`}>
                  <span className="eyebrow">Saldo {modeLabel(group.trade_mode)} · {group.currency}</span>
                  <h2>{formatMoney(group.balance, group.currency)}</h2>
                  <p className="muted">Ekuitas {formatMoney(group.equity, group.currency)} · Floating{" "}
                    <span className={toneClass(numberTone(group.floating_profit))}>{formatMoney(group.floating_profit, group.currency)}</span></p>
                  <p className="muted">{formatInteger(group.online_accounts)} dari {formatInteger(group.reported_accounts)} akun melapor</p>
                </article>
              ))}
              {performance.balance_groups.length === 0 && <p className="muted">Saldo akan tampil setelah EA Copy Trading mengirim laporan dari MT5.</p>}
            </div>
            <div className="cards">
              {performance.performance_groups.map((group, index) => (
                <article className="card" key={`${group.trade_mode}-${group.currency}-${index}`}>
                  <span className="eyebrow">Performa {modeLabel(group.trade_mode)} · {group.currency}</span>
                  <h2 className={toneClass(numberTone(group.realized_pnl))}>{formatMoney(group.realized_pnl, group.currency)}</h2>
                  <p className="muted">{formatInteger(group.closed_trades)} transaksi ditutup · {formatInteger(group.wins)} menang · {formatInteger(group.losses)} kalah</p>
                </article>
              ))}
            </div>
            {performance.updated_at && <p className="muted">Pembaruan MT5 terakhir {wibTime(performance.updated_at)}.</p>}
          </>
        )}
      </section>

      <section className="panel section">
        <span className="eyebrow">Kelola</span>
        <h2>Akun Copy Trading Anda</h2>
        <p className="muted">Kelola hingga 10 akun. Pilih <strong>Mulai copy trading</strong> hanya setelah mengecek login, ukuran lot, dan batas risiko. Job server akan menjalankan order live pada akun broker Anda.</p>
        {loading && <p className="muted">Memuat akun…</p>}
        {!loading && accounts.length === 0 && <p className="muted">Belum ada akun MT5 yang terhubung.</p>}
        <div className="cards">
          {accounts.map((account) => (
            <AccountCard
              key={account.id}
              account={account}
              plans={plans}
              copierAvailable={copier?.available ?? false}
              onChanged={() => { dirtyRef.current = false; void loadAll(); }}
              onError={(msg) => { setMessage(""); setError(msg); }}
              onShowToken={(label, value) => setToken({ label, value })}
              onDirty={() => { dirtyRef.current = true; }}
            />
          ))}
        </div>
      </section>

      {hasEaAccount && <EaInstallGuide />}
    </>
  );
}
