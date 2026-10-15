"use client";

import { FormEvent, useEffect, useState } from "react";
import { apiRequest } from "../lib/api";

type ChatMessage = { role: "user" | "assistant"; content: string };
type History = { messages?: ChatMessage[]; mode?: string; csrf_token?: string | null };

export function CustomerChatPanel() {
  const [messages, setMessages] = useState<ChatMessage[]>([]);
  const [draft, setDraft] = useState("");
  const [mode, setMode] = useState("public");
  const [error, setError] = useState("");
  const [busy, setBusy] = useState(false);
  useEffect(() => {
    apiRequest<History>("cs/chat/history").then((history) => {
      setMessages(history.messages ?? []);
      setMode(history.mode ?? "public");
      if (history.csrf_token) sessionStorage.setItem("algentra-csrf", history.csrf_token);
    }).catch((reason: unknown) => setError(reason instanceof Error ? reason.message : "Riwayat chat tidak tersedia."));
  }, []);
  async function send(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (!draft.trim()) return;
    setBusy(true); setError("");
    try {
      const response = await apiRequest<{ mode: string; message: string }>("cs/chat/message", {
        method: "POST", body: JSON.stringify({ message: draft })
      });
      setMessages((items) => [...items, { role: "user", content: draft }, { role: "assistant", content: response.message }]);
      setMode(response.mode); setDraft("");
    } catch (reason) { setError(reason instanceof Error ? reason.message : "Chat sementara tidak tersedia."); }
    finally { setBusy(false); }
  }
  return <section className="panel section">
    <span className="eyebrow">Bantuan · {mode === "client" ? "Portal klien" : "Pengunjung"}</span><h2>Chat ALGENTRA</h2>
    <div className="chat-log" aria-live="polite">{messages.map((message, index) => <p className={`chat-message ${message.role}`} key={`${index}-${message.role}`}><strong>{message.role === "user" ? "Anda" : "ALGENTRA"}</strong><br />{message.content}</p>)}
      {!messages.length && <p className="muted">Tanyakan informasi umum seputar layanan Algentra Capital.</p>}
    </div>
    {error && <p className="error" role="alert">{error}</p>}
    <form onSubmit={(event) => void send(event)}><label className="field">Pesan<textarea rows={3} maxLength={2000} required value={draft} onChange={(event) => setDraft(event.target.value)} /></label><button className="button primary" disabled={busy} aria-busy={busy}>{busy ? "Mengirim…" : "Kirim pesan"}</button></form>
  </section>;
}
