"use client";

import { FormEvent, useState } from "react";
import { apiRequest } from "../lib/api";

export function ParserPanel() {
  const [message, setMessage] = useState("");
  const [context, setContext] = useState("");
  const [result, setResult] = useState("Masukkan pesan untuk menguji parser.");

  async function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    setResult("Memproses…");
    try {
      const payload = await apiRequest<unknown>("parser/test", {
        method: "POST",
        body: JSON.stringify({ message, context: context || null }),
      });
      setResult(JSON.stringify(payload, null, 2));
    } catch (reason) {
      setResult(reason instanceof Error ? reason.message : "Permintaan tidak berhasil.");
    }
  }

  return (
    <main className="page-shell dashboard-page" id="main-content">
      <div className="page-heading">
        <div>
          <span className="eyebrow">Signal lab</span>
          <h1>Tes Parser Signal</h1>
          <p className="page-description">
            Uji format pesan sebelum mengaktifkan whitelist. Hasil ini hanya validasi parser dan tidak mengirim order
            ke MT5.
          </p>
        </div>
      </div>
      <section className="parser-form-panel">
        <form id="parser-form" className="parser-form" onSubmit={(event) => void submit(event)}>
          <div className="parser-field">
            <label htmlFor="message">
              Pesan baru
              <small className="field-help">
                Tempel pesan signal asli; format rentang seperti 4100:::4093 atau 4000-4010 menjadi zona; zona
                mengesampingkan kata BUY NOW/SELL NOW.
              </small>
            </label>
            <textarea
              id="message"
              required
              placeholder="Contoh: XAUUSD BUY 4100:::4093, TP 4120, SL 4085"
              value={message}
              onChange={(event) => setMessage(event.target.value)}
            />
          </div>
          <div className="parser-field parser-field--context">
            <label htmlFor="context">
              Konteks pesan sebelumnya (opsional)
              <small className="field-help">
                Isi hanya jika pesan terbaru melanjutkan arah/simbol dari pesan sebelumnya.
              </small>
            </label>
            <textarea
              id="context"
              placeholder="Contoh: BUY NOW"
              value={context}
              onChange={(event) => setContext(event.target.value)}
            />
          </div>
          <div className="parser-actions">
            <button type="submit">Parse pesan</button>
          </div>
        </form>
      </section>
      <section className="parser-result" aria-labelledby="parser-result-title">
        <div className="parser-result-heading">
          <h2 id="parser-result-title">Hasil parsing</h2>
          <p className="section-help">
            Periksa action, order type, zona entry, SL/TP, status validasi, dan alasan sebelum memakai sumber ini.
          </p>
        </div>
        <pre id="result" aria-live="polite">
          {result}
        </pre>
      </section>
    </main>
  );
}
