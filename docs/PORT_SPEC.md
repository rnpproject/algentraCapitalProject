# PORT_SPEC — Faithful Port dari Template Jinja ke Next.js

> Dokumen kerja untuk implementasi ulang. Sumber kebenaran: 13 template Jinja di
> `apps/core-api/app/templates/` pada backup stabil (commit `added2df`, pra-PR-#13).
> Tujuan: halaman Next.js di `apps/web` harus **semirip mungkin** — tampilan (CSS/kelas/layout)
> dan fitur — dengan template asli. Jangan mengarang endpoint; semua endpoint di bawah
> benar-benar ada di kode template.

---

## 1. DESIGN SYSTEM (dari `base.html`, `<style>` baris ±10–685)

### 1.1 Font & warna dasar

```css
--font-body: "Plus Jakarta Sans", Inter, ui-sans-serif, system-ui, sans-serif;
--font-heading: Poppins, "Trebuchet MS", ui-rounded, system-ui, sans-serif;
body { font: 400 16px/1.75 var(--font-body); }
h1, h2, h3 { font-family: var(--font-heading); }
```

Tema default = **dark** (`<html data-theme="dark">`; theme dibaca dari cookie `theme`, fallback `dark`).
Toggle tema ada di topbar (`#theme-toggle`), disimpan ke cookie + localStorage.

| Variable | Dark (default) | Light (`:root[data-theme="light"]`) |
|---|---|---|
| `--bg` | `#0d1a24` | `#f2f2f2` |
| `--panel` | `#132432` | `#fff` |
| `--panel-raised` | `#1a2f40` | `#eaf3f8` |
| `--surface` | `#132432` | `#fff` |
| `--ink` | `#e8f0f5` | `#183345` |
| `--muted` | `#a3b6c3` | `#526573` |
| `--line` | `#2a4152` | `#d1dce2` |
| `--control` | `#6f8a9c` | `#7b8e99` |
| `--action` (tombol primer) | `#4da3e8` (biru) | `#035aa6` |
| `--on-action` (teks di atas tombol) | `#08121a` | `#fff` |
| `--accent` / `--gold` | `#f2c438` (emas) | `#f2c438` |
| `--accent-strong` | `#4da3e8` | `#035aa6` |
| `--danger` | `#f2a0a0` | `#9d2929` |
| `--link` | `#6db8f0` | `#035aa6` |
| `--focus` | `#6db8f0` | `#035aa6` |
| `--glass-surface` | `rgb(19 36 50 / 78%)` | `rgb(255 255 255 / 70%)` |
| `--glass-raised` | `rgb(26 47 64 / 82%)` | `rgb(234 243 248 / 78%)` |
| `--glass-border` | `rgb(255 255 255 / 16%)` | `rgb(255 255 255 / 82%)` |
| `--glass-menu` | `rgb(19 36 50 / 98%)` | `rgb(255 255 255 / 98%)` |
| `--shadow-soft` | `0 18px 48px rgb(0 0 0 / 12%)` | `0 18px 48px rgb(24 51 69 / 9%)` |
| `--radius-control` | `10px` | |
| `--radius-panel` | `16px` | |
| `--radius-dialog` | `20px` | |
| `--motion` | `300ms` | |

Warna tone (status): `.tone-pos #4ade80` / `.tone-neg #ff7b7b` / `.tone-warn #facc15` /
`.tone-info #6db4ff` / `.tone-muted var(--muted)` (light theme: pos `#14703b`, neg `#b42323`,
warn `#8a6100`, info `#0b5ea8`).

### 1.2 Katalog kelas penting

**Layout & navigasi**
- `.topbar` — sticky, glass (`backdrop-filter: blur(16px)`), tinggi min 78px, padding responsif.
- `.brand` + `.brand-mark` (SVG 29px) + `.brand-copy strong/small` — logo teks "Algentra Capital".
- `.site-nav` — container link; `.nav-link` (kartu 2-baris: `span` judul 13px bold + `small` deskripsi 10px),
  `.nav-link.active` (gradient gelap + border), `.nav-link:hover` (translateY -1px).
- `.topbar-side` — kanan: `#theme-toggle` (ikon sun/moon swap), `#nav-toggle` (hamburger, hanya ≤600px),
  `.desktop-client-logout` / `.mobile-client-logout`, `.workspace-tag` (pill "Portal klien").
- `.page-shell` — `width: min(1460px,100%)`, padding `38px 28px 80px`.
- `.page-heading` — flex, `.eyebrow` (kicker 10px, letterspacing .16em, uppercase, warna link),
  `h1` clamp(27–36px), `.page-description` / `.section-help` (13px muted, max 820px),
  `.heading-aside` (11px muted).

**Komponen generik**
- `section` — kartu glass: border `var(--glass-border)`, radius 16px, `backdrop-filter: blur(14px)`,
  padding 24px, margin 20px 0. Animasi `soft-enter` staggered per section.
- `.cards` — grid auto-fit min 145px; `.card` — glass-raised, hover scale(1.02);
  `.card span` (label 11px muted), `.card b` (nilai 22px tabular), `.card small`.
- `.grid` — grid auto-fit min 360px, gap 14px.
- `.panel` — padding + border + radius, background `var(--surface-2)` (dipakai di public).
- `table` — font 12px, `th` 10px uppercase letterspacing, border-bottom `#29383a`;
  `tr:hover td` highlight; `.table-wrap` overflow auto; `td.num` tabular-nums.
- `input, select, textarea` — min-height 44px, border `var(--control)`, radius 10px,
  background `var(--surface)`; focus ring `var(--focus)`. Select custom arrow (gradient).
  Checkbox custom 18px (centang via ::before). Radio custom 18px bulat (dipakai plan-picker).
- `button` — primer biru `var(--action)`, teks `var(--on-action)`, hover translateY(-2px) +
  brightness(1.06); `.secondary` (gelap), `.danger` (merah), `.icon-button` (44px).
- `label` — 12px bold `#d2deea`; `.field-help` — 11px muted di bawah label.
- `.form-grid` — grid auto-fit min 220px; `.toolbar` — flex wrap, input/select max 210px.
- `.muted`, `.status` (12px, min-height 1.5em); `.pill` (badge dengan dot via ::before).
- `.notice-box` — border-left 3px emas, background glass-raised, 12px.
- `.row` — baris grup Telegram (checkbox + nama + alias).
- `.login-shell` — grid center, min-height calc(100vh-80px); `.login-card` — max 440px,
  form label margin 13px, tombol full-width.
- `.plan-picker` / `.plan-options` (grid 3 kolom) / `.plan-option` (kartu radio 180px,
  `:has(input:checked)` border action + ring; `:has(input:disabled)` opacity .62) /
  `.plan-option__name` / `.plan-option__availability` (badge "Belum tersedia").
- `.account-connect-form` — grid `repeat(3, 1fr) auto`, plan-picker span full.
- `.account-state` + `.account-state__dot` + modifier `--ready/--waiting_for_source/--offline/--disabled/--trading_disabled/--not_real_account` (warna dot + teks).
- `.account-login-check` + `--valid/--error/--pending`; `.account-balance` (15px bold tabular);
  `.account-balance-time`, `.account-last-seen` (11px muted).
- `.pager` — pagination kanan; `.toggle` / `.toggle-list` — checkbox card di settings.
- `.settings-block`, `.settings-row`, `.settings-grid`, `.settings-footer` (dashboard settings).
- `.music-toggle` — pill fixed kanan-bawah (audio `/musik.mp3`, autoplay kecuali pref `algentra-music=off`).
- `.cs-chat-*` — widget chat CS (launcher bulat 48px fixed / panel 370px, bubble user/asisten,
  typing dots, welcome text). Lihat §2.4.

**Khusus dashboard** (semua di-scope `.dashboard-page`):
- Section/card solid (tanpa glass): `background: var(--panel)`, border `var(--line)`, radius 14px.
- `.summary-card[aria-label="Ringkasan operasional"]` — grid 2 kolom: `.sc-overview` | `.sc-signals`;
  `.sc-block`, `.sc-label` (uppercase 11px), `.sc-chip` (chip dengan dot ::before),
  `.sc-live`, `.sc-profit b` (32px), `.sc-metrics` (grid 3 kolom, b 17px),
  `.sc-table` (tabel ringkas tanpa border), `.sc-help` (details/summary "Arti istilah").
- `.sc-mt5` — kartu MT5 live; `#mt5-live-chip`, `#mt5-live-status`.
- `.dashboard-page .cards` — min 210px; card b 17px.
- `.parser-form`, `.parser-field`, `.parser-actions`, `.parser-result pre` (monospace, max-height 420px).
- `#registration-controls .toolbar` — tombol Buka/Tutup pendaftaran.

**Khusus public** (scope `.public-shell`, width min(1200px,100%)):
- `.section-kicker` (12px accent uppercase); `.hero` (grid 2 kolom, min-height 480px,
  radius 28px, dekorasi lingkaran ::after); `.signal-orbit` (orbit animasi statis:
  lingkaran konsentris + `.orbit-line` + `.orbit-core` 116px + 3 `.orbit-node`);
  `.hero-actions` (`.public-button` primer/sekunder).
- `.community-card` (grid emblem|copy|count), `.community-count strong` (32px accent).
- `.equity-layout` (2 kolom), `.equity-value` (clamp 24–46px), `.stat` (baris label|nilai),
  `.chart-wrap` (220px), `#equity-chart` (SVG: grid, area gradient, line, point, gap rect,
  axis labels), `.delta-chip`, `.chart-meta`.
- `.quote-list` (grid 4 kolom) / `.quote-card` (symbol + live-dot, bid/ask `.quote-line`,
  `.quote-meta` spread + umur) / `.tick-up` / `.tick-down` (animasi flash 1.1s).
- `.approach-grid` (3 kolom) / `.approach-card` (ikon + index "01 / SUMBER").
- `.contact-grid` (2 kolom) / `.contact-card` / `.contact-link` (62px, ikon + label + small)
  / `.contact-email-cs` / `.cs-chat-launcher.contact-link`.
- `.registration-launch` + `.registration-countdown` (4 unit Hari/Jam/Menit/Detik).
- `.closing` (CTA), `.risk-copy` (11px center).
- `.status-pill` (dengan `data-state="live|warn"`, dot + glow).

**Khusus transaksi** (scope halaman, kelas di `transactions.html` `<style>`):
- `.transaction-toolbar`, `.transaction-card`, `.transaction-card__top`,
  `.transaction-badge` (+ `--approved/--rejected/--cancelled/--pending_payment/--payment_submitted`),
  `.transaction-details` (dl grid), `.transaction-note` (border-left emas),
  `.transaction-qr` (260px), `.transaction-proof-form`, `.receipt-dialog`
  (native `<dialog>`, 720px) + `.receipt-dialog__head/body`, `.receipt-proof` (img),
  `.receipt-empty`.

**Khusus 404**: `.not-found-shell`, `.not-found-card` (grid 2 kolom, gradient gelap),
`.not-found-code` (pill), `.not-found-path` (code), `.not-found-actions`,
`.not-found-art` (SVG trader animasi: chart-draw, candle-tick, price-drift, trader-bob,
bubble-pop, sweat-bob, steam-rise).

> Catatan: CSS di atas adalah sistem desain yang harus di-port verbatim ke `globals.css`
> (atau CSS module setara). Yang sekarang ada di `apps/web/app/globals.css` adalah desain
> **berbeda** (aksen lime `#b9ea57`, font Inter, tanpa glassmorphism) — harus diganti.

### 1.3 Perilaku CSS penting
- Animasi masuk section: `soft-enter` 420ms staggered (nth-of-type delay 50/100/150ms).
- `prefers-reduced-motion: reduce` — matikan semua animasi/transisi.
- Responsive breakpoints: 940px (topbar wrap), 800px, 640px, 600px (mobile: nav jadi dropdown
  `.site-nav.is-open`, plan-options 1 kolom, quote-card jadi grid 3 kolom, dsb).

---

## 2. STRUKTUR LAYOUT `base.html`

### 2.1 `<head>`
- `<html lang="id" data-theme="dark|light">` — theme dari cookie `theme`, default `dark`.
- `<meta name="csrf-token" content="{{ csrf_token }}">` — dipakai SEMUA POST/PUT/DELETE via header `X-CSRF-Token`.
- `<meta name="description">` — beda teks untuk admin vs klien/publik.
- `<meta name="theme-color" content="#0d1a24">` (diupdate JS saat ganti tema).
- `<title>` per halaman via `{% block title %}`.

### 2.2 Header / navigasi (4 varian, dirender server dari session)

```
<header class="topbar">
  <a class="brand" href="/dashboard|/"> [SVG chart] Algentra Capital <small>…</small></a>
  <nav class="site-nav" id="site-nav"> …varian… </nav>
  <div class="topbar-side">
    <button id="theme-toggle">…</button>
    <button id="nav-toggle">…</button>            ← hanya tampil ≤600px
    [Logout / Keluar / workspace-tag bila login]
  </div>
</header>
```

| Kondisi session | Nav links |
|---|---|
| `admin_username` | Workspace (`/dashboard`), Parser test (`/parser-test`), Telegram (`/telegram-setup`), Controls (`/dashboard#settings`), Logout (desktop `#logout` + mobile `[data-workspace-logout]`) |
| `client_user_id` | Akun Copy Trading (`/account`), Riwayat transaksi (`/transactions`), Performa (`/#performance`), Keluar |
| path `/login` (anonim) | Algentra Capital (`/`), Portal klien (`/login`, active), Daftar (`/register`) atau Menunggu launching (`/#registration-launch`) bila registrasi tutup |
| publik lain | Beranda (`/`), Performa (`/#performance`), Cara kerja (`/#approach`), Kontak (`/#contact`), Portal klien (`/login`) |

Active state: server (`request.url.path`) + JS sinkronisasi hash (`syncWorkspaceNav`, `syncPublicNav`).

### 2.3 Footer
- Admin: `Algentra Capital Workspace · Review risk controls before enabling live orders`
- Lainnya: `Algentra Capital · Trading memiliki risiko; hasil historis bukan jaminan kinerja mendatang.`

### 2.4 Komponen global di body
- `<a class="skip-link" href="#main">Lewati ke konten</a>`
- `<audio id="site-music" loop>` + `<button id="music-toggle">` — autoplay `/musik.mp3`
  kecuali `localStorage.algentra-music === "off"`; label "Putar musik"/"Musik aktif".
- **CS chat widget** — dirender di `public.html` (kontak) dan `account.html` (floating icon),
  tapi **JS-nya global di base.html** (hanya bila bukan admin):
  `#cs-chat-widget > #cs-chat-open` (launcher) + `#cs-chat-panel` (hidden) berisi
  header (avatar maskot + "ALGENTRA" + `#cs-chat-mode`), `#cs-chat-messages`,
  `.cs-chat-footnote`, `#cs-chat-status`, `#cs-chat-form` (textarea + Kirim).
  Mode `data-cs-mode="client|public"`.

### 2.5 Behavior JS global (base.html, `<script>` akhir)

| Fungsi | Perilaku |
|---|---|
| `window.AlgFmt` | Format angka locale `id-ID`: `number/integer/decimal2/price/lots/money/percent`; `numberTone` (pos/neg); `statusTone` (mapping teks status → tone, cth. EXECUTED/WIN/ONLINE→pos, REJECTED/LOSS/OFFLINE→neg, PENDING/PAUSED→warn); `setTone` (set class `tone-*`); EMPTY = `—` |
| `startResendCountdown(button, seconds, label)` | Countdown kirim-ulang kode (dipakai register & forgot-password), baca `Retry-After` |
| `formatApiError` / `parseApiResponse` | Parse error API; khusus 502 non-JSON → pesan diagnostik Cloudflare Tunnel (cek backend port 8000, log cloudflared) |
| Theme toggle | `dark ↔ light`; simpan cookie `theme` (1 thn) + localStorage; update `data-theme` + meta theme-color + aria-label |
| Nav toggle | Buka/tutup `.site-nav.is-open` di mobile; tutup saat klik link / klik luar / Escape; reset >600px |
| `syncWorkspaceNav` / `syncPublicNav` | Toggle class `active` berdasar `location.hash` |
| Logout `[data-workspace-logout]` | `POST /api/auth/logout` header `X-CSRF-Token` dari meta → `location.assign("/login")` |
| Music toggle | Play/pause `#site-music`, simpan preferensi, autoplay saat load |
| CS chat | `loadHistory`: `GET /api/cs/chat/history` → mode + messages + csrf_token; submit: `POST /api/cs/chat/message` `{message}` → bubble user + typing indicator + bubble assistant; Enter/Escape handling; footnote "Chat diproses Gemini; riwayat dihapus setelah 24 jam. Jangan kirim kata sandi atau OTP." |

---

## 3. SPESIFIKASI PER HALAMAN

Konvensi endpoint: template lama memanggil `fetch("/api/...")` **relatif ke origin yang sama**
(karena dulu Core API menyajikan halaman). Di Next.js semua harus lewat BFF:
`/api/core/<path>` → proxy ke `<CORE_API_ORIGIN>/v1/<path>` (kecuali `/downloads/*`
yang diproxy tanpa `/v1`). Jadi `/api/stats` lama = `apiRequest("dashboard/stats")` baru, dst.
Lihat tabel mapping di §4.

### 3.1 `public.html` → `/` (landing publik)

**Markup** (`main.public-shell`):
1. `section.hero` — `.hero-copy`: `.section-kicker` ("Analisis AI • Copy Trading • Kontrol Risiko"),
   `h1` "Sinyal dianalisis AI, eksekusi berjalan *otomatis.*", `.hero-text`, `.hero-actions`
   (Lihat performa → `#performance`, Pelajari cara kerja → `#approach`, Daftar client → `/register`
   atau Menunggu launching → `/#registration-launch` bila tutup).
   `.hero-visual`: `.signal-orbit` (orbit-line, 3 orbit-node, `.orbit-core` SVG + "AI • MT5"),
   `.visual-caption` "Dari sinyal ke eksekusi otomatis".
2. `section.registration-launch#registration-launch` — **hanya bila `registration_open == false`**:
   countdown (`.registration-countdown` + `#registration-days/hours/minutes/seconds`,
   `data-launch-at="{{ registration_launch_at }}"`, target **16 Januari 2027 09.00 WIB**)
   bila `registration_launch_pending`, atau pesan "Pendaftaran sedang ditutup".
3. `section.community-card` — emblem SVG, "Pengguna terverifikasi",
   `#registered-client-count` ("—" awal, live via API).
4. `section.public-section#performance` — `.section-heading` + `#master-account-status.status-pill`;
   `.equity-panel`: `.equity-layout` → `.equity-info` (`#equity-label`, `#equity-value.equity-value`,
   `.equity-stats` 4 `.stat`: Saldo MT5 / Floating profit / Terendah sesi / Tertinggi sesi,
   `#equity-updated`) + `.equity-chart` (`.chart-head`: "Realtime Equity" + `#equity-chart-delta.delta-chip`;
   `#equity-chart-wrap.chart-wrap` → `#equity-chart-empty.chart-empty` / `svg#equity-chart`
   (grid, area, line, point, gap rect+label, y/x labels) / `#equity-chart-meta.chart-meta`
   (`#equity-chart-first`, `#equity-chart-latest`)).
   `.watchlist-panel`: head "Watchlist harga" + `#market-status.status-pill` (awal RECONNECT/warn);
   `#market-prices.quote-list` (4 `.quote-card`: symbol + live-dot, Bid/Ask `.quote-line`,
   `.quote-meta` spread + umur).
5. `section.public-section#approach` — `.approach-grid` 3 `.approach-card`
   (01/SUMBER "Pilih kanal sinyal", 02/ANALISIS AI "Susun detail sinyal", 03/OTOMASI MT5 "Eksekusi dan salin").
6. `section.public-section#contact` — `.contact-grid`: kartu "Narahubung"
   (`contact_person`, link WhatsApp `https://wa.me/{{ contact_whatsapp }}`, email `mailto:`,
   **CS chat launcher** `.contact-link.cs-chat-launcher#cs-chat-open` dengan maskot) +
   kartu "Kanal resmi" (loop `contact_socials`: instagram/telegram/tiktok/website/generic + label/display).
7. `section.closing` — CTA "Copy trading otomatis dengan AI." + tombol daftar/tunggu.
8. `p.risk-copy` — disclaimer risiko.

**Jinja vars (server):** `registration_open`, `registration_launch_pending`,
`registration_launch_at`, `contact_person`, `contact_whatsapp`, `contact_email`,
`contact_socials[]` (`{url, icon, label, display}`).

**JS / API:**
- `GET /api/public/registration-status` tiap 10 dtk → `location.reload()` bila berubah.
- `GET /api/public/performance` tiap **1 dtk** (`?include_equity_curve=false` untuk poll ringan;
  curve penuh tiap 60 dtk). Response: `{registered_clients, master_account:{account_key, online,
  available, equity, balance, floating_profit, currency, trade_mode, updated_at}, equity_curve:[{hour, equity, currency}], market_quotes:[{symbol, bid, ask, updated_at}], market_quotes_online}`.
- Grafik equity: SVG digambar manual (sesi mulai 07.00 WIB, padding, gap bila data kosong,
  delta chip ±/%, low/high sesi). Watchlist: XAUUSD/EURUSD/USDJPY/GBPUSD, digit
  {XAUUSD:2, EURUSD:5, USDJPY:3, GBPUSD:5}, animasi tick-up/tick-down, umur harga
  ("baru saja/…dtk lalu"), status ONLINE/RECONNECT. Countdown: update tiap detik.

### 3.2 `login.html` → `/login`

- `main.page-shell.dashboard-page.login-shell > section.login-card`
- Eyebrow `PORTAL KLIEN / MASUK`, h1 "Masuk ke akun", deskripsi "Kelola akun Copy Trading MT5 dan pantau koneksinya."
- Form `#client-login-form`: Email, Kata sandi, checkbox `#remember-me`
  "Biarkan saya tetap masuk selama 30 hari".
- `POST /api/auth/client-login` `{email, password, remember_me}` → `location.assign("/account")`.
- Link: `/forgot-password` ("Lupa kata sandi?"), `/register` ("Buat akun dengan verifikasi email").
- Error ke `#client-message.status`.

### 3.3 `register.html` → `/register`

- Eyebrow `REGISTRASI KLIEN / VERIFIKASI EMAIL`, h1 "Buat akun klien".
- Form 1 `#register-form`: Email + Kata sandi (min 12, max 72 byte, field-help
  "Minimal 12 karakter; maksimal 72 byte.") → `POST /api/auth/register` `{email, password}`
  → sembunyikan form 1, tampilkan form 2; `startResendCountdown(#resend, result.retry_after_seconds)`.
- Form 2 `#verify-form` (hidden): Kode verifikasi 6 digit (`inputmode=numeric`, pattern `[0-9]{6}`,
  field-help "…Kode berlaku 10 menit.") → `POST /api/auth/verify-email` `{email, code}`
  → `location.assign("/account")`.
- Tombol `#resend.secondary` "Kirim ulang kode" → `POST /api/auth/resend-code` `{email, password}`.
- Error: baca header `Retry-After` / `result.retry_after_seconds` → countdown di tombol yang relevan.
- Link "Sudah punya akun? Masuk ke portal klien" → `/login`.

### 3.4 `forgot_password.html` → `/forgot-password`

- Eyebrow `PORTAL KLIEN / RESET KATA SANDI`, h1 "Lupa kata sandi?".
- Form 1 `#reset-request-form`: Email akun → `POST /api/auth/password-reset/request` `{email}`.
- Form 2 `#reset-confirm-form` (hidden): Kode email 6 digit + Kata sandi baru (min 12/max 72)
  + Ulangi kata sandi baru (cek sama di client: "Konfirmasi kata sandi tidak sama.")
  → `POST /api/auth/password-reset/confirm` `{email, code, password}` → sukses:
  sembunyikan semua form, tampilkan `#reset-success` ("Masuk dengan kata sandi baru" → `/login`).
- `#reset-resend` "Kirim ulang kode" + `#change-reset-email` "Gunakan email lain" (reset ke form 1).
- Countdown `startResendCountdown` + Retry-After seperti register.

### 3.5 `admin_login.html` → `/admin/login`

- `meta robots noindex, nofollow`.
- Eyebrow `ALGENTRA CAPITAL / PRIVATE ACCESS`, h1 "Admin Login".
- Form `#admin-login-form`: Username (text), Password → `POST /api/auth/login`
  `{username, password}` → `location.assign("/dashboard")`.

### 3.6 `account.html` → `/account` (portal klien)

`main.page-shell.dashboard-page`. Heading: eyebrow `PORTAL KLIEN / COPY TRADING`,
h1 "Akun Copy Trading", aside "Password broker tidak ditampilkan kembali setelah disimpan."

**Section 1 — Hubungkan akun** (`form#account-form.form-grid.account-connect-form`):
- `fieldset.plan-picker` "Cara menjalankan copy trading": 2 `.plan-option` radio —
  `MANAGED` "Dikelola server" (checked bila `managed_copy_enabled`, else disabled +
  badge `.plan-option__availability` "Belum tersedia" + `aria-disabled` + title
  "Layanan dikelola server sedang offline") dan `EA` "Pasang EA sendiri".
- `p#managed-copy-service-status.status` — teks status worker (diisi JS).
- `fieldset.plan-picker` "Pilih paket layanan": loop `client_plans` → `.plan-option` per plan:
  nama, `vCPU: …`, `RAM: …`, `Disk: …`, harga `Rp 1.500.000` (format id-ID).
  field-help: "Total mengikuti harga paket… Jangan transfer sebelum melihat instruksi pada pesanan Anda."
- Input: Nama akun (`label`, max 80), Server broker (`server`, max 128),
  Nomor akun broker (`login`, numeric, pattern `[0-9]+`, custom validity
  "Nomor akun MT5 hanya boleh berisi angka, tanpa spasi.").
- Submit "Lanjutkan ke pembayaran" → `POST /api/payments/orders`
  `{execution_mode, plan, label, server, login}` → `location.assign(/transactions?order={id})`.
- `p#account-message.status`, `#new-token[hidden]` (box token sekali-tampil),
  toolbar link "Buka riwayat transaksi dan nota" → `/transactions`.

**Section 2 — Saldo dan performa**: `#client-balance-groups.grid` (kartu per trade_mode+currency:
"Saldo Live · USD", saldo/ekuitas/floating, "N dari M akun melapor"),
`#client-performance-groups.grid` (realized PnL, "N transaksi ditutup · W menang · L kalah · winrate N%"),
`#client-performance-updated.muted`.

**Section 3 — Akun Copy Trading Anda**: `#accounts.grid` — kartu per akun:
- Mode EA: detail "Akun Copy Trading · {login} · {server} · {plan} (vcpu, RAM, Disk) · Rp …",
  `.account-balance` (Saldo/Ekuitas/Floating), `.account-balance-time`,
  state "Aktif · Terakhir terhubung: …", `.account-state.account-state--{connection_status}`
  (dot + label: ready/waiting_for_source/offline/trading_disabled/not_real_account/disabled),
  toolbar: **Edit akun** (prompt label/server/login → `PUT /api/mt5/accounts/{id}`),
  **Aktifkan/Nonaktifkan** (`PUT …/active` `{active}`), **Rotasi token**
  (`POST …/rotate-token` → `showToken`), **Hapus akun** icon-button danger
  (`DELETE /api/mt5/accounts/{id}`, confirm).
- Mode MANAGED: `addManagedAccountCard` — detail "Dikelola server · …",
  "Copy aktif/dijeda · Worker terakhir terhubung: …", `.account-login-check`
  (UNCHECKED/CHECKING/VALID/ERROR + `--valid/--error/--pending`),
  `.account-state` (SETUP_REQUIRED/PAUSED/STARTING/RUNNING/OFFLINE/ERROR/STOPPED),
  **form managed-copy**: password broker, follower_symbol, mode_lot (rasio/tetap),
  ratio_lot, lot_tetap, max_lot_per_order, max_lot_total, max_open_positions
  → `PUT /api/mt5/accounts/{id}/managed-copy`; tombol **Mulai copy trading (live)/Jeda**
  (`PUT …/managed-copy/active` `{active, acknowledge_live_risk}`, confirm bila mulai;
  disabled bila belum configured / worker offline / login belum VALID); **Hapus akun**.
- `showToken(title, token)`: `#new-token` → h3 + instruksi + `pre` token +
  tombol "Salin token" (clipboard → "Tersalin").

**Section 4 — `#ea-install-section`** (hidden; tampil bila ada akun EA-mode):
"Pasang EA Copy Trading" + link download `/downloads/MT5FollowerCopyEA.mq5` +
`ol` 5 langkah (salin token → Open Data Folder → MetaEditor F7 → Allow WebRequest +
tambah URL → pasang di chart + AccountToken/ServerURL + Algo Trading) +
`.notice-box` "Arti pesan di chart MT5" (HTTP -1/ERROR 4006, TRADING OFF) +
`.notice-box` hedging + VolumeMultiplier + sinkronisasi berhenti >20 dtk +
section-help "Jangan bagikan token…".

**CS chat floating**: `.cs-chat-widget.cs-chat-widget--account#cs-chat-widget`
(`data-cs-mode="client"`) — launcher icon bulat + panel (JS global base.html).

**Polling**: `loadAccounts()` = `GET /api/mt5/accounts` + `GET /api/mt5/performance` +
`GET /api/mt5/copier/status` (update radio MANAGED enable/disable + status line);
refresh tiap **5 dtk** kecuali `document.hidden` atau form managed sedang diedit
(`managedFormDirty`).

**Jinja vars:** `managed_copy_enabled` (bool), `client_plans` (dict → `tojson`).

### 3.7 `dashboard.html` → `/dashboard` (admin)

`main.page-shell.dashboard-page`. Memuat Chart.js via CDN jsdelivr.

1. `#registration-controls` — "Pendaftaran client", `#registration-state`,
   note launching 16 Jan 2027, tombol `#registration-open` / `#registration-close.secondary`,
   `#registration-message.status[hidden]`.
2. `section.summary-card[aria-label="Ringkasan operasional"]`:
   - `.sc-overview`: `.sc-summary-status` (chip `#ea-status/#telegram-status/#gemini-status`
     `.sc-chip.sc-dot` + `small` heartbeat/account/model hidden);
     `.sc-performance`: `.sc-profit` (TOTAL PROFIT `#total-profit`, `#wins-losses`),
     `.sc-metrics` (Winrate `#winrate`, Win/Lose `#wins-losses-inline`, Rugi hari ini `#daily-loss`).
   - `.sc-signals`: `.sc-label` "Sinyal Total N", `.sc-table` (Tereksekusi/Pending/Ditolak/Diabaikan/Diproses),
     `details.sc-help` "Arti istilah".
   - `#signal-status-warning.signal-status-warning[hidden]`.
3. `#mt5-live-section` — heading "Data MT5 live" + `#mt5-live-updated` + `#mt5-live-status.sc-dot`;
   `.summary-card.sc-mt5`: chips `#mt5-mode/#mt5-live-chip`, EQUITY `#mt5-equity`,
   `.sc-metrics-three` (Balance/Floating/Posisi terbuka); grid 2 section:
   tabel `#mt5-positions-table` (Ticket/Simbol/Arah/Lot/Entry/SL/TP),
   tabel `#mt5-quotes-table` (Simbol/Bid/Ask/Update).
4. "Signal terbaru" — tabel `#signals-table` (Waktu/Grup/Pesan/Parsing/Confidence/Status/Ticket/Profit), 5 baris.
5. `.grid` 2 kolom: "Win dan loss per grup" (`canvas#group-chart` bar Chart.js + `#groups-table`),
   "Profit kumulatif" (`canvas#profit-chart` line Chart.js).
6. "Log sistem" — toolbar (`#log-level` select, `#log-search`, `#filter-logs`,
   `a#logs-export` → `/api/logs/export.csv` diupdate via JS), `#logs-table` (Waktu/Level/Sumber/Pesan).
7. "Riwayat trade" — toolbar (date-from/to, `#trade-group`, `#trade-symbol`, `#trade-result`
   select, `#filter-trades`, `a#trades-export`), `#trades-table` 9 kolom,
   `.pager` (Sebelumnya/Halaman N/Berikutnya, 10/halaman).
8. `#settings` — eyebrow "Risk & configuration": form `#gemini-form.settings-block`
   (Model input + Simpan), form `#settings-form.settings-block` (confidence threshold,
   max market deviation %, entry mode select PARTIAL/SINGLE, symbol mapping textarea JSON,
   toggle allow_updates, toggle kill_switch, Simpan).
9. `p#connection.status` — "Menghubungkan realtime…".

**API (semua relatif, admin-only):**
`GET /api/stats`, `GET /api/mt5-live`, `GET /api/signals?limit=5`,
`GET /api/groups/stats`, `GET /api/logs?level&search&limit=5`, `/api/logs/export.csv`,
`GET /api/trades?date_from&date_to&group_id&symbol&result&limit&offset`, `/api/trades/export.csv`,
`GET /api/trades?limit=30` (grafik profit), `GET /api/settings`,
`PUT /api/settings` `{values:{registration_open|confidence_threshold|symbol_mapping|max_market_deviation_pct|entry_mode|allow_updates|kill_switch}}`,
`PUT /api/settings/gemini-config` `{model}`.
**WebSocket** `/ws` — pesan `{stats, latest_signal_id}`; fallback: bila WS gagal/8 dtk
→ polling `loadStatsAndSignals` tiap 5 dtk ("Realtime tidak tersedia; ringkasan diperbarui tiap 5 detik.");
MT5 live poll tiap 2 dtk; reconnect WS tiap 3 dtk.

### 3.8 `transactions.html` → `/transactions` (klien)

`main.page-shell.dashboard-page`. Heading: eyebrow `PORTAL KLIEN / PEMBAYARAN`,
h1 "Riwayat transaksi", link `.workspace-tag` "Kembali ke akun MT5" → `/account`.

- `.transaction-toolbar`: note "Nota hanya dapat dibuka oleh pemilik akun…",
  select `#transaction-filter` (ALL/PENDING_PAYMENT/PAYMENT_SUBMITTED/APPROVED/REJECTED/CANCELLED).
- `#transaction-message.status`, `#transactions-list.grid` (render fingerprint: skip bila tak berubah).
- Kartu `.card.transaction-card`: `.transaction-card__top` (h3 "{label} - Pesanan #{id}" +
  `.transaction-badge.transaction-badge--{status}`), `dl.transaction-details`
  (Nomor transaksi, Paket "{name} - {vcpu} - RAM {ram} - Disk {disk}", Total pembayaran
  "Rp …", Nama akun, Mode layanan, Login broker "{login} - {server}", Tanggal pesanan WIB).
- Aksi per status (`.toolbar`):
  - `has_proof` → "Lihat nota pembayaran" → `<dialog id="receipt-dialog" class="receipt-dialog">`
    (head "Nota pembayaran" + Tutup; body: h2, `.transaction-note` status,
    details, img `.receipt-proof` `src=/api/payments/orders/{id}/proof` atau `.receipt-empty`).
  - PENDING_PAYMENT → "Lanjutkan pembayaran"/"Tutup pembayaran" (expand):
    `.transaction-note` instruksi (`manualPaymentInstructions`) + "Saya sudah bayar" →
    `.transaction-proof-form` (file JPG/PNG/WebP ≤5MB) → `POST …/proof` (FormData).
  - PAYMENT_SUBMITTED → note progres; bila `!notification_sent` → "Kirim ulang notifikasi admin"
    (`POST …/notify`).
  - APPROVED → note + "Buka akun Copy Trading" → `/account`.
  - REJECTED → note "tidak valid".
  - PENDING_PAYMENT/PAYMENT_SUBMITTED → "Batalkan pesanan" (`POST …/cancel`).
- Deep link `?order=<id>` → order itu langsung expanded.
- Poll `GET /api/payments/orders` tiap **7 dtk** (skip bila hidden).
- **Jinja vars:** `client_plans`, `payment_manual_instructions` (keduanya `tojson`).

### 3.9 `parser_test.html` → `/parser-test` (admin)

- `main.page-shell.dashboard-page`, eyebrow "Signal lab", h1 "Tes Parser Signal".
- `section.parser-form-panel > form#parser-form.parser-form`:
  `.parser-field` label "Pesan baru" + textarea `#message` (placeholder contoh XAUUSD),
  `.parser-field--context` label "Konteks pesan sebelumnya (opsional)" + textarea `#context`,
  `.parser-actions` tombol "Parse pesan".
- `section.parser-result > .parser-result-heading` (h2 "Hasil parsing" + section-help) +
  `pre#result` "Masukkan pesan untuk menguji parser."
- Submit → `POST /api/parser/test` `{message, context|null}` header `X-CSRF-Token`
  → `pre` = JSON.stringify(payload, null, 2) atau error.

### 3.10 `telegram_setup.html` → `/telegram-setup` (admin)

- `main.page-shell.dashboard-page`, eyebrow "Signal sources", h1 "Telegram Setup",
  `#status.heading-aside` "Status: memeriksa...".
- Section "Hubungkan akun": label+input `#phone.phone-input` (+628…), tombol `#send-code`
  "Kirim kode Telegram"; `#code-step[hidden]` (input `#code` + `#verify-code`);
  `#password-step[hidden]` (input `#password` 2FA + `#verify-password`).
- Section tombol: `#reconnect.secondary`, `#telegram-logout.secondary`, `#refresh-groups.secondary`.
- `p#notice[role=status]`.
- Section "Grup dan channel": `#groups` ("Login untuk memuat daftar grup.") —
  baris `.row`: checkbox (`data-chat-id`, checked=enabled) + nama + input `.alias`
  (`data-alias-for`); tombol `#save-groups` "Simpan whitelist" (→ "Menyimpan…").
- API: `GET /api/telegram/status` → `{connected, account_name, state, configured}`;
  `GET /api/telegram/groups` → `[{chat_id, name, enabled, alias}]`;
  `POST /api/telegram/send-code` `{phone}`; `POST /api/telegram/verify-code` `{code}`
  → `{requires_2fa}`; `POST /api/telegram/verify-2fa` `{password}`;
  `POST /api/telegram/reconnect`; `POST /api/telegram/logout`;
  `POST /api/telegram/groups/select` `{groups:[{chat_id, enabled, alias}]}`.

### 3.11 `404.html` → halaman not-found

- `main.page-shell.dashboard-page.not-found-shell > section.not-found-card`:
  `.not-found-copy`: eyebrow "ALGENTRA CAPITAL / 404", `.not-found-code` pill
  "HALAMAN TIDAK DITEMUKAN", h1 "Sinyalnya nyasar?",
  p "Chart-nya jalan, halamannya nggak ketemu. Tenang, ini cuma floating loss satu halaman—yuk balik ke area aman.",
  `code.not-found-path` = `{{ requested_path }}`,
  `.not-found-actions`: link "Kembali ke beranda" (`/`, gradien hijau) + link `.secondary` "Masuk" (`/login`).
- `.not-found-art`: SVG 420×330 trader + chart animasi (7 keyframes, hormati reduced-motion).
- Catatan: di arsitektur lama 404 dirender server oleh Core API; di Next.js gunakan
  `not-found.tsx` (atau catch-all) dengan markup/CSS yang sama. `requested_path` →
  tampilkan `window.location.pathname` (client).

### 3.12 `_cs_mascot.html`

- Macro Jinja `algentra_mascot(class_name)` → SVG robot 48×48 (biru `#4da3e8`,
  emas `#f2c438`, navy `#16466e`). Di Next.js: jadikan komponen React
  (`Mascot.tsx`) menerima `className`.

### 3.13 `performance` (tidak ada template; hanya route `/performance` di public.py lama)

Di arsitektur lama tidak ada template khusus — section `#performance` adalah bagian dari
`public.html`. Di Next.js saat ini ada `/performance` + `PerformancePanel` terpisah;
faithful = section `#performance` tetap di landing `/`, dan `/performance` boleh
menampilkan ulang panel yang sama (bukan desain baru).

---

## 4. MAPPING TEMPLATE → NEXT.JS

### 4.1 File tujuan

| Template | Tujuan Next.js | Catatan |
|---|---|---|
| `base.html` | `app/layout.tsx` + `app/globals.css` + `app/components/SiteFrame.tsx` (+ komponen baru: `ThemeToggle`, `MusicToggle`, `CsChatWidget`, `Topbar`) | Layout tidak bisa 1 file: pecah jadi layout (head/meta), CSS global, dan komponen topbar |
| `public.html` | `app/page.tsx` (+ `PublicPage` component bila besar) | Section `#performance` tetap di `/` |
| `login.html` | `app/login/page.tsx` + `AuthForm` (mode client) | |
| `register.html` | `app/register/page.tsx` + `AuthForm` (mode register) | |
| `forgot_password.html` | `app/forgot-password/page.tsx` | |
| `admin_login.html` | `app/admin/login/page.tsx` + `AuthForm` (mode admin) | |
| `account.html` | `app/account/page.tsx` + `AccountPanel.tsx` | |
| `dashboard.html` | `app/dashboard/page.tsx` + `DashboardPanel.tsx` | Butuh Chart.js (CDN) untuk 2 grafik |
| `transactions.html` | `app/transactions/page.tsx` + `TransactionsPanel.tsx` | Pakai native `<dialog>` |
| `parser_test.html` | `app/parser-test/page.tsx` + `ParserPanel.tsx` | |
| `telegram_setup.html` | `app/telegram-setup/page.tsx` + `TelegramPanel.tsx` | |
| `404.html` | `app/not-found.tsx` | `requested_path` → `usePathname()` |
| `_cs_mascot.html` | `app/components/Mascot.tsx` | |

### 4.2 Jinja server-vars → client fetch (BFF)

| Variabel Jinja | Dipakai di | Pengganti client-side |
|---|---|---|
| `request.session.admin_username` / `client_user_id` | nav, footer, brand | Endpoint status sesi — **BELUM ADA**: butuh `GET /api/core/auth/session` (atau `/v1/auth/session`) yang mengembalikan `{admin: bool, client: bool}`. Alternatif sementara: coba `GET dashboard/stats` → 401 = anonim. |
| `request.cookies.theme` | `<html data-theme>` | Baca `document.cookie` + localStorage di client (sudah ada di JS base) |
| `csrf_token` (meta) | semua POST | Ambil dari `GET /api/core/auth/csrf` atau response header; simpan ke sessionStorage seperti sekarang (`algentra-csrf`). Pastikan endpoint CSRF ada. |
| `registration_open`, `registration_launch_pending`, `registration_launch_at` | public.html, nav | `GET /api/core/public/registration-status` → `{registration_open, registration_launch_pending, registration_launch_at}` (endpoint sudah ada) |
| `contact_person`, `contact_whatsapp`, `contact_email`, `contact_socials` | public.html kontak | `GET /api/core/public/contact` — **BELUM ADA**: butuh endpoint baru `{person, whatsapp, email, socials:[{url,icon,label,display}]}` |
| `managed_copy_enabled` | account.html radio | `GET /api/core/mt5/copier/status` → `{available, configured}` (sudah dipakai; ganti default awal ke loading, bukan Jinja) |
| `client_plans` | account.html, transactions.html | `GET /api/core/mt5/plans` → `{code: {name, vcpu, ram, disk, price_idr}}` (sudah ada dari porting sebelumnya) |
| `payment_manual_instructions` | transactions.html | `GET /api/core/payments/manual-instructions` → `{instructions}` (sudah ada) |
| `requested_path` | 404.html | `usePathname()` di `not-found.tsx` |

### 4.3 Mapping endpoint lama → BFF baru

BFF: `fetch("/api/core/<p>")` → `CORE_API_ORIGIN/v1/<p>` (kecuali `downloads/*` → tanpa `/v1`).

| Template lama (`/api/...`) | BFF baru (`apiRequest("...")`) | Core (`/v1/...`) |
|---|---|---|
| `/api/auth/login`, `/client-login`, `/register`, `/verify-email`, `/resend-code`, `/password-reset/request`, `/password-reset/confirm`, `/auth/logout` | `auth/...` (sama) | `/v1/auth/...` (router sudah dual-prefix `/api`+`/v1`) |
| `/api/cs/chat/history`, `/api/cs/chat/message` | `cs/chat/...` | `/v1/cs/chat/...` |
| `/api/public/performance`, `/api/public/registration-status` | `public/performance`, `public/registration-status` | `/v1/public/...` (router eksplisit dua prefix) |
| `/api/stats` | `dashboard/stats` | `/v1/dashboard/stats` |
| `/api/mt5-live` | `dashboard/mt5-live` | `/v1/dashboard/mt5-live` |
| `/api/signals` | `dashboard/signals` | `/v1/dashboard/signals` |
| `/api/groups/stats` | `dashboard/groups/stats` | `/v1/dashboard/groups/stats` |
| `/api/logs`, `/api/logs/export.csv` | `dashboard/logs`, `dashboard/logs/export.csv` | `/v1/dashboard/logs`, `/v1/dashboard/logs/export.csv` |
| `/api/trades`, `/api/trades/export.csv` | `dashboard/trades`, `dashboard/trades/export.csv` | `/v1/dashboard/trades`, `/v1/dashboard/trades/export.csv` |
| `/api/settings`, `/api/settings/gemini-config` | `settings`, `settings/gemini-config` | `/v1/settings`, `/v1/settings/gemini-config` |
| `/api/parser/test` | `parser/test` | `/v1/parser/test` |
| `/api/telegram/status|groups|send-code|verify-code|verify-2fa|reconnect|logout|groups/select` | `telegram/...` (sama) | `/v1/telegram/...` |
| `/api/mt5/accounts`, `…/performance`, `…/copier/status`, `…/{id}`, `…/{id}/active`, `…/{id}/rotate-token`, `…/{id}/managed-copy`, `…/{id}/managed-copy/active` | `mt5/...` (sama) | `/v1/mt5/...` |
| `/api/payments/orders`, `…/{id}/proof`, `…/{id}/notify`, `…/{id}/cancel` | `payments/...` (sama) | `/v1/payments/...` |
| `/downloads/MT5FollowerCopyEA.mq5` | `downloads/MT5FollowerCopyEA.mq5` (link langsung, BFF tanpa `/v1`) | `/downloads/...` (non-versioned) |
| `/musik.mp3` | **tidak lewat BFF** — file statis; di Next.js taruh di `public/musik.mp3` | `/musik.mp3` (Core API, non-versioned) |
| WebSocket `/ws` | **TIDAK BISA via Vercel serverless** — ganti polling (lihat §5.6) | `/ws` tetap ada di Core API |

---

## 5. PERBEDAAN KUNCI: TEMPLATE ASLI vs PORTING REACT SAAT INI

### 5.1 Desain (paling mencolok — ini keluhan utama user)
- **Aksen**: asli emas `#f2c438` + biru `#4da3e8` di atas dark-blue `#0d1a24`;
  porting: lime `#b9ea57` di atas near-black `#090c0e`. → Ganti total `globals.css`.
- **Font**: asli Poppins (heading) + Plus Jakarta Sans (body); porting: Inter semua. →
  Tambah Google Fonts di `layout.tsx`.
- **Glassmorphism**: asli `backdrop-filter: blur()` + `glass-border` di section/card/topbar;
  porting: flat solid. → Port CSS verbatim.
- **Nama kelas**: porting memakai `.shell/.panel/.section/.button/.muted/.error/.success`
  — tidak ada di desain asli. Asli: `.page-shell/.card/.toolbar/.status/.notice-box` dll.

### 5.2 Layout global (`SiteFrame` vs `base.html`)
- SiteFrame sekarang: topbar minimal (teks brand + 4 link statis), tanpa theme toggle,
  tanpa music toggle, tanpa CS chat, tanpa varian nav admin/klien/publik, footer beda teks.
- Yang harus ada: topbar glass + brand SVG + nav 4-varian (butuh endpoint sesi, §4.2) +
  `#theme-toggle` + `#nav-toggle` (mobile) + tombol Logout/Keluar + `workspace-tag` +
  footer 2-varian + `skip-link` + music toggle + CS chat widget global.

### 5.3 Landing (`/` vs `public.html`)
- Porting sekarang = placeholder generik (hero teks + 3 kartu + note QRIS).
- Asli = hero dengan **signal-orbit visual**, **registration countdown** (16 Jan 2027),
  **community card** (registered count live), **performance section** (grafik equity SVG
  custom + watchlist 4 simbol dengan tick animation), **approach** 3 kartu,
  **kontak** (WhatsApp/email/socials/CS chat launcher), **closing CTA**.
- Hampir seluruhnya belum di-port. Ini pekerjaan terbesar.

### 5.4 Dashboard (`DashboardPanel` vs `dashboard.html`)
- Ringkasan: asli `.summary-card` 2-kolom (overview chips + tabel sinyal + details "Arti istilah");
  porting: 8 StatCard grid. → Samakan layout.
- Grafik: asli Chart.js (`#group-chart` bar, `#profit-chart` line); porting: **tidak ada**.
- MT5 live: struktur mirip, tapi class/posisi beda (asli `.sc-mt5` + chips).
- Realtime: asli WebSocket `/ws` + fallback polling; porting: polling 5 dtk saja (wajar untuk Vercel).
- Log/trade/settings: fitur lengkap di porting, tapi class & copy beda; export CSV asli
  berupa `<a href>` yang diupdate JS (bukan fetch).
- Registration controls: ada di keduanya; porting pakai `disabled` bukan render ulang state.

### 5.5 Account (`AccountPanel` vs `account.html`)
- Fitur: sebagian besar sudah ada (form connect, plan picker, kartu akun EA/MANAGED,
  token box, EA guide). Yang hilang/beda:
  - Plan picker asli = **radio cards** (`.plan-option`, 3 kolom, badge "Belum tersedia");
    porting perlu dicek visualnya.
  - Status worker: asli `p#managed-copy-service-status.status` dengan 3 varian teks.
  - `.account-state` dot + label koneksi (6 status) — porting menyederhanakan.
  - `.account-login-check` 4 status (UNCHECKED/CHECKING/VALID/ERROR).
  - Form managed-copy: field dan batas sama; asli pakai `confirm()` untuk mulai live.
  - EA install: asli 5 langkah `ol` + 2 `.notice-box` detail (arti HTTP -1/4006, hedging,
    VolumeMultiplier, sinkronisasi >20 dtk); porting menyingkat.
  - Polling 5 dtk + `managedFormDirty` guard — porting: cek apakah ada.
  - CS chat floating account — porting: belum ada.

### 5.6 Transactions (`TransactionsPanel` vs `transactions.html`)
- Fitur inti ada (filter, kartu, aksi per status, upload bukti, retry notify, cancel, `?order=`).
- Beda: asli pakai **native `<dialog>`** (`.receipt-dialog`) — porting pakai `<section role="dialog">`.
- Asli ada **render fingerprint** (skip render bila data tak berubah) + poll 7 dtk.
- Kelas: `.transaction-card__top`, `.transaction-badge--*`, `dl.transaction-details`,
  `.transaction-note` — porting memakai kelas generik.
- Detail paket: asli "{name} - {vcpu} - RAM {ram} - Disk {disk}"; mode layanan ada 2 varian teks.

### 5.7 Auth (login/register/forgot/admin)
- Client login: asli ada **checkbox remember-me** ("Biarkan saya tetap masuk selama 30 hari",
  kirim `remember_me: true/false`); porting **hardcode `false`** → perbaiki.
- Register: asli 2-form + tombol **"Kirim ulang kode"** + countdown `startResendCountdown` +
  Retry-After; porting: cek kelengkapan (tidak ada resend di AuthForm yang saya baca).
- Forgot password: asli 2-form + "Kirim ulang kode" + **"Gunakan email lain"** + success state
  dengan link login; porting (`forgot-password/page.tsx`) versi sederhana — samakan.
- Admin login: mirip; asli ada `meta robots noindex`.
- Class: asli `page-shell dashboard-page login-shell` + `section.login-card` + eyebrow per mode
  ("PORTAL KLIEN / MASUK", "REGISTRASI KLIEN / VERIFIKASI EMAIL",
  "PORTAL KLIEN / RESET KATA SANDI", "ALGENTRA CAPITAL / PRIVATE ACCESS").

### 5.8 Telegram & Parser
- `TelegramPanel` saat ini: layout 2-kolom custom (`.telegram-*`), teks Inggris-campuran;
  asli: `.page-shell.dashboard-page`, heading + status di `.heading-aside`,
  section login (phone → code → 2FA), section tombol (Reconnect/Logout/Muat grup),
  `#notice`, section grup `.row` + "Simpan whitelist" ("Menyimpan…" saat busy).
- `ParserPanel` saat ini: versi mini ("Uji klasifikasi", tanpa field konteks);
  asli: `.parser-form` 2 field (Pesan + Konteks opsional), `.parser-result` dengan heading
  + `pre` monospace.

### 5.9 Halaman yang belum ada padanannya
- `not-found.tsx` — belum ada (asli: 404.html artistik). Next.js: buat `app/not-found.tsx`.
- CS chat widget global — belum ada di porting (asli: JS di base.html + markup di public/account).
- Music toggle — belum ada.
- Theme toggle — belum ada (asli: dark default, light tersedia).

### 5.10 Endpoint yang perlu dibuat di Core API (agar faithful penuh)
1. `GET /v1/auth/session` → `{admin: bool, client: bool}` (untuk nav 4-varian). **Baru.**
2. `GET /v1/public/contact` → `{person, whatsapp, email, socials}` (untuk section kontak). **Baru.**
3. Pastikan `GET /v1/auth/csrf` (atau setara) ada untuk `X-CSRF-Token` awal.
4. `/musik.mp3` → pindah ke `apps/web/public/musik.mp3` (atau tetap proxy; file statis Next.js lebih tepat).

---

## 6. URUTAN KERJA YANG DISARANKAN

1. `globals.css` — port seluruh CSS base.html (+ public/404/transactions `<style>` per halaman
   bisa jadi CSS module atau satu file).
2. `layout.tsx` — font Google (Poppins + Plus Jakarta Sans), `data-theme`, meta csrf/theme-color.
3. `SiteFrame` → Topbar 4-varian + ThemeToggle + MusicToggle + CsChatWidget + footer 2-varian.
4. `AuthForm` — samakan 4 mode persis (remember-me, resend countdown, gunakan-email-lain).
5. `app/page.tsx` — landing penuh (hero orbit, countdown, community, performance+SVG chart,
   watchlist, approach, kontak, closing).
6. `AccountPanel`, `TransactionsPanel`, `DashboardPanel` — samakan markup/kelas per §3.6–3.8
   (tambah Chart.js untuk dashboard).
7. `TelegramPanel`, `ParserPanel` — samakan.
8. `not-found.tsx` + `Mascot.tsx`.
9. Endpoint baru §5.10 di Core API.
