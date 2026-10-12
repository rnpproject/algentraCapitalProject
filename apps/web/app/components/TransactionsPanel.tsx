"use client";

import { useEffect, useState } from "react";
import { apiRequest } from "../lib/api";

type PaymentOrder = { id: number; label: string; plan: string; total_idr: number; status: string; created_at: string; has_proof: boolean };
const rupiah = (value: number) => new Intl.NumberFormat("id-ID", { style: "currency", currency: "IDR", maximumFractionDigits: 0 }).format(value);

export function TransactionsPanel() {
  const [orders, setOrders] = useState<PaymentOrder[]>([]);
  const [error, setError] = useState("");
  useEffect(() => {
    apiRequest<PaymentOrder[]>("payments/orders").then(setOrders).catch((reason: unknown) => {
      setError(reason instanceof Error ? reason.message : "Gagal memuat pesanan.");
    });
  }, []);
  async function cancel(id: number) {
    setError("");
    try {
      await apiRequest(`payments/orders/${id}/cancel`, { method: "POST" });
      setOrders((items) => items.map((item) => item.id === id ? { ...item, status: "CANCELLED" } : item));
    } catch (reason) { setError(reason instanceof Error ? reason.message : "Pesanan tidak dapat dibatalkan."); }
  }
  return <section className="panel section">
    <span className="eyebrow">Transaksi</span><h2>Pembayaran QRIS segera tersedia</h2>
    <p className="muted">Jangan melakukan transfer atau mengunggah bukti pembayaran. Pesanan lama yang belum dibayar dapat dibatalkan dari daftar.</p>
    {error && <p className="error" role="alert">{error}</p>}
    {orders.length === 0 ? <p className="muted">Belum ada pesanan.</p> : <div className="table-wrap"><table><thead><tr><th>Pesanan</th><th>Paket</th><th>Total</th><th>Status</th><th>Tanggal</th><th></th></tr></thead><tbody>
      {orders.map((order) => <tr key={order.id}><td>{order.label}</td><td>{order.plan}</td><td>{rupiah(order.total_idr)}</td><td>{order.status}</td><td>{order.created_at}</td><td>{["PENDING_PAYMENT", "PAYMENT_SUBMITTED"].includes(order.status) && <button className="button" onClick={() => void cancel(order.id)}>Batalkan</button>}</td></tr>)}
    </tbody></table></div>}
  </section>;
}
