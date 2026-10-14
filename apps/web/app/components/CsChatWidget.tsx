"use client";

import { FormEvent, useCallback, useEffect, useRef, useState } from "react";
import { createPortal } from "react-dom";
import Mascot from "./Mascot";
import { apiRequest } from "../lib/api";

type ChatRole = "user" | "assistant";
type ChatMessage = { id: number; role: ChatRole; content: string };
type ChatHistory = {
  mode?: string;
  messages?: { role?: string; content?: string }[];
  csrf_token?: string | null;
};
type ChatReply = { mode?: string; message: string };

// Teks verbatim dari apps/core-api/app/templates (backup stabil added2df).
const WELCOME_TEXT: Record<"client" | "public", string> = {
  client: "Halo kak, saya ALGENTRA. Saya bisa bantu fitur portal dan pemasangan EA akun Anda.",
  public: "Halo, saya ALGENTRA. Tanya seputar fitur dan cara menggunakan layanan kami.",
};

const FOOTNOTE_TEXT: Record<"client" | "public", string> = {
  client: "Riwayat chat dihapus setelah 24 jam. Jangan kirim kata sandi atau OTP.",
  public: "Chat diproses Gemini; riwayat dihapus setelah 24 jam. Jangan kirim kata sandi atau OTP.",
};

const MODE_LABEL: Record<"client" | "public", string> = {
  client: "Asisten akun klien",
  public: "Asisten publik",
};

function normalizeMode(value: unknown, fallback: "client" | "public"): "client" | "public" {
  if (value === "client" || value === "public") return value;
  return fallback;
}

function errorMessage(error: unknown, fallback: string): string {
  return error instanceof Error && error.message ? error.message : fallback;
}

/**
 * Widget chat CS — port dari markup + JS di base.html / public.html / account.html.
 * Self-contained: state open/close, riwayat, dan pengiriman dikelola internal.
 * Membuka panel juga bisa dipicu dari luar via window event "algentra:cs-open".
 */
export function CsChatWidget({ mode, className }: { mode: "client" | "public"; className?: string }) {
  const [open, setOpen] = useState(false);
  const [chatMode, setChatMode] = useState<"client" | "public">(mode);
  const [messages, setMessages] = useState<ChatMessage[]>([]);
  const [welcomeShown, setWelcomeShown] = useState(false);
  const [status, setStatus] = useState("");
  const [draft, setDraft] = useState("");
  const [sending, setSending] = useState(false);
  const [typing, setTyping] = useState(false);
  const [mounted, setMounted] = useState(false);

  const idRef = useRef(0);
  const historyRequested = useRef(false);
  const messagesRef = useRef<HTMLDivElement>(null);
  const inputRef = useRef<HTMLTextAreaElement>(null);

  const nextId = () => {
    idRef.current += 1;
    return idRef.current;
  };

  // Varian layout: "client" = floating icon ala account.html, "public" = inline ala public.html.
  const isAccount = mode === "client";

  useEffect(() => {
    setMounted(true);
  }, []);

  const loadHistory = useCallback(async () => {
    try {
      const data = await apiRequest<ChatHistory>("cs/chat/history");
      const nextMode = normalizeMode(data.mode, mode);
      setChatMode(nextMode);
      if (data.csrf_token) {
        try {
          sessionStorage.setItem("algentra-csrf", data.csrf_token);
        } catch {
          /* Storage may be disabled. */
        }
      }
      const items = (data.messages ?? []).map((item) => ({
        id: nextId(),
        role: item.role === "user" ? ("user" as const) : ("assistant" as const),
        content: item.content ?? "",
      }));
      setMessages(items);
      setWelcomeShown(items.length === 0);
      setStatus("");
    } catch (error) {
      setMessages([]);
      setWelcomeShown(true);
      setStatus(errorMessage(error, "Chat belum dapat dimuat."));
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [mode]);

  const openPanel = useCallback(() => {
    setOpen(true);
    if (!historyRequested.current) {
      historyRequested.current = true;
      void loadHistory();
    }
  }, [loadHistory]);

  // Event dari launcher di landing (tombol "Chat CS" di kartu kontak).
  useEffect(() => {
    const handleExternalOpen = () => openPanel();
    window.addEventListener("algentra:cs-open", handleExternalOpen);
    return () => window.removeEventListener("algentra:cs-open", handleExternalOpen);
  }, [openPanel]);

  // Escape menutup panel.
  useEffect(() => {
    const handleKey = (event: KeyboardEvent) => {
      if (event.key === "Escape") setOpen(false);
    };
    document.addEventListener("keydown", handleKey);
    return () => document.removeEventListener("keydown", handleKey);
  }, []);

  // Fokus ke input saat panel dibuka.
  useEffect(() => {
    if (open) inputRef.current?.focus();
  }, [open ]);

  // Selalu gulir ke pesan terbaru.
  useEffect(() => {
    const box = messagesRef.current;
    if (box && open) box.scrollTop = box.scrollHeight;
  }, [messages, typing, open, welcomeShown]);

  const send = async (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    const message = draft.trim();
    if (!message || sending) return;
    const userId = nextId();
    setSending(true);
    setStatus("");
    setWelcomeShown(false);
    setMessages((items) => [...items, { id: userId, role: "user", content: message }]);
    setDraft("");
    if (typeof document !== "undefined" && document.visibilityState === "visible") {
      await new Promise((resolve) => requestAnimationFrame(() => requestAnimationFrame(resolve)));
    }
    setTyping(true);
    try {
      const data = await apiRequest<ChatReply>("cs/chat/message", {
        method: "POST",
        body: JSON.stringify({ message }),
      });
      setChatMode(normalizeMode(data.mode, chatMode));
      setTyping(false);
      setMessages((items) => [...items, { id: nextId(), role: "assistant", content: data.message }]);
      setStatus("");
    } catch (error) {
      setTyping(false);
      // Faithful ke original: bubble user dihapus, draf dikembalikan ke input.
      setMessages((items) => items.filter((item) => item.id !== userId));
      setDraft(message);
      setStatus(errorMessage(error, "Chat ALGENTRA sedang tidak tersedia."));
    } finally {
      setSending(false);
      inputRef.current?.focus();
    }
  };

  const panel = (
    <section
      className={`cs-chat-panel${isAccount ? " cs-chat-panel--floating" : ""}`}
      id="cs-chat-panel"
      aria-label="Chat ALGENTRA"
      hidden={!open}
    >
      <header className="cs-chat-head">
        <div className="cs-chat-brand">
          <span className="cs-chat-avatar">
            <Mascot />
          </span>
          <span>
            <span className="cs-chat-title">ALGENTRA</span>
            <span className="cs-chat-mode" id="cs-chat-mode">
              {MODE_LABEL[chatMode]}
            </span>
          </span>
        </div>
        <button
          className="secondary cs-chat-close"
          id="cs-chat-close"
          type="button"
          aria-label="Tutup chat"
          onClick={() => setOpen(false)}
        >
          ×
        </button>
      </header>
      <div
        className="cs-chat-messages"
        id="cs-chat-messages"
        ref={messagesRef}
        role="log"
        aria-live="polite"
        aria-relevant="additions text"
      >
        {welcomeShown && messages.length === 0 && (
          <p className="cs-chat-welcome">{WELCOME_TEXT[chatMode]}</p>
        )}
        {messages.map((item) => (
          <div
            key={item.id}
            className={`cs-chat-bubble${item.role === "user" ? " cs-chat-bubble--user" : ""}`}
          >
            {item.content}
          </div>
        ))}
        {typing && (
          <div
            className="cs-chat-bubble cs-chat-bubble--typing"
            role="status"
            aria-label="ALGENTRA sedang mengetik"
          >
            <span aria-hidden="true">
              <i className="cs-chat-typing-dot" />
              <i className="cs-chat-typing-dot" />
              <i className="cs-chat-typing-dot" />
            </span>
          </div>
        )}
      </div>
      <p className="cs-chat-footnote">{FOOTNOTE_TEXT[chatMode]}</p>
      <p className="cs-chat-status" id="cs-chat-status" role="status">
        {status}
      </p>
      <form className="cs-chat-form" id="cs-chat-form" onSubmit={(event) => void send(event)}>
        <textarea
          className="cs-chat-input"
          id="cs-chat-input"
          ref={inputRef}
          rows={1}
          maxLength={2000}
          placeholder="Tulis pesan..."
          aria-label="Pesan untuk ALGENTRA"
          required
          value={draft}
          disabled={sending}
          onChange={(event) => setDraft(event.target.value)}
        />
        <button className="cs-chat-send" id="cs-chat-send" type="submit" disabled={sending}>
          Kirim
        </button>
      </form>
    </section>
  );

  return (
    <div
      className={["cs-chat-widget", isAccount ? "cs-chat-widget--account" : "", className]
        .filter(Boolean)
        .join(" ")}
      id="cs-chat-widget"
      data-cs-mode={chatMode}
    >
      {/* Panel dipindah ke <body> seperti document.body.append(panel) di base.html. */}
      {mounted ? createPortal(panel, document.body) : null}
      <button
        className="cs-chat-launcher cs-chat-launcher--icon"
        id="cs-chat-open"
        type="button"
        aria-label="Buka chat dengan ALGENTRA"
        title="Chat dengan ALGENTRA"
        aria-expanded={open}
        aria-controls="cs-chat-panel"
        onClick={() => (open ? setOpen(false) : openPanel())}
      >
        <Mascot />
        <span className="cs-chat-launcher__label">
          {isAccount ? "Chat dengan ALGENTRA, asisten akun klien" : "Chat dengan ALGENTRA"}
        </span>
      </button>
    </div>
  );
}
