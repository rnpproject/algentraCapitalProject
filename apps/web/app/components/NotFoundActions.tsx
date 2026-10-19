"use client";

import Link from "next/link";
import { usePathname, useRouter } from "next/navigation";

/** Bagian interaktif halaman 404: path yang diminta + tombol navigasi. */
export function NotFoundActions() {
  const pathname = usePathname() ?? "";
  const router = useRouter();

  function goBack() {
    if (window.history.length > 1) router.back();
    else router.push("/");
  }

  return (
    <>
      <code className="not-found-path">{pathname}</code>
      <div className="not-found-actions">
        <Link href="/" aria-label="Kembali ke beranda">
          Kembali ke beranda
          <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round" aria-hidden="true"><path d="M5 12h14M13 6l6 6-6 6" /></svg>
        </Link>
        <button type="button" className="secondary" onClick={goBack}>
          Halaman sebelumnya
        </button>
        <Link className="secondary" href="/login">
          Masuk
        </Link>
      </div>
    </>
  );
}
