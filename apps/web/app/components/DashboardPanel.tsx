"use client";

import { FormEvent, useEffect, useState } from "react";
import { apiRequest } from "../lib/api";
import { formatInteger, formatMoney, formatNumber, numberTone, toneClass } from "../lib/format";

type Stats = Record<string, number | string | boolean | null | undefined>;
type Mt5Live = {
  online: boolean; available: boolean; observed_at?: string | null;
  account?: {
    balance?: number | null; equity?: number | null; floating_profit?: number | null;
    currency?: string | null; trade_mode?: string | null;
  } | null;
  positions: Array<{ ticket: string; symbol: string; action: string; lots: number; entry_price: number; sl: number; tp: number }>;
  market_quotes: Array<{ symbol: string; bid: number; ask: number; time_msc: number }>;
};
type Signal = {
  created_at: string; group_name?: string | null; group_id?: string | null; raw_text: string;
  normalized_text?: string | null; confidence?: number | null; status: string; ticket?: string | null; profit?: number | null;
};
type GroupStat = { group_name: string; signals_total: number; wins: number; losses: number; winrate: number; profit: number };
type LogEntry = { created_at: string; level: string; source: string; message: string };
type Trade = {
  opened_at: string; closed_at?: string | null; group_name?: string | null; ticket: string;
  symbol: string; action: string; lots: number; entry: number; profit?: number | null; result?: string | null;
};

const wibTime = (value?: string | null) =>
  value ? `${new Date(value).toLocaleString("id-ID", { timeZone: "Asia/Jakarta" })} WIB` : "—";
const TRADE_PAGE_SIZE = 10;

function query(base: string, values: Record<string, string>): string {
  const params = new URLSearchParams();
  for (const [key, value] of Object.entries(values)) if (value) params.set(key, value);
  const qs = params.toString();
  return qs ? `${base}?${qs}` : base;
}

function StatCard({ label, value, tone }: { label: string; value: string; tone?: string }) {
  return (
    <article className="card">
      <span className="eyebrow">{label}</span>
      <h2 className={tone ? toneClass(tone as "pos" | "neg" | "warn" | "info" | "muted") : undefined}>{value}</h2>
    </article>
  );
}

export function DashboardPanel() {
  const [stats, setStats] = useState<Stats | null>(null);
  const [live, setLive] = useState<Mt5Live | null>(null);
  const [signals, setSignals] = useState<Signal[]>([]);
  const [groups, setGroups] = useState<GroupStat[]>([]);
  const [logs, setLogs] = useState<LogEntry[]>([]);
  const [trades, setTrades] = useState<Trade[]>([]);
  const [tradePage, setTradePage] = useState(0);
  const [hasMoreTrades, setHasMoreTrades] = useState(false);
  const [settings, setSettings] = useState<Record<string, unknown>>({});
  const [error, setError] = useState("");
  const [settingsMessage, setSettingsMessage] = useState("");
  const [geminiMessage, setGeminiMessage] = useState("");
  const [registrationMessage, setRegistrationMessage] = useState("");
  const [logLevel, setLogLevel] = useState("");
  const [logSearch, setLogSearch] = useState("");
  const [tradeFilters, setTradeFilters] = useState({ date_from: "", date_to: "", group_id: "", symbol: "", result: "" });
  const [symbolMapping, setSymbolMapping] = useState("");
  const [busySettings, setBusySettings] = useState(false);

  async function loadStatsAndSignals() {
    const [freshStats, freshSignals] = await Promise.all([
      apiRequest<Stats>("dashboard/stats"),
      apiRequest<Signal[]>("dashboard/signals?limit=5"),
    ]);
    setStats(freshStats);
    setSignals(freshSignals);
  }

  async function loadLive() {
    setLive(await apiRequest<Mt5Live>("dashboard/mt5-live"));
  }

  async function loadGroups() {
    const rows = await apiRequest<GroupStat[]>("dashboard/groups/stats");
    setGroups([...rows].sort((a, b) => b.signals_total - a.signals_total).slice(0, 8));
  }

  async function loadLogs() {
    setLogs(await apiRequest<LogEntry[]>(query("dashboard/logs", { level: logLevel, search: logSearch, limit: "5" })));
  }

  async function loadTrades(page: number) {
    const safePage = Math.max(0, page);
    const values: Record<string, string> = {
      date_from: tradeFilters.date_from ? `${tradeFilters.date_from}T00:00:00` : "",
      date_to: tradeFilters.date_to ? `${tradeFilters.date_to}T23:59:59` : "",
      group_id: tradeFilters.group_id,
      symbol: tradeFilters.symbol,
      result: tradeFilters.result,
      limit: String(TRADE_PAGE_SIZE + 1),
      offset: String(safePage * TRADE_PAGE_SIZE),
    };
    const rows = await apiRequest<Trade[]>(query("dashboard/trades", values));
    setHasMoreTrades(rows.length > TRADE_PAGE_SIZE);
    setTrades(rows.slice(0, TRADE_PAGE_SIZE));
    setTradePage(safePage);
  }

  async function loadSettings() {
    const data = await apiRequest<Record<string, unknown>>("settings");
    setSettings(data);
    const mapping = data.symbol_mapping as Record<string, string> | undefined;
    setSymbolMapping(JSON.stringify(mapping ?? { GOLD: "XAUUSD", XAU: "XAUUSD", EMAS: "XAUUSD" }, null, 2));
  }

  async function refresh() {
    try {
      await Promise.all([loadStatsAndSignals(), loadLive(), loadGroups(), loadLogs(), loadTrades(tradePage)]);
    } catch (reason) {
      setError(reason instanceof Error ? reason.message : "Gagal memuat dashboard.");
    }
  }

  useEffect(() => {
    void refresh();
    void loadSettings().catch((reason) => setSettingsMessage(reason instanceof Error ? reason.message : "Gagal memuat settings."));
    const timer = setInterval(() => {
      if (document.hidden) return;
      void loadStatsAndSignals().catch(() => undefined);
      void loadLive().catch(() => undefined);
    }, 5000);
    return () => clearInterval(timer);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  async function setRegistrationOpen(isOpen: boolean) {
    setRegistrationMessage("Menyimpan status pendaftaran…");
    try {
      await apiRequest("settings", { method: "PUT", body: JSON.stringify({ values: { registration_open: isOpen } }) });
      setRegistrationMessage(isOpen ? "Pendaftaran dibuka." : "Pendaftaran ditutup.");
      await loadSettings();
    } catch (reason) {
      setRegistrationMessage(`Gagal: ${reason instanceof Error ? reason.message : "unknown"}`);
    }
  }

  async function submitSettings(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    setBusySettings(true);
    setSettingsMessage("");
    try {
      const form = new FormData(event.currentTarget);
      const values = {
        confidence_threshold: Number(form.get("confidence_threshold")),
        symbol_mapping: JSON.parse(String(form.get("symbol_mapping") ?? "{}")),
        max_market_deviation_pct: Number(form.get("max_market_deviation_pct")),
        entry_mode: String(form.get("entry_mode") ?? "PARTIAL"),
        allow_updates: form.get("allow_updates") === "on",
        kill_switch: form.get("kill_switch") === "on",
      };
      await apiRequest("settings", { method: "PUT", body: JSON.stringify({ values }) });
      setSettingsMessage("Settings tersimpan.");
      await loadSettings();
      await refresh();
    } catch (reason) {
      setSettingsMessage(`Gagal: ${reason instanceof Error ? reason.message : "unknown"}`);
    } finally {
      setBusySettings(false);
    }
  }

  async function submitGemini(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    setGeminiMessage("");
    try {
      const form = new FormData(event.currentTarget);
      const result = await apiRequest<{ gemini_model?: string; gemini_configured?: boolean }>("settings/gemini-config", {
        method: "PUT",
        body: JSON.stringify({ model: String(form.get("gemini_model") ?? "").trim() }),
      });
      setGeminiMessage(`Tersimpan. Model: ${result.gemini_model ?? "—"}; API key ${result.gemini_configured ? "terpasang" : "belum ada"}.`);
      await loadStatsAndSignals();
    } catch (reason) {
      setGeminiMessage(`Gagal: ${reason instanceof Error ? reason.message : "unknown"}`);
    }
  }

  const num = (value: unknown) => (typeof value === "number" ? value : Number(value ?? 0));
  const registrationOpen = settings.registration_open !== false;
  const liveStatus = !live ? "Menghubungkan" : live.online ? "ONLINE" : live.available ? "DATA BASI" : "MENUNGGU TERMINAL";

  return (
    <>
      <section className="panel section">
        <span className="eyebrow">Algentra Capital · Operations</span>
        <h2>Operations workspace</h2>
        <p className="muted">Pantau sinyal, hasil eksekusi, performa trade, dan batas risiko dalam satu tampilan.</p>
        {error && <p className="error" role="alert">{error}</p>}
      </section>

      <section className="panel section">
        <span className="eyebrow">Public site</span>
        <h2>Pendaftaran client</h2>
        <p className="muted" role="status">{registrationOpen ? "Pendaftaran terbuka. Form daftar terlihat di situs publik." : "Pendaftaran ditutup. Situs publik menampilkan hitung mundur launching."}</p>
        <div className="actions">
          <button className="button primary" disabled={registrationOpen} onClick={() => void setRegistrationOpen(true)}>Buka pendaftaran</button>
          <button className="button" disabled={!registrationOpen} onClick={() => void setRegistrationOpen(false)}>Tutup pendaftaran</button>
        </div>
        {registrationMessage && <p className="muted" role="status">{registrationMessage}</p>}
      </section>

      <section className="panel section">
        <span className="eyebrow">Ringkasan operasional</span>
        <div className="cards">
          <StatCard label="EA" value={stats ? (stats.ea_online ? "EA Online" : "EA Offline") : "—"} tone={stats ? (stats.ea_online ? "pos" : "neg") : undefined} />
          <StatCard label="Telegram" value={stats ? (stats.telegram_connected ? "Terhubung" : "Offline") : "—"} tone={stats ? (stats.telegram_connected ? "pos" : "neg") : undefined} />
          <StatCard label="Gemini" value={stats ? (stats.gemini_configured ? "Siap" : "Belum siap") : "—"} tone={stats ? (stats.gemini_configured ? "pos" : "warn") : undefined} />
          <StatCard label="Total profit" value={stats ? formatNumber(num(stats.total_profit), 2, 2) : "—"} tone={stats ? numberTone(num(stats.total_profit)) : undefined} />
          <StatCard label="Winrate" value={stats && stats.winrate !== null && stats.winrate !== undefined ? `${formatNumber(num(stats.winrate), 0, 1)}%` : "—"} />
          <StatCard label="Win / Loss" value={stats ? `${formatInteger(num(stats.wins))} / ${formatInteger(num(stats.losses))}` : "—"} />
          <StatCard label="Rugi hari ini" value={stats ? formatNumber(Math.abs(num(stats.daily_loss)), 2, 2) : "—"} tone={stats && num(stats.daily_loss) !== 0 ? "neg" : undefined} />
          <StatCard label="Kill switch" value={stats ? (stats.kill_switch ? "AKTIF" : "Mati") : "—"} tone={stats ? (stats.kill_switch ? "neg" : "pos") : undefined} />
        </div>
        {stats && (
          <p className="muted">
            Sinyal: {formatInteger(num(stats.signals_total))} total · {formatInteger(num(stats.signals_executed))} tereksekusi ·{" "}
            {formatInteger(num(stats.signals_pending))} pending · {formatInteger(num(stats.signals_processing))} diproses ·{" "}
            {formatInteger(num(stats.signals_rejected))} ditolak · {formatInteger(num(stats.signals_ignored))} diabaikan
          </p>
        )}
      </section>

      <section className="panel section">
        <span className="eyebrow">MT5 live</span>
        <h2>Data MT5 live <span className={toneClass(live?.online ? "pos" : "warn")}>· {liveStatus}</span></h2>
        <p className="muted">{live?.observed_at ? `Snapshot terakhir ${wibTime(live.observed_at)}` : "Menunggu snapshot terminal…"}</p>
        {live?.account && (
          <div className="cards">
            <StatCard label="Equity" value={formatMoney(live.account.equity, live.account.currency)} />
            <StatCard label="Balance" value={formatMoney(live.account.balance, live.account.currency)} />
            <StatCard label="Floating P/L" value={formatMoney(live.account.floating_profit, live.account.currency)} tone={numberTone(live.account.floating_profit)} />
            <StatCard label="Posisi terbuka" value={formatInteger(live.positions.length)} />
          </div>
        )}
        <h3>Posisi terminal</h3>
        <div className="table-wrap"><table>
          <thead><tr><th>Ticket</th><th>Simbol</th><th>Arah</th><th>Lot</th><th>Entry</th><th>SL</th><th>TP</th></tr></thead>
          <tbody>
            {(live?.positions ?? []).map((position) => (
              <tr key={position.ticket}>
                <td>{position.ticket}</td><td>{position.symbol}</td><td>{position.action}</td>
                <td>{formatNumber(position.lots, 2, 3)}</td><td>{formatNumber(position.entry_price, 2, 5)}</td>
                <td>{Number(position.sl) > 0 ? formatNumber(position.sl, 2, 5) : "—"}</td>
                <td>{Number(position.tp) > 0 ? formatNumber(position.tp, 2, 5) : "—"}</td>
              </tr>
            ))}
            {(live?.positions ?? []).length === 0 && <tr><td colSpan={7} className="muted">Tidak ada posisi terbuka.</td></tr>}
          </tbody>
        </table></div>
        <h3>Harga pasar</h3>
        <div className="table-wrap"><table>
          <thead><tr><th>Simbol</th><th>Bid</th><th>Ask</th><th>Update</th></tr></thead>
          <tbody>
            {(live?.market_quotes ?? []).map((quote) => (
              <tr key={quote.symbol}>
                <td>{quote.symbol}</td><td>{formatNumber(quote.bid, 2, 5)}</td><td>{formatNumber(quote.ask, 2, 5)}</td>
                <td>{wibTime(new Date(Number(quote.time_msc)).toISOString())}</td>
              </tr>
            ))}
          </tbody>
        </table></div>
      </section>

      <section className="panel section">
        <span className="eyebrow">Sinyal</span>
        <h2>Signal terbaru</h2>
        <p className="muted">Lima pesan terbaru beserta hasil parsing, confidence, status pemrosesan, dan ticket order.</p>
        <div className="table-wrap"><table>
          <thead><tr><th>Waktu</th><th>Grup</th><th>Pesan</th><th>Parsing</th><th>Confidence</th><th>Status</th><th>Ticket</th><th>Profit</th></tr></thead>
          <tbody>
            {signals.map((signal, index) => (
              <tr key={`${signal.created_at}-${index}`}>
                <td>{wibTime(signal.created_at)}</td>
                <td>{signal.group_name || signal.group_id || "—"}</td>
                <td>{signal.raw_text.length > 60 ? `${signal.raw_text.slice(0, 60)}…` : signal.raw_text}</td>
                <td>{signal.normalized_text || "—"}</td>
                <td>{signal.confidence === null || signal.confidence === undefined ? "—" : formatNumber(signal.confidence, 2, 2)}</td>
                <td>{signal.status}</td>
                <td>{signal.ticket || "—"}</td>
                <td className={toneClass(numberTone(signal.profit))}>{signal.profit === null || signal.profit === undefined ? "—" : formatNumber(signal.profit, 2, 2)}</td>
              </tr>
            ))}
            {signals.length === 0 && <tr><td colSpan={8} className="muted">Belum ada sinyal.</td></tr>}
          </tbody>
        </table></div>
      </section>

      <section className="panel section">
        <span className="eyebrow">Grup</span>
        <h2>Win dan loss per grup</h2>
        <p className="muted">Bandingkan akurasi dan profit dari setiap grup sumber.</p>
        <div className="table-wrap"><table>
          <thead><tr><th>Grup</th><th>Signal</th><th>Win</th><th>Loss</th><th>Winrate</th><th>Profit</th></tr></thead>
          <tbody>
            {groups.map((group) => (
              <tr key={group.group_name}>
                <td>{group.group_name}</td><td>{formatInteger(group.signals_total)}</td>
                <td>{formatInteger(group.wins)}</td><td>{formatInteger(group.losses)}</td>
                <td>{formatNumber(Number(group.winrate), 0, 1)}%</td>
                <td className={toneClass(numberTone(group.profit))}>{formatNumber(group.profit, 2, 2)}</td>
              </tr>
            ))}
            {groups.length === 0 && <tr><td colSpan={6} className="muted">Belum ada data grup.</td></tr>}
          </tbody>
        </table></div>
      </section>

      <section className="panel section">
        <span className="eyebrow">Log</span>
        <h2>Log sistem</h2>
        <div className="actions">
          <label className="field">Level
            <select value={logLevel} onChange={(event) => setLogLevel(event.target.value)}>
              <option value="">Semua level</option><option value="INFO">INFO</option>
              <option value="WARN">WARN</option><option value="ERROR">ERROR</option>
            </select>
          </label>
          <label className="field">Cari
            <input value={logSearch} onChange={(event) => setLogSearch(event.target.value)} placeholder="Cari log" />
          </label>
          <button className="button" onClick={() => void loadLogs().catch((reason) => setError(reason instanceof Error ? reason.message : "Gagal memuat log."))}>Filter log</button>
          <a className="button" href={`/api/core/${query("dashboard/logs/export.csv", { level: logLevel, search: logSearch })}`}>Export CSV</a>
        </div>
        <div className="table-wrap"><table>
          <thead><tr><th>Waktu</th><th>Level</th><th>Sumber</th><th>Pesan</th></tr></thead>
          <tbody>
            {logs.map((log, index) => (
              <tr key={`${log.created_at}-${index}`}>
                <td>{wibTime(log.created_at)}</td><td>{log.level}</td><td>{log.source}</td><td>{log.message}</td>
              </tr>
            ))}
            {logs.length === 0 && <tr><td colSpan={4} className="muted">Tidak ada log.</td></tr>}
          </tbody>
        </table></div>
      </section>

      <section className="panel section">
        <span className="eyebrow">Trade</span>
        <h2>Riwayat trade</h2>
        <p className="muted">Filter transaksi yang telah dilaporkan EA; tabel menampilkan 10 per halaman. Unduh CSV untuk data lengkap.</p>
        <div className="actions">
          <label className="field">Dari<input type="date" value={tradeFilters.date_from} onChange={(event) => setTradeFilters({ ...tradeFilters, date_from: event.target.value })} /></label>
          <label className="field">Sampai<input type="date" value={tradeFilters.date_to} onChange={(event) => setTradeFilters({ ...tradeFilters, date_to: event.target.value })} /></label>
          <label className="field">Group ID<input value={tradeFilters.group_id} onChange={(event) => setTradeFilters({ ...tradeFilters, group_id: event.target.value })} placeholder="Group ID" /></label>
          <label className="field">Symbol<input value={tradeFilters.symbol} onChange={(event) => setTradeFilters({ ...tradeFilters, symbol: event.target.value })} placeholder="Symbol" /></label>
          <label className="field">Hasil
            <select value={tradeFilters.result} onChange={(event) => setTradeFilters({ ...tradeFilters, result: event.target.value })}>
              <option value="">Semua hasil</option><option value="WIN">WIN</option>
              <option value="LOSS">LOSS</option><option value="BE">BE</option>
            </select>
          </label>
          <button className="button" onClick={() => void loadTrades(0).catch((reason) => setError(reason instanceof Error ? reason.message : "Gagal memuat trade."))}>Filter trade</button>
          <a className="button" href={`/api/core/${query("dashboard/trades/export.csv", {
            date_from: tradeFilters.date_from ? `${tradeFilters.date_from}T00:00:00` : "",
            date_to: tradeFilters.date_to ? `${tradeFilters.date_to}T23:59:59` : "",
            group_id: tradeFilters.group_id, symbol: tradeFilters.symbol, result: tradeFilters.result,
          })}`}>Export CSV</a>
        </div>
        <div className="table-wrap"><table>
          <thead><tr><th>Waktu</th><th>Grup</th><th>Ticket</th><th>Symbol</th><th>Arah</th><th>Lot</th><th>Entry</th><th>Profit</th><th>Hasil</th></tr></thead>
          <tbody>
            {trades.map((trade) => (
              <tr key={trade.ticket}>
                <td>{wibTime(trade.opened_at)}</td><td>{trade.group_name || "—"}</td><td>{trade.ticket}</td>
                <td>{trade.symbol}</td><td>{trade.action}</td><td>{formatNumber(trade.lots, 2, 3)}</td>
                <td>{formatNumber(trade.entry, 2, 5)}</td>
                <td className={toneClass(numberTone(trade.profit))}>{trade.profit === null || trade.profit === undefined ? "—" : formatNumber(trade.profit, 2, 2)}</td>
                <td>{trade.result || "—"}</td>
              </tr>
            ))}
            {trades.length === 0 && <tr><td colSpan={9} className="muted">Tidak ada trade.</td></tr>}
          </tbody>
        </table></div>
        <div className="actions">
          <button className="button" disabled={tradePage === 0} onClick={() => void loadTrades(tradePage - 1)}>Sebelumnya</button>
          <span className="muted">Halaman {tradePage + 1}</span>
          <button className="button" disabled={!hasMoreTrades} onClick={() => void loadTrades(tradePage + 1)}>Berikutnya</button>
        </div>
      </section>

      <section className="panel section">
        <span className="eyebrow">Risk &amp; configuration</span>
        <h2>Settings</h2>
        <p className="muted">Memengaruhi validasi sinyal dan guard backend. Trading berjalan live; gunakan kill switch untuk menjeda distribusi sinyal.</p>
        <form onSubmit={(event) => void submitSettings(event)}>
          <h3>Aturan sinyal</h3>
          <label className="field">Confidence threshold
            <input name="confidence_threshold" type="number" min="0" max="1" step="0.01" defaultValue={String(settings.confidence_threshold ?? 0.75)} key={`ct-${String(settings.confidence_threshold)}`} />
            <span className="muted">Minimum keyakinan parser agar sinyal diteruskan.</span>
          </label>
          <label className="field">Deviasi harga maksimal (%)
            <input name="max_market_deviation_pct" type="number" min="0.1" step="0.1" defaultValue={String(settings.max_market_deviation_pct ?? 5)} key={`mmd-${String(settings.max_market_deviation_pct)}`} />
            <span className="muted">Tolak market entry yang terlalu jauh dari harga sinyal.</span>
          </label>
          <label className="field">Mode entry
            <select name="entry_mode" defaultValue={String(settings.entry_mode ?? "PARTIAL")} key={`em-${String(settings.entry_mode)}`}>
              <option value="PARTIAL">Partial entry</option>
              <option value="SINGLE">Single entry</option>
            </select>
          </label>
          <label className="field">Symbol mapping (JSON)
            <textarea name="symbol_mapping" rows={4} value={symbolMapping} onChange={(event) => setSymbolMapping(event.target.value)} />
            <span className="muted">Alias simbol sinyal ke nama instrumen broker.</span>
          </label>
          <label className="field">
            <span><input name="allow_updates" type="checkbox" defaultChecked={settings.allow_updates === true} key={`au-${String(settings.allow_updates)}`} /> Izinkan update signal</span>
            <span className="muted">Aktifkan pengelolaan posisi seperti close atau geser SL.</span>
          </label>
          <label className="field">
            <span><input name="kill_switch" type="checkbox" defaultChecked={settings.kill_switch === true} key={`ks-${String(settings.kill_switch)}`} /> <strong>Pause trading (kill switch)</strong></span>
            <span className="muted">Hentikan distribusi sinyal ke EA sampai dimatikan.</span>
          </label>
          <div className="actions">
            <button className="button primary" disabled={busySettings} type="submit">{busySettings ? "Menyimpan…" : "Simpan settings"}</button>
          </div>
          {settingsMessage && <p className="muted" role="status">{settingsMessage}</p>}
        </form>
        <form onSubmit={(event) => void submitGemini(event)}>
          <h3>Parser Gemini</h3>
          <label className="field">Model
            <input name="gemini_model" defaultValue={typeof settings.gemini_model === "string" ? settings.gemini_model : ""} key={`gm-${String(settings.gemini_model)}`} placeholder="gemini-3.8-flash" />
          </label>
          <div className="actions">
            <button className="button" type="submit">Simpan model</button>
          </div>
          {geminiMessage && <p className="muted" role="status">{geminiMessage}</p>}
        </form>
      </section>
    </>
  );
}
