import Link from "next/link";

export function SiteFrame({ children }: Readonly<{ children: React.ReactNode }>) {
  return (
    <>
      <header className="shell topbar">
        <Link className="brand" href="/" aria-label="Algentra Capital beranda">
          <span className="brand-mark" aria-hidden="true" /> ALGENTRA CAPITAL
        </Link>
        <nav className="nav" aria-label="Navigasi utama">
          <Link href="/#platform">Platform</Link>
          <Link href="/chat">Bantuan</Link>
          <Link href="/login">Masuk</Link>
          <Link href="/register">Daftar</Link>
        </nav>
      </header>
      {children}
      <footer className="shell footer">Algentra Capital · Copy trading MT5 dengan kontrol yang jelas.</footer>
    </>
  );
}
