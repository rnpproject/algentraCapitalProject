"use client";

import { Fragment, useCallback, useEffect, useRef, useState } from "react";
import { apiRequest } from "../lib/api";
import {
  formatInteger,
  formatMoney,
  numberTone,
  toneClass,
  type Tone,
} from "../lib/format";
import { CsChatWidget } from "./CsChatWidget";

type Plan = {
  name: string;
  vcpu: string;
  ram: string;
  disk: string;
  price_idr: number;
};

type Pricing = {
  management_fee_idr: number;
  copy_trading_fee_idr: number;
  vps_options: VpsOption[];
};

type VpsOption = {
  code: string;
  name: string;
  vcpu: string;
  ram: string;
  disk: string;
  price_idr: number;
};

type ManagedCopy = {
  configured: boolean;
  active: boolean;
  status: string;
  status_message?: string | null;
  login_status?: string | null;
  login_message?: string | null;
  last_seen_at?: string | null;
  follower_symbol?: string | null;
  mode_lot?: string | null;
  ratio_lot?: number | null;
  lot_tetap?: number | null;
  max_lot_per_order?: number | null;
  max_lot_total?: number | null;
  max_open_positions?: number | null;
};

type Account = {
  id: number;
  label: string;
  server: string;
  login: string;
  plan: string;
  execution_mode: string;
  active: boolean;
  last_seen_at?: string | null;
  connection_status: string;
  balance?: number | null;
  equity?: number | null;
  floating_profit?: number | null;
  currency?: string | null;
  trade_mode?: string | null;
  state_updated_at?: string | null;
  managed_copy?: ManagedCopy | null;
};

type BalanceGroup = {
  trade_mode: string;
  currency: string;
  balance: number;
  equity: number;
  floating_profit: number;
  online_accounts: number;
  reported_accounts: number;
  updated_at?: string | null;
};

type PerformanceGroup = {
  trade_mode: string;
  currency: string;
  realized_pnl: number;
  closed_trades: number;
  wins: number;
  losses: number;
  win_rate: number;
};

type Performance = {
  balance_groups: BalanceGroup[];
  performance_groups: PerformanceGroup[];
  updated_at?: string | null;
};

type CopierStatus = {
  configured: boolean;
  available: boolean;
  status?: string | null;
  last_seen_at?: string | null;
};

type TokenBox = { title: string; token: string; accountId: number } | null;
type BrokerServerOption = { name: string; company: string };

type MetricItem = [string, number | null | undefined, (string | null | undefined)?, Tone?];

const ZERO_PLAN: Plan = { name: "—", vcpu: "—", ram: "—", disk: "—", price_idr: 0 };

const wib = (value?: string | null) =>
  value
    ? new Date(value).toLocaleString("id-ID", { timeZone: "Asia/Jakarta" })
    : "";

const modeLabel = (mode?: string | null) =>
  ({ real: "Live", demo: "Demo", contest: "Kompetisi" } as Record<string, string>)[
    mode ?? ""
  ] ?? mode ?? "MT5";

const CONNECTION_LABELS: Record<string, string> = {
  ready: "EA Copy Trading dan akun utama terhubung.",
  waiting_for_source:
    "EA Copy Trading terhubung; menunggu pembaruan dari akun utama.",
  offline:
    "EA Copy Trading belum terhubung. Periksa token, ServerURL, dan Allow WebRequest.",
  trading_disabled: "Koneksi aktif, tetapi izin trading MT5 belum aktif.",
  not_real_account: "Mode akun MT5 belum dikenali.",
  disabled: "Akun Copy Trading ini dinonaktifkan.",
};

function Metrics({ items }: { items: MetricItem[] }) {
  return (
    <>
      {items.map(([label, value, currency, tone], index) => (
        <Fragment key={index}>
          {index > 0 ? " · " : null}
          {`${label} `}
          <span className={toneClass(tone ?? "")}>
            {formatMoney(value, currency)}
          </span>
        </Fragment>
      ))}
    </>
  );
}

function EaAccountCard({
  account,
  plan,
  onMessage,
  onReload,
  onToken,
  clearTokenForAccount,
}: {
  account: Account;
  plan: Plan;
  onMessage: (message: string) => void;
  onReload: () => void;
  onToken: (title: string, token: string, accountId: number) => void;
  clearTokenForAccount: (accountId: number) => void;
}) {
  const connectionStatus =
    CONNECTION_LABELS[account.connection_status] !== undefined
      ? account.connection_status
      : "offline";
  const detail = `Akun Copy Trading · ${account.login} · ${account.server} · ${
    plan.name
  } (${plan.vcpu}, RAM ${plan.ram}, Disk ${plan.disk}) · Rp ${formatInteger(
    plan.price_idr
  )}`;
  const stateText = `${account.active ? "Aktif" : "Nonaktif"} · Terakhir terhubung: ${
    account.last_seen_at ? wib(account.last_seen_at) : "belum ada koneksi"
  }`;
  const balanceTimeText = account.state_updated_at
    ? `Data MT5 diperbarui ${wib(account.state_updated_at)} WIB`
    : "Hubungkan EA agar saldo dan histori MT5 tersinkron.";
  const connectionText =
    connectionStatus === "ready"
      ? `Siap menyalin pada akun ${modeLabel(account.trade_mode)}: ${
          CONNECTION_LABELS.ready
        }`
      : CONNECTION_LABELS[connectionStatus];

  const handleEdit = async () => {
    const label = prompt("Nama akun", account.label);
    if (label === null) return;
    const server = prompt("Broker server", account.server);
    if (server === null) return;
    const login = prompt("Nomor login MT5", account.login);
    if (login === null) return;
    try {
      await apiRequest(`mt5/accounts/${account.id}`, {
        method: "PUT",
        body: JSON.stringify({ label, server, login }),
      });
      onReload();
    } catch (error) {
      onMessage(error instanceof Error ? error.message : "Permintaan tidak berhasil.");
    }
  };

  const handleToggleActive = async () => {
    try {
      await apiRequest(`mt5/accounts/${account.id}/active`, {
        method: "PUT",
        body: JSON.stringify({ active: !account.active }),
      });
      onReload();
    } catch (error) {
      onMessage(error instanceof Error ? error.message : "Permintaan tidak berhasil.");
    }
  };

  const handleRotateToken = async () => {
    try {
      const result = await apiRequest<{ token: string }>(
        `mt5/accounts/${account.id}/rotate-token`,
        { method: "POST" }
      );
      onToken(`Token baru: ${account.label}`, result.token, account.id);
      onReload();
    } catch (error) {
      onMessage(error instanceof Error ? error.message : "Permintaan tidak berhasil.");
    }
  };

  const handleRemove = async () => {
    if (
      !confirm(
        `Hapus akun Copy Trading ${account.label} (${account.login})? Token terminal ini tidak akan bisa digunakan lagi.`
      )
    )
      return;
    onMessage("");
    try {
      await apiRequest(`mt5/accounts/${account.id}`, { method: "DELETE" });
      clearTokenForAccount(account.id);
      onMessage(`Akun ${account.label} berhasil dihapus.`);
      onReload();
    } catch (error) {
      onMessage(error instanceof Error ? error.message : "Permintaan tidak berhasil.");
    }
  };

  return (
    <article className="card">
      <h3>{account.label}</h3>
      <p>{detail}</p>
      <p className="account-balance">
        {account.balance == null ? (
          "Saldo MT5: menunggu laporan EA"
        ) : (
          <Metrics
            items={[
              ["Saldo", account.balance, account.currency],
              ["Ekuitas", account.equity, account.currency],
              [
                "Floating",
                account.floating_profit,
                account.currency,
                numberTone(account.floating_profit),
              ],
            ]}
          />
        )}
      </p>
      <small className="muted account-balance-time">{balanceTimeText}</small>
      <p className="muted">{stateText}</p>
      <div
        className={`account-state account-state--${connectionStatus}`}
        role="status"
      >
        <span className="account-state__dot" aria-hidden="true">
          {connectionStatus === "ready" ? "✓" : ""}
        </span>
        <span>{connectionText}</span>
      </div>
      <div className="toolbar">
        <button type="button" className="secondary" onClick={handleEdit}>
          Edit akun
        </button>
        <button
          type="button"
          className={account.active ? "secondary" : ""}
          onClick={handleToggleActive}
        >
          {account.active ? "Nonaktifkan" : "Aktifkan"}
        </button>
        <button type="button" className="secondary" onClick={handleRotateToken}>
          Rotasi token
        </button>
        <button
          type="button"
          className="danger icon-button"
          title={`Hapus akun ${account.label}`}
          aria-label={`Hapus akun ${account.label}`}
          onClick={handleRemove}
        >
          <svg
            viewBox="0 0 24 24"
            width="18"
            height="18"
            aria-hidden="true"
            fill="none"
            stroke="currentColor"
            strokeWidth="2"
            strokeLinecap="round"
            strokeLinejoin="round"
          >
            <path d="M3 6h18"></path>
            <path d="M8 6V4h8v2"></path>
            <path d="M19 6l-1 14H6L5 6"></path>
            <path d="M10 11v6M14 11v6"></path>
          </svg>
        </button>
      </div>
    </article>
  );
}
function ManagedAccountCard({
  account,
  plan,
  managedWorkerReady,
  onDirty,
  onMessage,
  onReload,
}: {
  account: Account;
  plan: Plan;
  managedWorkerReady: boolean;
  onDirty: () => void;
  onMessage: (message: string) => void;
  onReload: () => void;
}) {
  const profile: ManagedCopy = account.managed_copy ?? {
    configured: false,
    active: false,
    status: "SETUP_REQUIRED",
  };
  const detail = `Dikelola server · ${account.login} · ${account.server} · ${plan.name}`;
  const stateText = `${
    profile.active ? "Copy aktif" : "Copy dijeda"
  } · Worker terakhir terhubung: ${
    profile.last_seen_at ? wib(profile.last_seen_at) : "belum terhubung"
  }`;

  const loginMessages: Record<string, string> = {
    UNCHECKED:
      "Login broker belum diperiksa. Simpan kredensial untuk memulai pemeriksaan otomatis.",
    CHECKING: "Sedang memeriksa login broker dan kesiapan terminal MT5…",
    VALID: profile.login_message || "Login broker berhasil dan terminal siap.",
    ERROR:
      profile.login_message ||
      "Pemeriksaan login belum berhasil. Silakan periksa kembali data akun atau coba lagi nanti.",
  };
  const loginMessage =
    (profile.login_status && loginMessages[profile.login_status]) ||
    loginMessages.UNCHECKED;
  const loginClass = `account-login-check${
    profile.login_status === "VALID"
      ? " account-login-check--valid"
      : profile.login_status === "ERROR"
        ? " account-login-check--error"
        : " account-login-check--pending"
  }`;

  const stateLabels: Record<string, string> = {
    SETUP_REQUIRED: "Masukkan password broker dan atur batas copy terlebih dahulu.",
    PAUSED: "Copy trading dijeda. Tidak ada order baru yang dikirim.",
    STARTING: "Permintaan diterima. Menunggu worker copier mengambil job.",
    RUNNING:
      "Worker copier tersambung. Periksa terminal dan akun broker secara berkala.",
    OFFLINE:
      "Worker copier tidak mengirim heartbeat terbaru. Akun tidak sedang disinkronkan.",
    ERROR:
      profile.status_message ||
      "Worker copier melaporkan kendala. Periksa server dan akun broker.",
    STOPPED: "Worker copier berhenti. Periksa konfigurasi server.",
  };
  const connectionModifier =
    profile.status === "RUNNING"
      ? "ready"
      : profile.status === "PAUSED" || profile.status === "SETUP_REQUIRED"
        ? "disabled"
        : "offline";
  const connectionText =
    stateLabels[profile.status] || stateLabels.SETUP_REQUIRED;

  const balanceTimeText = account.state_updated_at
    ? `Data akun diperbarui ${wib(account.state_updated_at)} WIB`
    : "Belum menerima laporan saldo dari worker.";

  const handleSubmit = async (event: React.FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    onMessage("");
    const data = Object.fromEntries(
      new FormData(event.currentTarget).entries()
    ) as Record<string, FormDataEntryValue>;
    for (const key of [
      "ratio_lot",
      "lot_tetap",
      "max_lot_per_order",
      "max_lot_total",
      "max_open_positions",
    ]) {
      data[key] = Number(data[key]) as unknown as FormDataEntryValue;
    }
    try {
      await apiRequest(`mt5/accounts/${account.id}/managed-copy`, {
        method: "PUT",
        body: JSON.stringify(data),
      });
      onMessage(
        "Pengaturan tersimpan. Login broker sedang diperiksa otomatis; copy trading masih dijeda."
      );
      onReload();
    } catch (error) {
      onMessage(error instanceof Error ? error.message : "Permintaan tidak berhasil.");
    }
  };

  const handleToggleCopy = async () => {
    const active = !profile.active;
    if (
      active &&
      !confirm(
        "Konfirmasi: job ini akan mengirim order live ke akun broker Anda. Pastikan akun demo sudah diuji, login/server benar, serta batas lot dan posisi sesuai. Lanjutkan?"
      )
    )
      return;
    try {
      await apiRequest(`mt5/accounts/${account.id}/managed-copy/active`, {
        method: "PUT",
        body: JSON.stringify({ active, acknowledge_live_risk: active }),
      });
      onReload();
    } catch (error) {
      onMessage(error instanceof Error ? error.message : "Permintaan tidak berhasil.");
    }
  };

  const handleRemove = async () => {
    if (
      !confirm(
        `Hapus akun ${account.label} (${account.login})? Jeda copy trading terlebih dahulu.`
      )
    )
      return;
    try {
      await apiRequest(`mt5/accounts/${account.id}`, { method: "DELETE" });
      onReload();
    } catch (error) {
      onMessage(error instanceof Error ? error.message : "Permintaan tidak berhasil.");
    }
  };

  const toggleDisabled =
    !profile.configured ||
    (!profile.active &&
      (!managedWorkerReady || profile.login_status !== "VALID"));

  return (
    <article className="card">
      <h3>{account.label}</h3>
      <p>{detail}</p>
      <p className="muted">{stateText}</p>
      <p className={loginClass} role="status" aria-live="polite">
        {loginMessage}
      </p>
      <div
        className={`account-state account-state--${connectionModifier}`}
        role="status"
      >
        <span>{connectionText}</span>
      </div>
      <form
        className="form-grid"
        onSubmit={handleSubmit}
        onInput={onDirty}
        onChange={onDirty}
      >
        <label>
          Password akun broker
          <input
            name="broker_password"
            type="password"
            autoComplete="off"
            maxLength={128}
            required
            placeholder={
              profile.configured
                ? "Masukkan ulang untuk mengganti password tersimpan"
                : "Password trading akun follower"
            }
          />
        </label>
        <label>
          Nama simbol XAUUSD di broker
          <input
            name="follower_symbol"
            type="text"
            maxLength={64}
            pattern="[A-Za-z0-9._-]+"
            defaultValue={profile.follower_symbol || "XAUUSD"}
          />
        </label>
        <label>
          Metode ukuran lot
          <select name="mode_lot" defaultValue={profile.mode_lot || "rasio"}>
            <option value="rasio">Rasio terhadap lot sumber</option>
            <option value="tetap">Lot tetap</option>
          </select>
        </label>
        <label>
          Pengali lot rasio
          <input
            name="ratio_lot"
            type="number"
            min="0.01"
            max="5"
            step="0.01"
            required
            defaultValue={profile.ratio_lot ?? 1}
          />
        </label>
        <label>
          Lot tetap
          <input
            name="lot_tetap"
            type="number"
            min="0.01"
            max="5"
            step="0.01"
            required
            defaultValue={profile.lot_tetap ?? 0.01}
          />
        </label>
        <label>
          Batas lot per order
          <input
            name="max_lot_per_order"
            type="number"
            min="0.01"
            max="5"
            step="0.01"
            required
            defaultValue={profile.max_lot_per_order ?? 0.05}
          />
        </label>
        <label>
          Batas total lot terbuka
          <input
            name="max_lot_total"
            type="number"
            min="0.01"
            max="10"
            step="0.01"
            required
            defaultValue={profile.max_lot_total ?? 0.1}
          />
        </label>
        <label>
          Maksimum posisi terbuka
          <input
            name="max_open_positions"
            type="number"
            min="1"
            max="50"
            step="1"
            required
            defaultValue={profile.max_open_positions ?? 3}
          />
        </label>
        <div>
          <button type="submit">
            {profile.configured
              ? "Simpan pengaturan"
              : "Simpan kredensial dan pengaturan"}
          </button>
        </div>
      </form>
      <div className="toolbar">
        <button
          type="button"
          className={profile.active ? "secondary" : ""}
          disabled={toggleDisabled}
          onClick={handleToggleCopy}
        >
          {profile.active ? "Jeda copy trading" : "Mulai copy trading (live)"}
        </button>
        <button type="button" className="danger" onClick={handleRemove}>
          Hapus akun
        </button>
      </div>
      <p className="account-balance">
        {account.balance == null ? (
          "Saldo follower menunggu laporan worker."
        ) : (
          <Metrics
            items={[
              ["Saldo", account.balance, account.currency],
              ["Ekuitas", account.equity, account.currency],
              [
                "Floating",
                account.floating_profit,
                account.currency,
                numberTone(account.floating_profit),
              ],
            ]}
          />
        )}
      </p>
      <small className="muted account-balance-time">{balanceTimeText}</small>
    </article>
  );
}
export function AccountPanel() {
  const [plans, setPlans] = useState<Record<string, Plan>>({});
  const [accounts, setAccounts] = useState<Account[]>([]);
  const [performance, setPerformance] = useState<Performance | null>(null);
  const [copier, setCopier] = useState<CopierStatus | null>(null);
  const [executionMode, setExecutionMode] = useState("EA");
  const [pricing, setPricing] = useState<Pricing | null>(null);
  const [selectedVps, setSelectedVps] = useState<string | null>(null);
  const [accountMessage, setAccountMessage] = useState("");
  const [connectingAccount, setConnectingAccount] = useState(false);
  const [serverSearch, setServerSearch] = useState("");
  const [serverOptions, setServerOptions] = useState<BrokerServerOption[]>([]);
  const [selectedBrokerServer, setSelectedBrokerServer] = useState("");
  const [manualServerEntry, setManualServerEntry] = useState(false);
  const [manualServer, setManualServer] = useState("");
  const [serverSearchBusy, setServerSearchBusy] = useState(false);
  const [serverSearchMessage, setServerSearchMessage] = useState("");
  const [showAccountForm, setShowAccountForm] = useState(false);
  const [rootUrl, setRootUrl] = useState("");

  const selectedVpsPrice =
    pricing?.vps_options.find((v) => v.code === selectedVps)?.price_idr ?? 0;
  const totalIdr = (pricing?.copy_trading_fee_idr ?? 0) + selectedVpsPrice;
  const [tokenBox, setTokenBox] = useState<TokenBox>(null);
  const [tokenCopied, setTokenCopied] = useState(false);
  const managedFormDirty = useRef(false);
  const loginCheckPending = useRef(false);
  const modeTouched = useRef(false);

  const managedWorkerReady = copier?.available === true;
  const serviceStatus = !copier
    ? ""
    : copier.available
      ? "Worker managed copy online. Akun managed dapat didaftarkan dan dijalankan."
      : copier.configured
        ? "Worker managed copy offline. Layanan managed belum dapat dipesan atau dimulai."
        : "Layanan managed belum dikonfigurasi di server. Anda tetap dapat memilih Pasang EA sendiri.";

  const refreshAccountList = useCallback(async () => {
    const accountList = await apiRequest<Account[]>("mt5/accounts");
    setAccounts(accountList);
    loginCheckPending.current = accountList.some(
      (account) => account.managed_copy?.login_status === "CHECKING"
    );
  }, []);

  const loadAccounts = useCallback(async () => {
    try {
      const [accountList, perfData, copierStatus] = await Promise.all([
        apiRequest<Account[]>("mt5/accounts"),
        apiRequest<Performance>("mt5/performance"),
        apiRequest<CopierStatus>("mt5/copier/status"),
      ]);
      setAccounts(accountList);
      loginCheckPending.current = accountList.some(
        (account) => account.managed_copy?.login_status === "CHECKING"
      );
      setPerformance(perfData);
      setCopier(copierStatus);
      if (!modeTouched.current) {
        setExecutionMode(copierStatus.available === true ? "MANAGED" : "EA");
      }
    } catch (error) {
      if (error instanceof Error && /401|login diperlukan|unauthorized/i.test(error.message)) {
        setAccounts([]);
        setPerformance(null);
        setCopier(null);
        setTokenBox(null);
      }
      setAccountMessage(
        error instanceof Error ? error.message : "Permintaan tidak berhasil."
      );
    }
  }, []);

  useEffect(() => {
    setRootUrl(window.location.origin);
    apiRequest<Record<string, Plan>>("mt5/plans")
      .then(setPlans)
      .catch(() => {});
    apiRequest<Pricing>("mt5/pricing")
      .then(setPricing)
      .catch(() => {});
    loadAccounts();
    let timer: ReturnType<typeof setTimeout>;
    const pollAccounts = async () => {
      if (!document.hidden && !managedFormDirty.current) {
        if (loginCheckPending.current) {
          try {
            await refreshAccountList();
          } catch {
            // Keep the last known status and retry on the next short poll.
          }
        } else {
          await loadAccounts();
        }
      }
      const fastPoll = !document.hidden && !managedFormDirty.current && loginCheckPending.current;
      timer = setTimeout(pollAccounts, fastPoll ? 1000 : 5000);
    };
    timer = setTimeout(pollAccounts, 1000);
    return () => clearTimeout(timer);
  }, [loadAccounts, refreshAccountList]);

  useEffect(() => {
    const query = serverSearch.trim();
    if (query.length < 3) return;

    const controller = new AbortController();
    const timer = setTimeout(async () => {
      setServerSearchBusy(true);
      setServerSearchMessage("");
      try {
        const response = await fetch(
          `/api/mt5/servers?q=${encodeURIComponent(query)}`,
          { signal: controller.signal }
        );
        const payload = (await response.json()) as {
          servers?: BrokerServerOption[];
          error?: string;
        };
        if (!response.ok) throw new Error(payload.error || "Pencarian server gagal.");
        const options = Array.isArray(payload.servers) ? payload.servers : [];
        setServerOptions(options);
        setServerSearchMessage(
          options.length ? `${options.length} server ditemukan.` : "Server tidak ditemukan. Coba nama broker atau server yang lebih singkat."
        );
      } catch (error) {
        if (controller.signal.aborted) return;
        setServerOptions([]);
        setServerSearchMessage(
          error instanceof Error ? error.message : "Pencarian server gagal."
        );
      } finally {
        if (!controller.signal.aborted) setServerSearchBusy(false);
      }
    }, 350);

    return () => {
      clearTimeout(timer);
      controller.abort();
    };
  }, [serverSearch]);

  const handleConnectSubmit = async (
    event: React.FormEvent<HTMLFormElement>
  ) => {
    event.preventDefault();
    const form = event.currentTarget;
    if (!(manualServerEntry ? manualServer.trim() : selectedBrokerServer)) {
      setAccountMessage("Pilih server akun dari hasil pencarian atau masukkan nama server yang diberikan broker.");
      return;
    }
    setConnectingAccount(true);
    setAccountMessage("");
    const formData = new FormData(form);
    const mode = String(formData.get("execution_mode") || "EA");
    const payload = {
      execution_mode: mode,
      plan: mode === "MANAGED" ? "MANAGED" : selectedVps ? `EA_${selectedVps}` : "EA",
      label: formData.get("label"),
      server: formData.get("server"),
      login: formData.get("login"),
    };
    try {
      const order = await apiRequest<{ id: string | number }>(
        "payments/orders",
        { method: "POST", body: JSON.stringify(payload) }
      );
      form.reset();
      setServerSearch("");
      setServerOptions([]);
      setSelectedBrokerServer("");
      setManualServer("");
      setManualServerEntry(false);
      setExecutionMode("EA");
      setSelectedVps(null);
      modeTouched.current = false;
      window.location.href = `/transactions?order=${encodeURIComponent(
        String(order.id)
      )}`;
    } catch (error) {
      setAccountMessage(
        error instanceof Error ? error.message : "Permintaan tidak berhasil."
      );
    } finally {
      setConnectingAccount(false);
    }
  };

  const handleToken = (title: string, token: string, accountId: number) => {
    setTokenCopied(false);
    setTokenBox({ title, token, accountId });
  };

  const clearTokenForAccount = (accountId: number) => {
    setTokenBox((prev) =>
      prev && prev.accountId === accountId ? null : prev
    );
  };

  const handleCopyToken = async () => {
    if (!tokenBox) return;
    try {
      await navigator.clipboard.writeText(tokenBox.token);
      setTokenCopied(true);
    } catch {
      setTokenCopied(false);
    }
  };

  const handleLoginInput = (
    event: React.FormEvent<HTMLInputElement>
  ) => {
    const input = event.currentTarget;
    input.setCustomValidity(
      input.validity.patternMismatch
        ? "Nomor akun MT5 hanya boleh berisi angka, tanpa spasi."
        : ""
    );
  };

  const handleLoginInvalid = (
    event: React.FormEvent<HTMLInputElement>
  ) => {
    event.currentTarget.setCustomValidity(
      "Nomor akun MT5 hanya boleh berisi angka, tanpa spasi."
    );
  };

  const balanceGroups = performance?.balance_groups ?? [];
  const performanceGroups = performance?.performance_groups ?? [];
  const hasEaAccount = accounts.some(
    (account) => account.execution_mode !== "MANAGED"
  );
  const hasExistingPackage = accounts.length > 0;

  return (
    <main className="page-shell dashboard-page account-page" id="main-content" tabIndex={-1}>
      <div className="page-heading">
        <div>
          <span className="eyebrow">PORTAL KLIEN / COPY TRADING</span>
          <h1>Akun Copy Trading</h1>
          <p className="page-description">
            Daftarkan akun follower dan atur layanan salin posisi dari portal ini.
          </p>
        </div>
        <p className="heading-aside">
          Password broker tidak ditampilkan kembali setelah disimpan.
        </p>
      </div>

      <section>
        <h2>Hubungkan akun Copy Trading</h2>
        {hasExistingPackage && !showAccountForm ? (
          <>
            <p className="section-help">Paket Copy Trading Anda sudah terdaftar. Gunakan aksi di bawah untuk melihat transaksi atau mendaftarkan akun tambahan.</p>
            <div className="account-primary-actions">
              <a className="account-action-button account-action-button--secondary" href="/transactions">Riwayat transaksi dan nota</a>
              <button className="account-action-button" type="button" onClick={() => setShowAccountForm(true)}>Hubungkan akun lain</button>
            </div>
          </>
        ) : (
        <>
        <p className="section-help">
          Pilih cara menjalankan akun, layanan, dan detail akun broker. Akun dibuat
          setelah pembayaran disetujui. Untuk layanan dikelola server, password
          broker dienkripsi dan hanya dikirim ke worker copier yang terautentikasi.
        </p>
        <form id="account-form" onSubmit={handleConnectSubmit}>
          <fieldset className="plan-picker plan-picker--compact">
            <legend>Cara menjalankan copy trading</legend>
            <div
              className="price-list"
              role="radiogroup"
              aria-label="Cara menjalankan copy trading"
            >
              <label
                className="price-row"
                htmlFor="execution-managed"
                {...(!managedWorkerReady
                  ? {
                      "aria-disabled": "true",
                      title: "Layanan dikelola server sedang offline",
                    }
                  : {})}
              >
                <input
                  id="execution-managed"
                  type="radio"
                  name="execution_mode"
                  value="MANAGED"
                  required
                  disabled={!managedWorkerReady}
                  checked={executionMode === "MANAGED"}
                  onChange={() => {
                    modeTouched.current = true;
                    setExecutionMode("MANAGED");
                  }}
                />
                <span className="price-row__body">
                  <span className="price-row__name">
                    Dikelola server
                    {!managedWorkerReady && (
                      <span className="plan-option__availability">
                        Belum tersedia
                      </span>
                    )}
                  </span>
                  <span className="price-row__desc">
                    Pengaturan melalui website. MT5 berjalan di server
                    layanan, bukan di komputer Anda.
                  </span>
                </span>
              </label>
              <label className="price-row" htmlFor="execution-ea">
                <input
                  id="execution-ea"
                  type="radio"
                  name="execution_mode"
                  value="EA"
                  required
                  checked={executionMode === "EA"}
                  onChange={() => {
                    modeTouched.current = true;
                    setExecutionMode("EA");
                  }}
                />
                <span className="price-row__body">
                  <span className="price-row__name">Pasang EA sendiri</span>
                  <span className="price-row__desc">
                    Anda menjalankan terminal MT5 dan EA Copy Trading di
                    perangkat/VPS sendiri.
                  </span>
                </span>
              </label>
            </div>
          </fieldset>
          <p
            id="managed-copy-service-status"
            className="status"
            role="status"
            aria-live="polite"
          >
            {serviceStatus}
          </p>
          <fieldset className="plan-picker plan-picker--compact">
            <legend>Biaya layanan</legend>
            {pricing ? (
              executionMode === "MANAGED" ? (
                <div className="price-list">
                  <div className="price-row">
                    <span className="price-row__body">
                      <span className="price-row__name">
                        Biaya pengelolaan akun
                      </span>
                      <span className="price-row__desc">
                        MT5 berjalan di server layanan.
                      </span>
                    </span>
                    <span className="price-row__price">
                      Rp {formatInteger(pricing.management_fee_idr)}
                    </span>
                  </div>
                </div>
              ) : (
                <div className="account-price-stack">
                  <div className="price-list">
                    <div className="price-row">
                      <span className="price-row__body">
                        <span className="price-row__name">
                          Biaya copy trading
                        </span>
                        <span className="price-row__desc">
                          Setengah dari biaya pengelolaan server.
                        </span>
                      </span>
                      <span className="price-row__price">
                        Rp {formatInteger(pricing.copy_trading_fee_idr)}
                      </span>
                    </div>
                  </div>
                  {pricing.vps_options.length > 0 && (
                    <>
                      <p className="mini-title">
                        Tambah VPS{" "}
                        <span className="mini-title__opt">(opsional)</span>
                      </p>
                      <div
                        className="price-list"
                        role="radiogroup"
                        aria-label="Pilihan VPS"
                      >
                        <label className="price-row">
                          <input
                            type="radio"
                            name="vps_option"
                            checked={selectedVps === null}
                            onChange={() => setSelectedVps(null)}
                          />
                          <span className="price-row__body">
                            <span className="price-row__name">Tanpa VPS</span>
                            <span className="price-row__desc">
                              Jalankan di perangkat atau VPS Anda sendiri.
                            </span>
                          </span>
                          <span className="price-row__price">Rp 0</span>
                        </label>
                        {pricing.vps_options.map((vps) => (
                          <label key={vps.code} className="price-row">
                            <input
                              type="radio"
                              name="vps_option"
                              checked={selectedVps === vps.code}
                              onChange={() => setSelectedVps(vps.code)}
                            />
                            <span className="price-row__body">
                              <span className="price-row__name">
                                {vps.name}
                              </span>
                              <span className="price-row__desc">
                                {vps.vcpu} · RAM {vps.ram} · Disk {vps.disk}
                              </span>
                            </span>
                            <span className="price-row__price">
                              Rp {formatInteger(vps.price_idr)}
                            </span>
                          </label>
                        ))}
                      </div>
                    </>
                  )}
                  <div className="price-list price-list--summary">
                    <div className="price-list__total">
                      <span>Total bayar</span>
                      <strong>Rp {formatInteger(totalIdr)}</strong>
                    </div>
                  </div>
                </div>
              )
            ) : (
              <p className="status">Memuat harga layanan…</p>
            )}
            <p className="field-help">
              Harga mengikuti pengaturan layanan. Pembayaran sementara
              dilakukan manual dan diperiksa admin. Jangan transfer sebelum
              melihat instruksi pada pesanan Anda.
            </p>
          </fieldset>
          <div className="account-fields">
            <label>
              Nama akun
              <input
                name="label"
                maxLength={80}
                placeholder="Copy Trading 1"
                required
              />
            </label>
            <div className="broker-server-field">
              <label htmlFor="broker-server-query">Cari broker atau server MT5</label>
              <input
                id="broker-server-query"
                type="search"
                value={serverSearch}
                maxLength={96}
                autoComplete="off"
                placeholder="Contoh: HFM atau HFMarketsGlobal-Live11"
                aria-describedby="broker-server-help"
                onChange={(event) => {
                  const value = event.currentTarget.value;
                  setServerSearch(value);
                  setServerOptions([]);
                  setSelectedBrokerServer("");
                  setServerSearchBusy(value.trim().length >= 3);
                  setServerSearchMessage(
                    value.trim().length >= 3
                      ? "Menyiapkan pencarian…"
                      : "Ketik minimal 3 karakter untuk mencari server MT5."
                  );
                }}
              />
              {manualServerEntry ? (
                <>
                  <label htmlFor="manual-broker-server">Nama server akun</label>
                  <input
                    id="manual-broker-server"
                  name="server"
                  value={manualServer}
                  maxLength={128}
                  required
                  placeholder="Nama server persis dari broker"
                  onChange={(event) => setManualServer(event.currentTarget.value)}
                  />
                </>
              ) : (
                <>
                  <label htmlFor="broker-server-selection">Server akun</label>
                  <select
                    id="broker-server-selection"
                    name="server"
                    value={selectedBrokerServer}
                    required
                    onChange={(event) => setSelectedBrokerServer(event.currentTarget.value)}
                    aria-describedby="broker-server-help"
                  >
                    <option value="">Pilih server akun dari hasil pencarian</option>
                    {serverOptions.map((option) => (
                      <option key={`${option.company}:${option.name}`} value={option.name}>
                        {option.name} — {option.company}
                      </option>
                    ))}
                  </select>
                </>
              )}
              <span id="broker-server-help" className="field-help" role="status">
                {serverSearchBusy ? "Mencari direktori server MT5…" : serverSearchMessage || "Pilih server yang tercantum pada detail akun broker Anda."}
              </span>
              <span className="field-help">
                Pencarian hanya mengirim teks broker/server; nomor akun dan password tidak dikirim.
              </span>
              <button
                type="button"
                className="server-manual-toggle"
                onClick={() => {
                  setManualServerEntry((value) => !value);
                  setManualServer("");
                  setSelectedBrokerServer("");
                }}
              >
                {manualServerEntry ? "Pilih dari hasil pencarian" : "Nama server tidak ada? Masukkan manual"}
              </button>
            </div>
            <label>
              Nomor akun broker
              <input
                name="login"
                maxLength={32}
                inputMode="numeric"
                pattern="[0-9]+"
                title="Nomor akun MT5 hanya boleh berisi angka"
                aria-describedby="account-login-help"
                required
                onInput={handleLoginInput}
                onInvalid={handleLoginInvalid}
              />
              <span id="account-login-help" className="field-help">
                Masukkan nomor login MT5 berupa angka saja, tanpa spasi.
              </span>
            </label>
          </div>
          <div className="form-actions">
            <button type="submit" disabled={connectingAccount} aria-busy={connectingAccount}>{connectingAccount ? "Menyiapkan pembayaran…" : "Lanjutkan ke pembayaran"}</button>
          </div>
        </form>
        <p id="account-message" className="status" role="status">
          {accountMessage}
        </p>
        <div id="new-token" hidden={!tokenBox}>
          {tokenBox && (
            <>
              <h3>{tokenBox.title}</h3>
              <p>
                Salin token ini sekarang, lalu tempel ke input AccountToken pada EA
                MT5. Token tidak ditampilkan lagi setelah panel ditutup.
              </p>
              <pre>{tokenBox.token}</pre>
              <button type="button" onClick={handleCopyToken}>
                {tokenCopied ? "Tersalin" : "Salin token"}
              </button>
            </>
          )}
        </div>
        {hasExistingPackage && showAccountForm && (
          <div className="account-form-actions">
            <button type="button" className="account-action-button account-action-button--secondary" onClick={() => setShowAccountForm(false)}>Tutup formulir</button>
          </div>
        )}
        </>
        )}
      </section>

      <section>
        <h2>Saldo dan performa akun</h2>
        <p className="section-help">
          Worker managed melaporkan saldo, ekuitas, posisi terbuka, dan transaksi
          follower ke portal. Nilai dengan mata uang berbeda ditampilkan terpisah.
        </p>
        <div id="client-balance-groups" className="grid">
          {balanceGroups.length === 0 ? (
            <p className="empty-state">
              Saldo akan tampil setelah EA Copy Trading mengirim laporan dari MT5.
            </p>
          ) : (
            balanceGroups.map((group) => (
              <article key={`${group.trade_mode}-${group.currency}`} className="card">
                <h3>
                  Saldo {modeLabel(group.trade_mode)} · {group.currency}
                </h3>
                <p className="account-balance">
                  Saldo {formatMoney(group.balance, group.currency)}
                </p>
                <p className="muted">
                  <Metrics
                    items={[
                      ["Ekuitas", group.equity, group.currency],
                      [
                        "Floating",
                        group.floating_profit,
                        group.currency,
                        numberTone(group.floating_profit),
                      ],
                    ]}
                  />
                </p>
                <small className="muted">
                  {`${formatInteger(group.online_accounts)} dari ${formatInteger(
                    group.reported_accounts
                  )} akun melapor · ${
                    group.updated_at
                      ? `${wib(group.updated_at)} WIB`
                      : "belum ada pembaruan"
                  }`}
                </small>
              </article>
            ))
          )}
        </div>
        <div id="client-performance-groups" className="grid client-performance-groups">
          {performanceGroups.length === 0 ? (
            <p className="empty-state">
              Belum ada transaksi tertutup yang dilaporkan oleh MT5.
            </p>
          ) : (
            performanceGroups.map((group) => (
              <article key={`${group.trade_mode}-${group.currency}`} className="card">
                <h3>
                  Performa {modeLabel(group.trade_mode)} · {group.currency}
                </h3>
                <p className="account-balance">
                  <Metrics
                    items={[
                      [
                        "Hasil terealisasi",
                        group.realized_pnl,
                        group.currency,
                        numberTone(group.realized_pnl),
                      ],
                    ]}
                  />
                </p>
                <p className="muted">
                  {`${formatInteger(group.closed_trades)} transaksi ditutup · ${formatInteger(
                    group.wins
                  )} menang · ${formatInteger(group.losses)} kalah · tingkat kemenangan ${formatInteger(
                    group.win_rate
                  )}%`}
                </p>
              </article>
            ))
          )}
        </div>
        <p id="client-performance-updated" className="muted" aria-live="polite">
          {performance?.updated_at
            ? `Pembaruan MT5 terakhir ${wib(performance.updated_at)} WIB.`
            : "Belum menerima data saldo MT5."}
        </p>
      </section>

      <section>
        <h2>Akun Copy Trading Anda</h2>
        <p className="section-help">
          Kelola hingga 10 akun. Pilih <strong>Mulai copy trading</strong> hanya
          setelah mengecek login, ukuran lot, dan batas risiko. Job server akan
          menjalankan order live pada akun broker Anda.
        </p>
        <div id="accounts" className="grid">
          {accounts.length === 0 ? (
            <p className="empty-state">Belum ada akun MT5.</p>
          ) : (
            accounts.map((account) => {
              const plan = plans[account.plan] || ZERO_PLAN;
              return account.execution_mode === "MANAGED" ? (
                <ManagedAccountCard
                  key={account.id}
                  account={account}
                  plan={plan}
                  managedWorkerReady={managedWorkerReady}
                  onDirty={() => {
                    managedFormDirty.current = true;
                  }}
                  onMessage={setAccountMessage}
                  onReload={() => {
                    managedFormDirty.current = false;
                    void loadAccounts();
                  }}
                />
              ) : (
                <EaAccountCard
                  key={account.id}
                  account={account}
                  plan={plan}
                  onMessage={setAccountMessage}
                  onReload={loadAccounts}
                  onToken={handleToken}
                  clearTokenForAccount={clearTokenForAccount}
                />
              );
            })
          )}
        </div>
      </section>

      <section id="ea-install-section" hidden={!hasEaAccount}>
        <h2>Pasang EA Copy Trading</h2>
        <p className="section-help">
          Petunjuk ini berlaku untuk akun yang memilih menjalankan EA sendiri.
          Akun yang dikelola server tidak perlu mengikuti langkah MT5 ini di
          komputer client.
        </p>
        <div className="toolbar">
          <a
            className="workspace-tag ea-download-button"
            href="/api/core/downloads/MT5FollowerCopyEA.mq5"
          >
            Unduh source EA Copy Trading
          </a>
        </div>
        <ol>
          <li>
            Hubungkan akun Copy Trading di atas. Salin token yang muncul; token
            hanya terlihat saat dibuat atau dirotasi.
          </li>
          <li>
            Di MT5, pilih File → Open Data Folder, buka folder{" "}
            <code>MQL5/Experts</code>, lalu salin file <code>.mq5</code> yang
            diunduh ke sana. Buka file itu di MetaEditor dan tekan F7; hasil compile <code>.ex5</code> siap digunakan
            di folder tersebut.
          </li>
          <li>
            Di MT5, pilih Tools → Options → Expert Advisors, aktifkan Allow WebRequest for listed URL, lalu tambahkan{" "}
            <code>{rootUrl || "root URL situs ini"}</code>.
          </li>
          <li>
            Dari Navigator → Expert Advisors, pasang EA Copy Trading di chart. Isi <code>AccountToken</code> dengan token akun tersebut, pastikan <code>ServerURL</code> benar, lalu aktifkan Algo Trading.
          </li>
          <li>
            Pastikan EA utama Algentra aktif. EA Copy Trading akan menyalin posisi
            live saat izin trading MT5 aktif; atur{" "}
            <code>VolumeMultiplier</code> sesuai kebutuhan sebelum menjalankannya.
          </li>
        </ol>
        <div
          className="notice-box"
          role="note"
          aria-label="Bantuan pesan status EA Copy Trading"
        >
          <h3>Arti pesan di chart MT5</h3>
          <p>
            <strong>HTTP -1 / ERROR 4006:</strong> EA belum mendapat jawaban dari
            server. Kode 4006 berarti MT5 menemukan data array yang tidak valid
            saat EA berjalan; pesan ini sendiri belum membuktikan token salah.
            Pastikan URL <code>ServerURL</code> sama dengan alamat yang diizinkan
            di <em>Allow WebRequest</em>. Jika masih muncul, kirim tangkapan tab{" "}
            <strong>Experts</strong> kepada dukungan. Jangan kirim token.
          </p>
          <p>
            <strong>TRADING OFF:</strong> Periksa <strong>Algo Trading</strong>{" "}
            serta izin trading terminal, akun, dan Expert Advisor di MT5.
          </p>
        </div>
        <p className="notice-box">
          Akun Copy Trading harus memakai mode <strong>hedging</strong>. Volume
          salinan mengikuti volume sumber dikalikan{" "}
          <code>VolumeMultiplier</code>; nilainya tidak menyesuaikan ekuitas
          secara otomatis. Jika pembaruan dari akun utama terhenti lebih dari 20
          detik, sinkronisasi berhenti. Posisi yang sudah terbuka tetap berada di
          MT5 jika EA berhenti.
        </p>
        <p className="section-help">
          Jangan bagikan token EA. Jika token hilang atau terekspos, pilih{" "}
          <strong>Rotasi token</strong> dan perbarui token di EA Copy Trading.
        </p>
      </section>

      <CsChatWidget mode="client" className="cs-chat-widget--account" />
    </main>
  );
}
