"use client";

import { FormEvent, useEffect, useState } from "react";
import { apiRequest, apiUpload } from "../lib/api";

type PaymentOrder = {
  id: number; label: string; server: string; login: string; plan: string; execution_mode: string;
  total_idr: number; status: string; created_at: string; has_proof: boolean; notification_sent: boolean;
};

const rupiah = (value: number) =>
  new Intl.NumberFormat("id-ID", { style: "currency", currency: "IDR", maximumFractionDigits: 0 }).format(value);
const orderDate = (value: string) =>
  value ? `${new Date(value).toLocaleString("id-ID", { timeZone: "Asia/Jakarta", dateStyle: "medium", timeStyle: "short" })} WIB` : "—";
const STATUS_NAMES: Record<string, string> = {
  PENDING_PAYMENT: "Menunggu pembayaran",
  PAYMENT_SUBMITTED: "Sedang diproses",
  APPROVED: "Transaksi diterima",
  REJECTED: "Transaksi tidak valid",
  CANCELLED: "Dibatalkan",
};
const statusTone = (status: string) =>
  status === "APPROVED" ? "tone-pos" : ["REJECTED", "CANCELLED"].includes(status) ? "tone-neg" : "tone-warn";

export function TransactionsPanel() {
  const [orders, setOrders] = useState<PaymentOrder[]>([]);
  const [instructions, setInstructions] = useState<string | null>(null);
  const [filter, setFilter] = useState("ALL");
  const [expandedOrder, setExpandedOrder] = useState<number | null>(null);
  const [receiptOrder, setReceiptOrder] = useState<PaymentOrder | null>(null);
  const [error, setError] = useState("");
  const [message, setMessage] = useState("");
  const [busy, setBusy] = useState(false);

  async function load() {
    try {
      const [orderList, manual] = await Promise.all([
        apiRequest<PaymentOrder[]>("payments/orders"),
        apiRequest<{ instructions: string | null }>("payments/manual-instructions").catch(() => ({ instructions: null })),
      ]);
      setOrders(orderList);
      setInstructions(manual.instructions);
    } catch (reason) {
      setError(reason instanceof Error ? reason.message : "Gagal memuat pesanan.");
    }
  }

  useEffect(() => {
    void load();
    // Deep link dari halaman akun setelah membuat pesanan: /transactions?order=<id>
    const params = new URLSearchParams(window.location.search);
    const target = Number(params.get("order"));
    if (Number.isFinite(target) && target > 0) setExpandedOrder(target);
  }, []);

  async function cancel(id: number) {
    setError("");
    setMessage("");
    try {
      await apiRequest(`payments/orders/${id}/cancel`, { method: "POST" });
      setOrders((items) => items.map((item) => (item.id === id ? { ...item, status: "CANCELLED" } : item)));
    } catch (reason) {
      setError(reason instanceof Error ? reason.message : "Pesanan tidak dapat dibatalkan.");
    }
  }

  async function notify(id: number) {
    setError("");
    setMessage("");
    setBusy(true);
    try {
      const result = await apiRequest<{ notification_sent: boolean }>(`payments/orders/${id}/notify`, { method: "POST" });
      setMessage(result.notification_sent
        ? "Notifikasi transaksi terkirim ke admin."
        : "Bot belum dapat menghubungi admin. Periksa konfigurasi Telegram.");
      await load();
    } catch (reason) {
      setError(reason instanceof Error ? reason.message : "Gagal mengirim notifikasi.");
    } finally {
      setBusy(false);
    }
  }

  async function uploadProof(event: FormEvent<HTMLFormElement>, order: PaymentOrder) {
    event.preventDefault();
    setError("");
    setMessage("");
    const input = event.currentTarget.querySelector('input[type="file"]') as HTMLInputElement | null;
    const file = input?.files?.[0];
    if (!file) {
      setError("Pilih file bukti pembayaran terlebih dahulu.");
      return;
    }
    setBusy(true);
    try {
      const form = new FormData();
      form.append("file", file);
      const result = await apiUpload<PaymentOrder>(`payments/orders/${order.id}/proof`, form);
      setOrders((items) => items.map((item) => (item.id === order.id ? result : item)));
      setMessage("Bukti pembayaran terkirim dan menunggu pemeriksaan admin.");
    } catch (reason) {
      setError(reason instanceof Error ? reason.message : "Gagal mengunggah bukti.");
    } finally {
      setBusy(false);
    }
  }

  const visible = filter === "ALL" ? orders : orders.filter((order) => order.status === filter);

  return (
    <section className="panel section">
      <span className="eyebrow">Transaksi</span>
      <h2>Riwayat transaksi</h2>
      <p className="muted">Lihat status pesanan, instruksi pembayaran manual, nota, dan bukti transaksi.</p>

      {error && <p className="error" role="alert">{error}</p>}
      {message && <p className="success" role="status">{message}</p>}

      <label className="field">Filter status
        <select value={filter} onChange={(event) => setFilter(event.target.value)}>
          <option value="ALL">Semua status</option>
          {Object.entries(STATUS_NAMES).map(([code, label]) => (
            <option key={code} value={code}>{label}</option>
          ))}
        </select>
      </label>

      {visible.length === 0 && <p className="muted">{orders.length ? "Tidak ada transaksi untuk filter ini." : "Belum ada transaksi."}</p>}

      <div className="cards">
        {visible.map((order) => (
          <article className="card" key={order.id} id={`order-${order.id}`}>
            <span className={`eyebrow ${statusTone(order.status)}`}>{STATUS_NAMES[order.status] ?? order.status}</span>
            <h3>{order.label} — Pesanan #{order.id}</h3>
            <p className="muted">Paket {order.plan} · {rupiah(order.total_idr)}</p>
            <p className="muted">Mode: {order.execution_mode === "MANAGED" ? "Dikelola server" : "Pasang EA sendiri"}</p>
            <p className="muted">Login broker: {order.login} — {order.server}</p>
            <p className="muted">Tanggal pesanan: {orderDate(order.created_at)}</p>

            <div className="actions">
              {order.has_proof && (
                <button className="button" onClick={() => setReceiptOrder(order)}>Lihat nota pembayaran</button>
              )}
              {order.status === "PENDING_PAYMENT" && (
                <button className="button primary" onClick={() => setExpandedOrder(expandedOrder === order.id ? null : order.id)}>
                  {expandedOrder === order.id ? "Tutup pembayaran" : "Lanjutkan pembayaran"}
                </button>
              )}
              {["PENDING_PAYMENT", "PAYMENT_SUBMITTED"].includes(order.status) && (
                <button className="button" onClick={() => void cancel(order.id)}>Batalkan</button>
              )}
            </div>

            {expandedOrder === order.id && order.status === "PENDING_PAYMENT" && (
              <div>
                <h4>Instruksi pembayaran manual</h4>
                {instructions
                  ? <p style={{ whiteSpace: "pre-wrap" }}>{instructions}</p>
                  : <p className="muted">Instruksi pembayaran belum dikonfigurasi. Hubungi admin.</p>}
                <p className="muted">Jangan transfer sebelum instruksi resmi dikonfirmasi.</p>
                <form onSubmit={(event) => void uploadProof(event, order)}>
                  <label className="field">Unggah bukti pembayaran (JPG/PNG/WebP, maks 5 MB)
                    <input type="file" accept=".jpg,.jpeg,.png,.webp" required />
                  </label>
                  <button className="button primary" disabled={busy} type="submit">
                    {busy ? "Mengunggah…" : "Kirim bukti pembayaran"}
                  </button>
                </form>
              </div>
            )}

            {order.status === "PAYMENT_SUBMITTED" && (
              <p className="muted">
                {order.notification_sent
                  ? "Bukti sudah dikirim. Pembuatan akun sedang diproses dan menunggu pemeriksaan admin."
                  : "Bukti tersimpan, tetapi notifikasi ke admin belum terkirim."}
                {!order.notification_sent && (
                  <> <button className="button" disabled={busy} onClick={() => void notify(order.id)}>Kirim ulang notifikasi admin</button></>
                )}
              </p>
            )}
          </article>
        ))}
      </div>

      {receiptOrder && (
        <section className="panel section" role="dialog" aria-label={`Nota transaksi #${receiptOrder.id}`}>
          <span className="eyebrow">Nota</span>
          <h2>Nota transaksi #{receiptOrder.id}</h2>
          <p className="muted">Status: {STATUS_NAMES[receiptOrder.status] ?? receiptOrder.status}</p>
          <p className="muted">{receiptOrder.label} · Paket {receiptOrder.plan} · {rupiah(receiptOrder.total_idr)}</p>
          {receiptOrder.has_proof ? (
            <>
              <h3>Bukti pembayaran</h3>
              {/* eslint-disable-next-line @next/next/no-img-element */}
              <img src={`/api/core/payments/orders/${receiptOrder.id}/proof`}
                alt={`Bukti pembayaran untuk transaksi ${receiptOrder.id}`}
                style={{ maxWidth: "100%", maxHeight: "55vh", objectFit: "contain" }} />
            </>
          ) : (
            <p className="muted">Bukti transaksi belum diunggah untuk pesanan ini.</p>
          )}
          <div className="actions">
            <button className="button" onClick={() => setReceiptOrder(null)}>Tutup</button>
          </div>
        </section>
      )}
    </section>
  );
}
