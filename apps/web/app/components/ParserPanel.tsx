"use client";

import { FormEvent, useState } from "react";
import { apiRequest } from "../lib/api";

export function ParserPanel() {
  const [message, setMessage] = useState("");
  const [output, setOutput] = useState("");
  const [error, setError] = useState("");
  const [busy, setBusy] = useState(false);
  async function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault(); setBusy(true); setError(""); setOutput("");
    try {
      const result = await apiRequest<unknown>("parser/test", { method: "POST", body: JSON.stringify({ message }) });
      setOutput(JSON.stringify(result, null, 2));
    } catch (reason) { setError(reason instanceof Error ? reason.message : "Parser tidak dapat dijalankan."); }
    finally { setBusy(false); }
  }
  return <section className="panel section"><span className="eyebrow">Peralatan admin</span><h2>Uji parser</h2>
    <form onSubmit={submit}><label className="field">Pesan sinyal<textarea required rows={5} maxLength={8000} value={message} onChange={(event) => setMessage(event.target.value)} /></label>
      {error && <p className="error">{error}</p>}<button className="button primary" disabled={busy}>{busy ? "Memeriksa…" : "Uji klasifikasi"}</button>
    </form>{output && <pre className="panel" style={{ overflow: "auto", marginTop: 18 }}>{output}</pre>}
  </section>;
}
