"use client";

import { FormEvent, Suspense, useEffect, useRef, useState } from "react";
import Link from "next/link";
import { useSearchParams } from "next/navigation";
import { apiRequest, apiUpload } from "../lib/api";

type Plan = {
  name: string;
  vcpu: string;
  ram: string;
  disk: string;
  price_idr: number;
};

type PaymentOrder = {
  id: number;
  label: string;
  server: string;
  login: string;
  plan: string;
  execution_mode: string;
  total_idr: number;
  status: string;
  created_at: string;
  has_proof: boolean;
  notification_sent: boolean;
};

const money = (value: number) =>
  `Rp ${new Intl.NumberFormat("id-ID", { maximumFractionDigits: 0 }).format(value || 0)}`;

const orderDate = (value: string) =>
  value
    ? `${new Date(value).toLocaleString("id-ID", {
        timeZone: "Asia/Jakarta",
        dateStyle: "medium",
        timeStyle: "short",
      })} WIB`
    : "-";

const statusNames: Record<string, string> = {
  PENDING_PAYMENT: "Menunggu pembayaran",
  PAYMENT_SUBMITTED: "Sedang diproses",
  APPROVED: "Transaksi diterima",
  REJECTED: "Transaksi tidak valid",
  CANCELLED: "Dibatalkan",
};

const filterOptions: Array<[string, string]> = [
  ["ALL", "Semua transaksi"],
  ["PENDING_PAYMENT", "Menunggu pembayaran"],
  ["PAYMENT_SUBMITTED", "Sedang diproses"],
  ["APPROVED", "Transaksi diterima"],
  ["REJECTED", "Tidak valid"],
  ["CANCELLED", "Dibatalkan"],
];

const instructionFallback =
  "Hubungi admin melalui kanal resmi untuk mendapatkan rekening tujuan. Jangan transfer sebelum detail rekening dan nominal dikonfirmasi.";

function OrderDetails({ order, plans }: { order: PaymentOrder; plans: Record<string, Plan> }) {
  const plan = plans[order.plan] || { name: order.plan, vcpu: "-", ram: "-", disk: "-" };
  const items: Array<[string, string]> = [
    ["Nomor transaksi", `#${order.id}`],
    ["Paket", `${plan.name} - ${plan.vcpu} - RAM ${plan.ram} - Disk ${plan.disk}`],
    ["Total pembayaran", money(order.total_idr)],
    ["Nama akun", order.label],
    [
      "Mode layanan",
      order.execution_mode === "MANAGED"
        ? "Dikelola server (MT5 berjalan di worker)"
        : "Pasang EA di MT5 sendiri",
    ],
    ["Login broker", `${order.login} - ${order.server}`],
    ["Tanggal pesanan", orderDate(order.created_at)],
  ];
  return (
    <dl className="transaction-details">
      {items.map(([label, value]) => (
        <div key={label}>
          <dt>{label}</dt>
          <dd>{value}</dd>
        </div>
      ))}
    </dl>
  );
}

function TransactionsPanelInner() {
  const searchParams = useSearchParams();
  const [orders, setOrders] = useState<PaymentOrder[]>([]);
  const [plans, setPlans] = useState<Record<string, Plan>>({});
  const [instructions, setInstructions] = useState<string | null>(null);
  const [filter, setFilter] = useState("ALL");
  const [expandedOrderId, setExpandedOrderId] = useState<number | null>(null);
  const [proofFormOrderId, setProofFormOrderId] = useState<number | null>(null);
  const [message, setMessage] = useState("");
  const [receiptOrder, setReceiptOrder] = useState<PaymentOrder | null>(null);
  const fingerprintRef = useRef("");
  const dialogRef = useRef<HTMLDialogElement>(null);

  async function loadOrders(force = false) {
    try {
      const data = await apiRequest<PaymentOrder[]>("payments/orders");
      const fingerprint = JSON.stringify(data);
      if (!force && fingerprint === fingerprintRef.current) return;
      fingerprintRef.current = fingerprint;
      setOrders(data);
    } catch (error) {
      setMessage(error instanceof Error ? error.message : "Permintaan gagal.");
    }
  }

  useEffect(() => {
    void loadOrders(true);
    apiRequest<Record<string, Plan>>("mt5/plans")
      .then(setPlans)
      .catch(() => {});
    apiRequest<{ instructions: string | null }>("payments/manual-instructions")
      .then((result) => setInstructions(result.instructions))
      .catch(() => {});
    const requested = Number(searchParams.get("order")) || null;
    if (requested && requested > 0) setExpandedOrderId(requested);
    const timer = setInterval(() => {
      if (!document.hidden) void loadOrders();
    }, 7000);
    return () => clearInterval(timer);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  useEffect(() => {
    if (receiptOrder && dialogRef.current && !dialogRef.current.open) {
      dialogRef.current.showModal();
    }
  }, [receiptOrder]);

  function closeReceipt() {
    dialogRef.current?.close();
  }

  async function notify(id: number) {
    try {
      const result = await apiRequest<{ notification_sent: boolean }>(
        `payments/orders/${id}/notify`,
        { method: "POST" }
      );
      setMessage(
        result.notification_sent
          ? "Notifikasi transaksi terkirim ke admin."
          : "Bot belum dapat menghubungi admin. Periksa konfigurasi Telegram."
      );
      await loadOrders(true);
    } catch (error) {
      setMessage(error instanceof Error ? error.message : "Permintaan gagal.");
    }
  }

  async function cancel(id: number) {
    if (!window.confirm("Batalkan pesanan ini?")) return;
    try {
      await apiRequest(`payments/orders/${id}/cancel`, { method: "POST" });
      setMessage("Pesanan dibatalkan.");
      setExpandedOrderId(null);
      setProofFormOrderId(null);
      await loadOrders(true);
    } catch (error) {
      setMessage(error instanceof Error ? error.message : "Permintaan gagal.");
    }
  }

  async function uploadProof(event: FormEvent<HTMLFormElement>, order: PaymentOrder) {
    event.preventDefault();
    const input = event.currentTarget.querySelector('input[type="file"]') as HTMLInputElement | null;
    const file = input?.files?.[0];
    if (!file) return;
    if (file.size > 5 * 1024 * 1024) {
      setMessage("Ukuran file bukti pembayaran maksimal 5 MB.");
      return;
    }
    const form = new FormData();
    form.append("file", file);
    try {
      await apiUpload(`payments/orders/${order.id}/proof`, form);
      setMessage("Bukti terkirim. Pembuatan akun sedang diproses sambil menunggu pemeriksaan admin.");
      setProofFormOrderId(null);
      setExpandedOrderId(null);
      await loadOrders(true);
    } catch (error) {
      setMessage(error instanceof Error ? error.message : "Permintaan gagal.");
    }
  }

  const visible = filter === "ALL" ? orders : orders.filter((order) => order.status === filter);

  return (
    <>
      <main className="page-shell dashboard-page" id="main-content">
        <div className="page-heading">
          <div>
            <span className="eyebrow">PORTAL KLIEN / PEMBAYARAN</span>
            <h1>Riwayat transaksi</h1>
            <p className="page-description">
              Lihat status pesanan, instruksi pembayaran manual, nota, dan bukti transaksi.
            </p>
          </div>
          <Link className="workspace-tag" href="/account">
            Kembali ke akun MT5
          </Link>
        </div>

        <section aria-labelledby="transactions-title">
          <h2 id="transactions-title">Pesanan dan pembayaran</h2>
          <div className="transaction-toolbar">
            <p className="section-help">
              Nota hanya dapat dibuka oleh pemilik akun. Bukti pembayaran yang dikirim tersimpan
              privat.
            </p>
            <label htmlFor="transaction-filter">
              Tampilkan
              <select
                id="transaction-filter"
                value={filter}
                onChange={(event) => setFilter(event.target.value)}
              >
                {filterOptions.map(([value, label]) => (
                  <option key={value} value={value}>
                    {label}
                  </option>
                ))}
              </select>
            </label>
          </div>
          <p id="transaction-message" className="status" role="status" aria-live="polite">
            {message}
          </p>
          <div id="transactions-list" className="grid" aria-live="polite">
            {visible.length === 0
              ? orders.length
                ? "Tidak ada transaksi untuk filter ini."
                : "Belum ada transaksi."
              : visible.map((order) => (
                  <article className="card transaction-card" key={order.id} id={`order-${order.id}`}>
                    <div className="transaction-card__top">
                      <h3>
                        {order.label} - Pesanan #{order.id}
                      </h3>
                      <span
                        className={`transaction-badge transaction-badge--${order.status.toLowerCase()}`}
                      >
                        {statusNames[order.status] || order.status}
                      </span>
                    </div>
                    <OrderDetails order={order} plans={plans} />

                    {order.status === "PAYMENT_SUBMITTED" && (
                      <p className="transaction-note">
                        {order.notification_sent
                          ? "Bukti sudah dikirim. Pembuatan akun sedang diproses dan menunggu pemeriksaan admin."
                          : "Bukti tersimpan, tetapi notifikasi ke admin belum terkirim."}
                      </p>
                    )}
                    {order.status === "APPROVED" && (
                      <p className="transaction-note">
                        {order.execution_mode === "MANAGED"
                          ? "Pembayaran diterima. Buka portal akun untuk mengatur kredensial broker; copy trading tetap dijeda sampai Anda memulainya."
                          : "Pembayaran diterima dan akun Copy Trading sudah dibuat."}
                      </p>
                    )}
                    {order.status === "REJECTED" && (
                      <p className="transaction-note">
                        Admin menandai pembayaran ini tidak valid. Hubungi admin untuk tindak lanjut.
                      </p>
                    )}

                    <div className="toolbar">
                      {order.has_proof && (
                        <button
                          type="button"
                          className="secondary"
                          onClick={() => setReceiptOrder(order)}
                        >
                          Lihat nota pembayaran
                        </button>
                      )}
                      {order.status === "PENDING_PAYMENT" && (
                        <button
                          type="button"
                          onClick={() =>
                            setExpandedOrderId((current) =>
                              current === order.id ? null : order.id
                            )
                          }
                        >
                          {expandedOrderId === order.id ? "Tutup pembayaran" : "Lanjutkan pembayaran"}
                        </button>
                      )}
                      {order.status === "PAYMENT_SUBMITTED" && !order.notification_sent && (
                        <button type="button" className="secondary" onClick={() => void notify(order.id)}>
                          Kirim ulang notifikasi admin
                        </button>
                      )}
                      {order.status === "APPROVED" && (
                        <button
                          type="button"
                          className="secondary"
                          onClick={() => window.location.assign("/account")}
                        >
                          Buka akun Copy Trading
                        </button>
                      )}
                      {(order.status === "PENDING_PAYMENT" ||
                        order.status === "PAYMENT_SUBMITTED") && (
                        <button
                          type="button"
                          className="secondary"
                          onClick={() => void cancel(order.id)}
                        >
                          Batalkan pesanan
                        </button>
                      )}
                    </div>

                    {order.status === "PENDING_PAYMENT" && expandedOrderId === order.id && (
                      <>
                        <p className="transaction-note">{instructions || instructionFallback}</p>
                        {proofFormOrderId !== order.id ? (
                          <button type="button" onClick={() => setProofFormOrderId(order.id)}>
                            Saya sudah bayar
                          </button>
                        ) : (
                          <form
                            className="transaction-proof-form"
                            onSubmit={(event) => void uploadProof(event, order)}
                          >
                            <label>
                              Unggah bukti transaksi (JPG, PNG, atau WebP, maksimal 5 MB)
                              <input
                                type="file"
                                name="file"
                                accept=".jpg,.jpeg,.png,.webp"
                                required
                              />
                            </label>
                            <button type="submit">Kirim bukti transaksi</button>
                          </form>
                        )}
                      </>
                    )}
                  </article>
                ))}
          </div>
        </section>
      </main>

      <dialog
        id="receipt-dialog"
        className="receipt-dialog"
        aria-labelledby="receipt-title"
        ref={dialogRef}
        onClick={(event) => {
          if (event.target === event.currentTarget) closeReceipt();
        }}
        onClose={() => setReceiptOrder(null)}
      >
        <div className="receipt-dialog__head">
          <strong>Nota pembayaran</strong>
          <button
            id="receipt-close"
            className="secondary"
            type="button"
            aria-label="Tutup nota"
            onClick={closeReceipt}
          >
            Tutup
          </button>
        </div>
        <div id="receipt-content" className="receipt-dialog__body">
          {receiptOrder && (
            <>
              <h2 id="receipt-title">Nota transaksi #{receiptOrder.id}</h2>
              <p className="transaction-note">
                Status: {statusNames[receiptOrder.status] || receiptOrder.status}
              </p>
              <OrderDetails order={receiptOrder} plans={plans} />
              {receiptOrder.has_proof ? (
                <>
                  <h3>Bukti pembayaran</h3>
                  {/* eslint-disable-next-line @next/next/no-img-element */}
                  <img
                    className="receipt-proof"
                    src={`/api/core/payments/orders/${receiptOrder.id}/proof`}
                    alt={`Bukti pembayaran untuk transaksi ${receiptOrder.id}`}
                  />
                </>
              ) : (
                <p className="receipt-empty">
                  Bukti transaksi belum diunggah untuk pesanan ini.
                </p>
              )}
            </>
          )}
        </div>
      </dialog>
    </>
  );
}

export function TransactionsPanel() {
  return (
    <Suspense>
      <TransactionsPanelInner />
    </Suspense>
  );
}
