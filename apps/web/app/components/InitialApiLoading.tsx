"use client";

import { useLayoutEffect, useRef, useState } from "react";

export function InitialApiLoading() {
  const [visible, setVisible] = useState(true);
  const pending = useRef(0);
  const timer = useRef<ReturnType<typeof setTimeout> | null>(null);

  useLayoutEffect(() => {
    const clearTimer = () => {
      if (timer.current) clearTimeout(timer.current);
      timer.current = null;
    };
    const scheduleHide = () => {
      clearTimer();
      if (pending.current > 0) return;
      timer.current = setTimeout(() => setVisible(false), 350);
    };
    const onStart = () => {
      pending.current += 1;
      clearTimer();
    };
    const onEnd = () => {
      pending.current = Math.max(0, pending.current - 1);
      scheduleHide();
    };
    window.addEventListener("algentra:api-start", onStart);
    window.addEventListener("algentra:api-end", onEnd);
    scheduleHide();
    return () => {
      clearTimer();
      window.removeEventListener("algentra:api-start", onStart);
      window.removeEventListener("algentra:api-end", onEnd);
    };
  }, []);

  if (!visible) return null;
  return (
    <div className="app-loading app-loading--overlay" role="status" aria-live="polite" aria-label="Memuat data terbaru">
      <div className="app-loading__brand" aria-hidden="true">
        <img className="brand-logo brand-logo--dark" src="/brand-kit/logo/algentra-horizontal-dark.svg" alt="" />
        <img className="brand-logo brand-logo--light" src="/brand-kit/logo/algentra-horizontal-light.svg" alt="" />
        <img className="brand-mark-image brand-mark-image--dark" src="/brand-kit/logo/algentra-mark-dark.svg" alt="" />
        <img className="brand-mark-image brand-mark-image--light" src="/brand-kit/logo/algentra-mark-light.svg" alt="" />
      </div>
      <span className="app-loading__spinner" aria-hidden="true" />
      <p>Menyiapkan halaman Anda</p>
      <span className="app-loading__hint">Mengambil data terbaru…</span>
    </div>
  );
}
