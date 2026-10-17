export default function Loading() {
  return (
    <main className="app-loading" role="status" aria-live="polite" aria-label="Memuat halaman">
      <div className="app-loading__brand" aria-hidden="true">
        <span className="app-loading__mark">A</span>
        <span>ALGENTRA CAPITAL</span>
      </div>
      <span className="app-loading__spinner" aria-hidden="true" />
      <p>Menyiapkan halaman Anda</p>
      <span className="app-loading__hint">Mengambil data terbaru…</span>
    </main>
  );
}
