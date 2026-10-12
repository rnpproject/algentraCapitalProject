import Link from "next/link";
import { SiteFrame } from "./components/SiteFrame";

export default function HomePage() {
  return <SiteFrame>
    <main className="shell">
      <section className="hero">
        <span className="eyebrow">Trading infrastructure · Since 2019</span>
        <h1>Copy trading yang transparan dan terukur.</h1>
        <p className="lead">Pantau akun MT5, kelola strategi copy trading, dan kendalikan akses dari satu portal yang dirancang untuk kejelasan operasional.</p>
        <div className="actions">
          <Link className="button primary" href="/register">Buat akun</Link>
          <Link className="button" href="/login">Masuk ke portal</Link>
        </div>
      </section>
      <section className="cards" id="platform">
        <article className="card"><span className="eyebrow">01 · Kontrol</span><h2>Atur akses akun</h2><p>Kelola koneksi terminal follower dan pantau statusnya dari portal klien.</p></article>
        <article className="card"><span className="eyebrow">02 · Visibilitas</span><h2>Ringkasan yang jelas</h2><p>Performa ditampilkan berdasarkan laporan akun yang tersedia, dengan status data yang mudah dipahami.</p></article>
        <article className="card"><span className="eyebrow">03 · Perlindungan</span><h2>Akses terpisah</h2><p>Token perangkat dibuat khusus untuk tiap akun dan dapat dikelola melalui portal.</p></article>
      </section>
      <section className="panel section">
        <span className="eyebrow">Informasi pembayaran</span>
        <h2>Pembayaran QRIS segera tersedia</h2>
        <p className="muted">Jangan melakukan transfer atau mengirim bukti pembayaran melalui portal saat ini.</p>
      </section>
    </main>
  </SiteFrame>;
}
