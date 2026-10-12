# instruksi.md — Refactor Algentra Capital

Dokumen ini adalah instruksi kerja untuk merefactor `agusmaulana-xyz/algentraCapitalProject` menjadi proyek yang **terstruktur rapi, cepat, hemat bandwidth (terutama database), realtime, ringan, stabil, dan keamanan tinggi**.

Target deployment:

| Komponen | Platform |
|---|---|
| Web (UI publik, portal klien, dashboard admin) | **Vercel** |
| Backend inti, Telegram/Gemini, worker copy trading, terminal MT5 | **Windows Server** |
| Database | **Supabase (PostgreSQL terkelola)** |

> **Catatan dasar dokumen.** Analisis ini disusun dari `README.md` repo (backend FastAPI + SQLite, EA MT5, worker `copyTrading/`, Cloudflare Tunnel, WebSocket `/ws`, chat Gemini, pembayaran QRIS + bot Telegram). Isi folder `backend/`, `copyTrading/`, `mt5/` belum diaudit baris per baris. **Fase 0 wajib memverifikasi asumsi di sini terhadap kode asli** sebelum mengubah apa pun.

> **Riwayat perubahan dokumen.** Revisi ini mengganti database dari *Oracle NoSQL Cloud (Always Free)* ke **Supabase**. Dampak utama: desain data berubah dari single-table NoSQL (3 tabel) menjadi **skema relasional Postgres**; TTL bawaan diganti `pg_cron` + partisi; pencatat RU/WU diganti pemantau ukuran DB, egress, dan koneksi; migrasi SQLite → Postgres jauh lebih sederhana. Prinsip arsitektur lain (hot path di memori, outbox, satu pemilik data, keamanan) **tidak berubah**.

---

## 0. Cara memakai dokumen ini

1. Kerjakan **berurutan per fase** (bagian 14). Satu fase = satu branch/PR.
2. Jangan lanjut ke fase berikutnya sebelum **Definition of Done (DoD)** fase itu terpenuhi.
3. Fase 1–2 **tidak boleh mengubah perilaku** (hanya memindahkan dan merapikan kode). Perubahan perilaku baru dimulai dari fase 3.
4. Semua fitur yang menyentuh **order live** wajib punya test sebelum dipindahkan.
5. Jika menemukan kondisi kode yang berbeda dari asumsi di sini, **perbarui dokumen ini** (bagian "Catatan audit" di akhir), jangan diam-diam menyimpang.

## 1. Prinsip desain (berlaku untuk semua keputusan)

1. **Satu pemilik data.** Hanya `core-api` yang berbicara dengan Supabase/Postgres. Vercel, EA, worker, dan browser tidak pernah memegang kredensial database (termasuk `service_role` key dan connection string).
2. **Hot path tanpa database.** Jalur eksekusi sinyal, snapshot posisi master, heartbeat, dan push realtime dilayani dari **memori**. Database hanya untuk state yang harus tahan lama.
3. **Tulis seperlunya, baca dari cache.** Tulis ke DB hanya saat ada perubahan bermakna, secara batch, lewat outbox. Baca dari cache in-memory dengan invalidasi eksplisit.
4. **Fail-safe.** Jika Supabase, Telegram, Gemini, atau SMTP mati, posisi live yang sudah ada tetap dikelola dan kill switch tetap bekerja.
5. **Idempoten di mana-mana.** Setiap sinyal, order, laporan EA, dan job worker punya idempotency key. Retry tidak boleh menggandakan order (di DB: `INSERT ... ON CONFLICT`).
6. **Least privilege & zero secret di repo.**
7. **Ukur dulu.** Pantau ukuran DB, egress, koneksi, dan beban compute; ada alarm sebelum batas plan tercapai.
8. **Sederhana dulu.** Satu proses `core-api` dengan satu worker uvicorn cukup; jangan menambah broker pesan/Redis kecuali metrik membuktikan perlu.
9. **Pakai Supabase sebagai Postgres, bukan sebagai backend.** Auth, sesi, otorisasi, dan realtime tetap dikelola `core-api` (alasan di bagian 5.2). Fitur Supabase lain dipakai hanya bila ada nilai jelas (Storage privat, `pg_cron`, backup).

## 2. Temuan dari README dan solusinya

| # | Temuan di kondisi sekarang | Risiko | Arah solusi |
|---|---|---|---|
| 1 | Monolit: UI (halaman server-side + Chart.js CDN), API, Telegram, Gemini, SMTP, pembayaran dalam satu proses FastAPI | Satu bug menjatuhkan semuanya; UI membebani server trading | Pisahkan: UI → Vercel; `core-api`, `signal-service`, `copier-worker` → Windows service terpisah |
| 2 | Database SQLite lokal, backup via `run-backup.bat` | Single point of failure di satu disk | Supabase Postgres sebagai sumber kebenaran + outbox lokal + `pg_dump` terenkripsi terjadwal |
| 3 | EA master mengirim snapshot posisi **tiap detik**; follower polling posisi | Jika tiap snapshot ditulis/dibaca dari DB → membakar egress, IO, dan koneksi | Snapshot hanya di memori; kirim hanya saat berubah (ETag/revision); DB hanya mencatat event buka/tutup |
| 4 | `EA_API_KEY` tunggal & statis dipakai bersama | Satu kebocoran = semua terminal terbuka | Kredensial per-perangkat, disimpan sebagai hash, request bertanda tangan HMAC + timestamp + nonce |
| 5 | Password broker klien (mode managed) disimpan terenkripsi dengan key turunan `APP_SECRET_KEY` | **Risiko tertinggi.** Satu secret bocor = semua password broker terbuka; mengganti secret merusak semua data | Envelope encryption dengan key terpisah (bagian 10.3); minimalkan/hindari penyimpanan password trading |
| 6 | Chat AI bisa menyisipkan token EA ke jawaban | Prompt injection / pencurian token lewat chat | **Hapus fitur pengungkapan token via chat.** Token hanya ditampilkan sekali di portal setelah re-auth |
| 7 | Pesan Telegram (input tak tepercaya) diparse Gemini lalu dieksekusi live | Prompt injection / sinyal palsu | LLM hanya mengekstrak; validasi deterministik (whitelist simbol, batas lot/risiko, jarak SL/TP) sebelum order |
| 8 | Repo **publik** berisi `BLACKLIST GRUP.xlsx`, `CUSTOMER_SERVICE_KNOWLEDGE_BASE.md`, `musik.mp3`, `qris.jpeg`, skrip `.bat` | Kebocoran data bisnis/operasional | Jadikan repo privat, audit riwayat git, pindahkan data ke DB/storage |
| 9 | Dashboard mengirim pembaruan berkala lewat `/ws`; polling di beberapa tempat | Beban berulang dan boros | Hub realtime dengan fan-out dari memori, throttle, delta |
| 10 | Pembersih chat tiap 15 menit, OTP, sesi dikelola manual | Beban tulis/baca tambahan | Kolom `expires_at` + job `pg_cron` per jam + **partisi bulanan** untuk tabel besar (drop partisi = hampir tanpa biaya) |
| 11 | Satu worker uvicorn, Cloudflare Tunnel ke `127.0.0.1:8000` | Baik untuk keamanan; kurang untuk observabilitas | Pertahankan pola tunnel; tambah health/ready, metrik, alert |
| 12 | "Belum teruji": Gemini, Telegram, SMTP, EA live | Regresi tidak terdeteksi | Contract test + simulator EA + staging akun demo |

## 3. Arsitektur target

```
                         ┌───────────────────────────────────────────┐
 Browser ───HTTPS───────►│ Vercel: Next.js (UI + BFF route handlers) │
    │                    │  - halaman publik ISR/cache               │
    │                    │  - sesi cookie HttpOnly                   │
    │                    └───────────────┬───────────────────────────┘
    │                                    │ server→server (HMAC + CF Access service token)
    │ WSS (tiket JWT 60 dtk)             ▼
    │                    ┌───────────────────────────────────────────┐
    └───────────────────►│ Cloudflare (WAF, rate limit, Access, TLS) │◄── EA MT5 (HTTPS, HMAC)
                         └───────────────┬───────────────────────────┘
                                         │ Cloudflare Tunnel (tanpa port inbound)
                         ┌───────────────▼───────────────────────────┐
                         │ WINDOWS SERVER                            │
                         │  core-api  (FastAPI, 1 worker)            │
                         │   ├ REST v1  ├ Realtime hub (WS/SSE)      │
                         │   ├ state store in-memory + cache         │
                         │   └ outbox (SQLite WAL) ─► flusher ──────────► Supabase Postgres
                         │  signal-service (Telethon + Gemini)       │     (Supavisor pooler, TLS,
                         │  copier-worker  (MetaTrader5 + terminal)  │      outbound saja)
                         │  MT5 master terminal + EA                 │
                         └───────────────────────────────────────────┘
```

**Tiga hostname publik** (pisahkan agar aturan WAF/rate-limit berbeda):

| Hostname | Dipakai oleh | Catatan |
|---|---|---|
| `app.<domain>` | Browser → Vercel | UI + BFF |
| `ea.<domain>` | EA MT5 | Hanya path `/v1/ea/*` dan `/v1/follower/*`; tidak lewat Vercel |
| `rt.<domain>` | Browser (WebSocket) | Vercel tidak bisa menahan koneksi WebSocket; hub berada di Windows Server |
| `admin` (di bawah `app`) | Admin | Dilindungi **Cloudflare Access** + login admin + TOTP |

Keputusan arsitektur:
- **Vercel tidak menyimpan state dan tidak memegang kredensial Supabase.** Ia hanya memanggil `core-api`. **Jangan** memasang integrasi Vercel–Supabase bawaan (ia menyuntikkan URL/key database ke environment Vercel dan melanggar prinsip 1).
- **Region:** buat project Supabase di **Southeast Asia (Singapore)**, region terdekat dari Indonesia. Pilih region fungsi Vercel yang sama (Singapore/`sin1`) dan Windows Server sedapat mungkin di Asia Tenggara. Region project **tidak bisa diubah** setelah dibuat (pindah = migrasi), jadi putuskan sebelum Fase 3.
- DB tetap tidak boleh ada di jalur panas: latensi ke Postgres terkelola bukan nol, dan setiap query memakai koneksi, CPU, dan IO compute yang terbatas.
- `signal-service` dan `core-api` berkomunikasi lewat loopback (`127.0.0.1`) dengan kunci internal; keduanya boleh digabung dulu di fase awal bila RAM terbatas, tetapi kode harus tetap dipisah per modul.
- Terminal MT5 butuh sesi desktop interaktif (tidak bisa berjalan sebagai Windows Service murni karena isolasi Session 0). Jalankan terminal lewat akun non-admin dengan auto-logon terbatas atau Task Scheduler "run at logon", dan biarkan worker memantau prosesnya.

## 4. Struktur repo target (monorepo)

```
algentra/
├─ apps/
│  ├─ web/                      # Next.js (App Router) + TypeScript → Vercel
│  │  ├─ app/                   # (public)/ (client)/ (admin)/ api/ (BFF)
│  │  ├─ components/  lib/  styles/
│  │  └─ vercel.json            # regions, headers, cache
│  ├─ core-api/                 # FastAPI → Windows Service
│  │  ├─ app/
│  │  │  ├─ main.py             # app factory, lifespan
│  │  │  ├─ core/               # config, security, logging, errors, rate_limit, crypto
│  │  │  ├─ domain/             # entitas & aturan murni: risk, signal, symbol, lot
│  │  │  ├─ services/           # use case: auth, payments, signals, copy, chat, equity
│  │  │  ├─ repositories/       # interface + postgres/ + memory/ (untuk test)
│  │  │  ├─ api/v1/             # auth, public, client, admin, ea, follower, copier, ws
│  │  │  ├─ realtime/           # hub, channels, throttling
│  │  │  ├─ state/              # in-memory store (posisi master, heartbeat, settings cache)
│  │  │  ├─ outbox/             # antrean tulis + flusher batch
│  │  │  └─ jobs/               # ekspor/backup, rekonsiliasi, housekeeping
│  │  └─ tests/                 # unit/ integration/ contract/
│  ├─ signal-service/           # Telethon + filter lokal + Gemini parser
│  └─ copier-worker/            # managed copy (MetaTrader5)
├─ mt5/
│  ├─ experts/                  # TelegramSignalEA.mq5, MT5FollowerCopyEA.mq5
│  ├─ include/                  # klien HTTP, HMAC, JSON, retry/backoff bersama
│  └─ build.ps1                 # compile via MetaEditor CLI + injeksi versi
├─ packages/
│  └─ contracts/                # OpenAPI/JSON Schema → generate klien TS & model pydantic
├─ infra/
│  ├─ windows/                  # install-services.ps1 (NSSM), task XML, hardening.ps1
│  ├─ cloudflare/               # config tunnel (tanpa token), catatan WAF/Access
│  └─ supabase/                 # config.toml, migrations/*.sql, seed, policies, cron, README
├─ scripts/                     # migrate_sqlite_to_postgres.py, backup_db.ps1, seed.py
├─ docs/                        # arsitektur, runbook, ADR/
├─ .github/workflows/           # ci.yml (lint+test+secret-scan), deploy-web.yml
├─ .env.example                 # hanya nama variabel, tanpa nilai
├─ .gitignore  .editorconfig  README.md  instruksi.md
```

Aturan perpindahan file:

| Sekarang | Tujuan |
|---|---|
| `backend/` | `apps/core-api/` (pecah per lapisan di atas) |
| `copyTrading/` | `apps/copier-worker/` |
| `mt5/` | `mt5/experts/` + `mt5/include/` |
| `run.bat`, `run-service.bat`, `run-backup.bat` | `infra/windows/` (diganti skrip PowerShell + NSSM) |
| `BLACKLIST GRUP.xlsx` | Impor ke tabel `group_blacklist` di DB, **hapus dari repo & riwayat** |
| `CUSTOMER_SERVICE_KNOWLEDGE_BASE.md` | `apps/core-api/app/services/chat/kb/` (atau DB), dipaketkan sebagai data statis |
| `musik.mp3` | Keluar dari repo; layani via CDN/storage, `preload="none"`, tidak autoplay |
| `backend/app/static/qris.jpeg` | Aset web (`apps/web/public/`), dikompres |
| Template/halaman statis FastAPI | Diganti komponen Next.js |

Konvensi: Python → `ruff` + `mypy` (strict untuk `domain/`), TypeScript → `eslint` + `tsc --strict`, commit gaya Conventional Commits. Tidak ada logika bisnis di router; router hanya validasi input → panggil service → bentuk respons. **Skema DB hanya berubah lewat migrasi berurutan yang diuji pada project Supabase development**; production tidak menjalankan migrasi saat aplikasi start atau deploy. Project production harus sudah diprovisikan dengan revisi schema yang cocok sebelum cutover.

## 5. Batasan & fitur Supabase (rancang di sekitarnya)

### 5.1 Batas plan (verifikasi ulang di halaman pricing sebelum final, karena kuota bisa berubah)

Per pengecekan terakhir (Agustus 2026):

| Aspek | Free | Pro ($25/org/bulan + usage) |
|---|---|---|
| Ukuran database | 500 MB per project (read-only bila terlampaui) | 8 GB termasuk, lalu ±$0,125/GB |
| Egress | 5 GB/bulan (+ cached egress terpisah) | 250 GB termasuk (database egress), lalu ±$0,09/GB |
| Backup otomatis | **Tidak ada** | Harian, disimpan 7 hari |
| Auto-pause | **Ya, setelah 1 minggu tanpa aktivitas** | Tidak pernah |
| Compute | Shared, RAM terbatas | Credit $10 menutup 1 instance Micro (2-core ARM, 1 GB RAM); compute ditagih per project |

Implikasi:
1. **Produksi wajib Plan Pro.** Free tidak cocok: bisa *pause* (layanan trading tidak boleh bergantung pada project yang bisa tidur), tanpa backup otomatis, dan 500 MB mudah penuh. Free hanya untuk dev/staging, dan staging pun harus "dibangunkan" sebelum dipakai.
2. **Egress dan ukuran DB adalah meter biaya utama**, bukan jumlah query. Karena itu aturan hemat bandwidth (bagian 7) fokus pada: jangan `SELECT *`, jangan tarik daftar penuh, paginasi kursor, dan simpan telemetri dalam bentuk teragregasi.
3. **Compute kecil (Micro) punya batas koneksi dan IO.** Gunakan pooler, batasi ukuran pool, dan jangan membuka banyak koneksi dari banyak proses.
4. Bayangkan biaya sebagai anggaran: rencanakan ≈ $25–35/bulan (Pro + 1 project) dan pasang alert penggunaan di dashboard Supabase.

### 5.2 Fitur Supabase: dipakai atau tidak

| Fitur | Keputusan | Alasan |
|---|---|---|
| **Postgres** | Dipakai | Inti. Skema relasional, transaksi, `ON CONFLICT`, `jsonb`, partisi |
| **Supavisor (pooler)** | Dipakai | Lihat 5.3 |
| **`pg_cron`** | Dipakai | Pembersihan data kedaluwarsa & pembuatan partisi, berjalan di dalam DB (tanpa round trip dari server) |
| **Storage (bucket privat)** | Dipakai bila bukti bayar perlu disimpan | URL bertanda tangan berumur pendek; jangan simpan blob di tabel |
| **Supabase Auth** | **Tidak dipakai** (default) | Sistem sudah punya OTP, kebijakan password, sesi, admin+TOTP; memindah auth = perubahan perilaku besar dan menambah titik kebocoran. Boleh dievaluasi ulang setelah fase 7 |
| **Realtime (Postgres Changes/Broadcast)** | **Tidak dipakai** untuk jalur panas | Data realtime berasal dari memori `core-api`; lewat DB berarti menambah tulis, latensi, dan batas pesan/koneksi plan |
| **Data API (PostgREST)** | Dinonaktifkan/dikunci | Semua akses lewat `core-api`; lihat 10.4 |
| **Edge Functions** | Tidak dipakai | Logika berada di `core-api` |
| **Vault / pgsodium** | Tidak untuk KEK | KEK harus terpisah dari DB (10.3) |

### 5.3 Koneksi dari Windows Server

- `core-api` adalah **satu proses berumur panjang** → cukup **satu pool kecil** (mis. 5–10 koneksi total untuk semua service; hitung dari batas koneksi compute yang dipakai).
- Gunakan **Supavisor session mode** (port 5432) atau koneksi langsung. Koneksi langsung memakai IPv6; bila Windows Server hanya punya IPv4, pakai pooler (atau add-on IPv4 berbayar). **Hindari transaction mode (port 6543) kecuali perlu**; bila dipakai, matikan prepared statement cache pada driver.
- Driver: `asyncpg` atau `psycopg` 3 (pilih satu lewat ADR). SQL eksplisit di `repositories/postgres/`; boleh SQLAlchemy Core, **hindari ORM berat** agar kontrol query/kolom tetap ketat.
- Wajib TLS (`sslmode=verify-full` dengan CA Supabase) dan `statement_timeout` (mis. 5 detik) + `idle_in_transaction_session_timeout` di role aplikasi.
- Satu pool per service, semua opsional dengan *timeout* akuisisi pendek; kegagalan memicu circuit breaker (bagian 7).

## 6. Desain data (Postgres)

Skema relasional di schema `app` (bukan `public`), semua lewat migrasi SQL. Daftar tabel (disederhanakan; tipe/kolom final ditentukan di Fase 2–3):

| Kelompok | Tabel | Catatan |
|---|---|---|
| Identitas | `users`, `sessions`, `otp_codes`, `devices` (kredensial EA/follower, hash saja) | PII hanya di `users`. `sessions` & `otp_codes` punya `expires_at` |
| Pembayaran | `orders`, `payments`, `payment_proofs` | Bukti: simpan `telegram_file_id` + SHA-256 + status, atau path Storage privat |
| Konfigurasi | `settings` (kunci → `jsonb`), `group_configs`, `group_blacklist` | Dibaca ke cache memori |
| Trading | `signals`, `signal_entries`, `trades`, `mt5_accounts`, `copy_configs` | `signals`/`trades` dipartisi per bulan bila volumenya besar |
| Telemetri | `equity_hourly` (open/high/low/close per jam), `chat_messages`, `audit_log`, `app_events` | `chat_messages` & `app_events` dipartisi per hari/bulan agar pembersihan murah |
| Operasional | `idempotency_keys`, `schema_migrations` | `idempotency_keys` punya `expires_at` |

Aturan data:
- Waktu disimpan **`timestamptz` (UTC)**; tampilkan WIB di UI. Sesi equity harian dihitung 07.00 WIB (aturan yang sudah ada).
- Semua entitas punya `schema_version` bila memakai `jsonb`, `created_at`, `updated_at`, dan untuk mutasi: kolom `version` (optimistic concurrency: `UPDATE ... WHERE id=$1 AND version=$2`).
- ID memakai ULID/UUIDv7 (terurut waktu, indeks B-tree efisien, tanpa kebocoran urutan numerik).
- Uang/harga/lot memakai `numeric`, bukan `float`.
- Data PII (email, nomor telepon) hanya di `users`; tabel lain memakai `user_id`. Email disimpan juga sebagai `email_hash` (untuk lookup unik) bila diperlukan.
- **Indeks seperlunya**: PK, unique (`email_hash`), dan indeks untuk query yang benar-benar ada (mis. `(user_id, created_at desc)`, `expires_at` parsial). Setiap indeks menambah ukuran DB dan beban tulis; catat alasan tiap indeks di migrasi.
- **Row Level Security (RLS) aktif di semua tabel** (default deny); lihat 10.3–10.4.
- Dokumen `jsonb` tetap kecil (idealnya < 2 KB); jangan simpan blob/gambar di tabel.

**Retensi** (dijalankan `pg_cron`, jadwal dari migrasi):

| Data | Retensi | Mekanisme |
|---|---|---|
| Sesi | 7 hari (absolut) + idle | `DELETE ... WHERE expires_at < now()` tiap jam, batch kecil |
| OTP | 10 menit | idem, tiap 15 menit |
| Chat CS | 24 jam | partisi harian, drop partisi lama |
| Log aplikasi/event | 14 hari | partisi harian/mingguan |
| Equity per jam | sesuai kebutuhan chart (mis. 12 bulan) | pruning bulanan |
| Audit | panjang (≥ 12 bulan) | partisi bulanan, arsipkan ke file terenkripsi |
| Sinyal/trade | arsip, tanpa TTL | partisi bulanan |

## 7. Aturan hemat bandwidth & beban database

Target operasional: DB **≤ 60% ukuran plan**, egress bulanan **≤ 30% kuota**, penggunaan pool koneksi **≤ 50%**, dan beban CPU/IO compute rata-rata rendah. Wajib dicatat di metrik.

| Data | Di mana | Frekuensi tulis ke DB |
|---|---|---|
| Snapshot posisi master (tiap detik dari EA) | **Memori saja** | Tidak pernah; catat hanya event *position opened/closed/modified* |
| Heartbeat EA/worker/Telegram | Memori | Hanya saat status **berubah** (online↔offline) |
| Titik equity | Memori (1 titik/10 dtk) | Upsert **bucket per jam** tiap 5 menit + saat jam berganti |
| Settings & kill switch | Cache memori, invalidasi saat PUT | Hanya saat admin mengubah |
| Sinyal & trade | Memori (aktif) + DB | 1 tulis per perubahan status penting, dibatch |
| Log aplikasi | File JSON berotasi di server | Hanya level `WARNING+` dan event audit ke DB |
| Chat CS | DB (retensi 24 jam) | Per pesan |
| Sesi/OTP | DB + cache memori | Buat/hapus saja; kedaluwarsa dibersihkan `pg_cron` |

Praktik wajib:
1. **Outbox pattern:** setiap mutasi ditulis dulu ke SQLite lokal (WAL) dalam satu transaksi, flusher mengirim ke Postgres secara batch (satu transaksi, multi-row `INSERT ... ON CONFLICT DO UPDATE/NOTHING` atau `executemany`; `COPY` untuk impor besar), dengan retry + exponential backoff + jitter, lalu menandai selesai. Karena penulisan memakai `ON CONFLICT`, kirim ulang tidak menggandakan data. Outbox ini **bukan** sumber kebenaran; ia penyangga agar Supabase yang lambat/mati/di-pause tidak menjatuhkan trading.
2. **Baca:** cache in-memory dengan TTL dan *stale-while-revalidate*; invalidasi eksplisit saat tulis. Pilih kolom secukupnya (tanpa `SELECT *`), dan gunakan `jsonb` hanya bila memang perlu.
3. **Paginasi kursor (keyset)**, `limit` maks 50; dashboard publik/admin tidak memuat ulang daftar penuh. Tabel signal/log di dashboard tetap 5 baris (perilaku sekarang) dan memuat lebih banyak hanya on-demand.
4. **Tanpa N+1 dan tanpa seq scan di tabel besar.** Satu halaman = maksimal 1–3 query; review `EXPLAIN` untuk setiap query baru.
5. **Pemantau kapasitas** (menggantikan pencatat RU/WU): kumpulkan (a) ukuran DB dan ukuran per tabel (`pg_total_relation_size`), (b) egress dari dashboard/Usage API Supabase bila tersedia, (c) jumlah koneksi aktif dan waktu tunggu pool, (d) latensi & jumlah query per endpoint (`pg_stat_statements` + metrik aplikasi), (e) ukuran dan umur outbox. Alert Telegram admin saat DB > 60%, egress proyeksi > 60% bulanan, pool > 70%, outbox menumpuk. Pada DB > 80%: aktifkan **mode hemat** (hentikan tulis telemetri non-esensial, perpanjang interval flush, percepat pruning).
6. **Circuit breaker** untuk Postgres: setelah N kegagalan beruntun, berhenti mencoba selama jeda tertentu; sistem berjalan dari memori + outbox, lalu *catch-up* saat pulih.
7. **Maintenance:** pantau *bloat* pada tabel yang sering di-update (`equity_hourly`, `sessions`); atur autovacuum bila perlu. Hindari transaksi panjang.

## 8. Realtime

Tujuan: data terasa langsung, tetapi **biaya per klien mendekati nol** untuk DB.

- **Hub di `core-api`** (WebSocket; SSE sebagai cadangan) pada `rt.<domain>`. Sumber data = state in-memory, bukan DB dan bukan Supabase Realtime.
- **Saluran (channel):** `public.equity`, `public.performance` (anonim, read-only), `client.<userId>` (status akun, posisi follower milik sendiri), `admin.*` (statistik, heartbeat, log live, status Telegram/Gemini).
- **Fan-out terpusat:** hitung payload sekali per tick, kirim ke semua pelanggan channel (bukan dihitung per koneksi).
- **Throttle:** maksimum 1 pesan/detik per channel untuk publik, 2–5 detik untuk dashboard admin; kirim **delta/perubahan** saja dengan `rev` berurutan; klien yang tertinggal meminta snapshot.
- **Kompresi:** `permessage-deflate`, JSON ringkas (kunci pendek, angka dibulatkan sesuai kebutuhan).
- **Autentikasi WS:** tiket JWT berumur 60 detik diterbitkan oleh BFF Vercel setelah memeriksa sesi; hub memverifikasi tanda tangan, mengikat tiket ke channel yang diizinkan, dan menolak `Origin` selain domain resmi.
- **Batas:** maks koneksi per IP/per user, maks ukuran pesan masuk kecil, ping/pong untuk mendeteksi koneksi mati, penutupan otomatis saat idle.
- **Klien web:** jeda koneksi saat tab tersembunyi, reconnect dengan backoff + jitter, fallback ke polling cache 30 detik bila WS gagal.
- Data publik (grafik Indeks Equity, performa) juga disediakan lewat endpoint ber-cache (`s-maxage=30, stale-while-revalidate=60`) sehingga ribuan pengunjung tidak membebani `core-api`.

## 9. Protokol EA MT5 (`/v1/ea/*`, `/v1/follower/*`)

MQL5 `WebRequest` bersifat blocking dan hanya HTTP(S) — rancang protokol yang ringan:

1. **Versi API** di path (`/v1`); EA mengirim `X-EA-Version`; server menolak versi di bawah minimum dengan pesan jelas.
2. **Kredensial per perangkat** (bukan satu `EA_API_KEY` bersama): `device_id` + secret. Server menyimpan **hash**-nya (tabel `devices`). Tiap request membawa `X-Timestamp`, `X-Nonce`, dan `X-Signature = HMAC-SHA256(secret, method + path + timestamp + nonce + SHA256(body))`. Tolak jika selisih waktu > 30 detik atau nonce pernah dipakai (cache nonce di memori, TTL 60 detik).
3. **Snapshot master berbasis perubahan:** EA menghitung hash posisi; kirim hanya bila berubah, ditambah heartbeat ringan tiap 5–10 detik. Server menyimpan `rev` di memori.
4. **Follower:** `GET /v1/follower/positions` dengan `If-None-Match: <rev>` → **304 kosong** bila tak ada perubahan. Dilayani dari memori, tanpa DB. Interval 1 detik cukup karena respons 304 hampir tanpa bandwidth.
5. **Idempotensi:** setiap entry punya `idempotency_key` (`signalId:entryIndex`); server dan EA menyimpan penanda, sehingga polling ulang/restart tidak membuat order ganda (perilaku ini sudah ada — pertahankan dan uji). Di DB dijamin oleh unique constraint + `ON CONFLICT`.
6. **Backoff + jitter** pada error/timeout; batas maksimal request per menit di sisi EA.
7. **Pending order & klaim sinyal** (klaim 90 detik, kedaluwarsa 1 jam, hanya XAUUSD) tetap, namun status klaim hidup di memori dan dipersist lewat outbox.
8. **Kill switch** dibaca dari cache memori EA/server; harus tetap efektif walau DB tidak terjangkau.
9. **Build & rilis EA:** `mt5/build.ps1` meng-compile lewat MetaEditor CLI dan menyuntik nomor versi; berkas `.ex5` didistribusikan lewat rilis terautentikasi, **bukan** di-commit ke git. Sertakan *simulator EA* (skrip Python) untuk contract test.
10. Whitelist WebRequest MT5 hanya untuk `https://ea.<domain>`.

## 10. Keamanan (tingkat tinggi)

### 10.1 Repo & rahasia
- [ ] Jadikan repo **privat**. Jalankan `gitleaks`/`trufflehog` pada **seluruh riwayat git**; jika ada rahasia pernah ter-commit, **rotasi semuanya** (anggap sudah bocor) lalu bersihkan riwayat (`git filter-repo`).
- [ ] Rotasi: `APP_SECRET_KEY`, `ADMIN_PASSWORD`, `EA_API_KEY`, `COPIER_WORKER_API_KEY`, `GEMINI_*`, `TELEGRAM_BOT_TOKEN`, App Password SMTP, `TELEGRAM_API_HASH`, session Telethon.
- [ ] Kredensial Supabase **tidak pernah di repo**: password role database, connection string, `service_role` key, JWT secret. Hanya ada di environment **service** `core-api` (NSSM) atau secret store Windows. `anon` key tidak dipakai sama sekali.
- [ ] `.env` tidak pernah di repo; hanya `.env.example` tanpa nilai. Di Vercel gunakan Environment Variables bertanda sensitif (isi: URL `core-api`, service token Cloudflare, secret HMAC — **tanpa** kredensial Supabase).
- [ ] CI memblokir merge bila secret scan atau dependency audit (`pip-audit`, `npm audit`) gagal; aktifkan Dependabot/Renovate.
- [ ] Branch protection di `main`: PR + review + CI hijau.
- [ ] Akun Supabase & organisasi: **MFA wajib** untuk semua anggota, batasi jumlah owner, gunakan akun khusus (bukan akun pribadi tunggal), aktifkan notifikasi keamanan.

### 10.2 Autentikasi & sesi
- [ ] Hash password **argon2id**. Kebijakan password minimal 12 karakter (sudah ada untuk admin; terapkan juga ke klien).
- [ ] Admin: **TOTP 2FA wajib** + Cloudflare Access + opsi allowlist IP. Pisahkan login admin dari login klien (sudah terpisah; pertahankan).
- [ ] Sesi opaque, token disimpan sebagai **hash**, cookie `HttpOnly; Secure; SameSite=Lax` (`Strict` untuk admin), rotasi saat login/perubahan hak, TTL absolut + idle.
- [ ] CSRF: token/double-submit untuk semua mutasi berbasis cookie; cek `Origin`.
- [ ] Rate limit berlapis (Cloudflare + aplikasi) per IP dan per akun untuk login, OTP, reset password, chat, upload. Pertahankan batas OTP yang ada (1/menit, 3 berturut-turut, tunggu 1 jam, 3 percobaan).
- [ ] Tambahkan **Cloudflare Turnstile** pada register, login, dan lupa password.
- [ ] Respons login/reset **seragam** (tidak membocorkan apakah email terdaftar).

### 10.3 Data sensitif
- [ ] **Password broker klien (managed copy):** target utama adalah *tidak menyimpannya*. Jika bisnis mengharuskan, gunakan **envelope encryption**: DEK acak per record, dibungkus KEK yang **terpisah** dari `APP_SECRET_KEY` **dan dari database** (DPAPI/key file dengan ACL ketat di Windows Server, atau KMS eksternal). Jangan menaruh KEK di Supabase Vault/tabel yang sama: kebocoran DB (atau akses dashboard) tidak boleh cukup untuk membuka password. Dukung rotasi KEK tanpa kehilangan data. Password hanya didekripsi di memori `copier-worker`, tidak pernah dicatat di log, tidak pernah dikirim ke Vercel/browser/LLM. Kolom ciphertext tidak pernah ikut di-`SELECT` oleh endpoint selain `copier-worker`.
- [x] Token terminal follower di Supabase development disimpan sebagai hash; ciphertext lama sudah dihapus. Chat tidak mengungkap token dan token baru hanya dikembalikan saat dibuat/dirotasi.
- [ ] Tambahkan re-auth sebelum menampilkan/merotasi token dan siapkan pembersihan data terenkripsi pada cutover data production.
- [ ] Session Telethon = kunci ke akun Telegram: gunakan akun khusus, simpan di luar folder kode dengan ACL hanya akun service, jangan masuk backup tak terenkripsi.
- [ ] Log tidak boleh memuat password, token, OTP, API key, isi `Authorization`; terapkan *redaction filter* terpusat.
- [ ] Minimalisasi PII; tetapkan masa retensi (log 14 hari, chat 24 jam, OTP 10 menit) dan terapkan lewat `pg_cron`.
- [ ] Catatan kepatuhan: layanan copy trading/pengelolaan dana klien dapat diatur regulasi (mis. perlindungan data pribadi dan aturan perdagangan berjangka/investasi di Indonesia). Data klien akan berada di region Singapore (lintas negara): periksa kewajiban transfer data pribadi dan kebijakan privasi. Konsultasikan dengan ahli hukum sebelum peluncuran 16 Januari 2027.

### 10.4 API, aplikasi, dan database
- [ ] Validasi input ketat (pydantic/zod), batas ukuran body, batas panjang field.
- [ ] Otorisasi **per objek** (klien hanya bisa membaca/ubah akun miliknya) — tulis test negatif untuk setiap endpoint `/api/mt5/accounts/{id}/*`.
- [ ] **Query selalu berparameter** (tanpa string-concat SQL); lint CI untuk pola f-string pada SQL.
- [ ] **Role Postgres khusus** (`algentra_api`) dengan hak minimal: hanya `USAGE` pada schema `app` dan `SELECT/INSERT/UPDATE/DELETE` pada tabel yang diperlukan; tanpa `CREATE`, tanpa akses `auth`/`storage`. Migrasi memakai role terpisah (`algentra_migrator`) yang hanya dipakai saat deploy. Jika memakai role ini, **jangan** memakai kunci `service_role` di aplikasi.
- [ ] **RLS aktif di semua tabel** `app.*` (default deny) sebagai lapisan cadangan; kunci Data API/PostgREST: cabut hak `anon` dan `authenticated` dari schema `app`, jangan expose schema `app` di pengaturan API, atau nonaktifkan Data API sepenuhnya bila tidak dipakai. Tes CI: panggilan dengan `anon` key harus gagal membaca apa pun.
- [ ] **Network Restrictions / allowlist IP** untuk koneksi database ke IP keluar Windows Server (bila fitur tersedia di plan); pakai `sslmode=verify-full`; batasi siapa yang boleh mengakses dashboard Supabase.
- [ ] Header keamanan di Vercel (`vercel.json`/middleware): CSP ketat (tanpa `unsafe-inline` bila bisa), HSTS, `X-Content-Type-Options`, `Referrer-Policy`, `Permissions-Policy`, `frame-ancestors 'none'`.
- [ ] **Hapus Chart.js dari CDN**; paketkan lokal (atau pakai pustaka grafik ringan seperti uPlot/lightweight-charts). Bila ada skrip eksternal, pakai SRI.
- [ ] CORS: whitelist eksplisit; `core-api` tidak menerima panggilan browser langsung kecuali WS ber-tiket.
- [ ] Upload bukti bayar: batasi tipe/ukuran, verifikasi magic bytes, hapus EXIF. Jika memakai Supabase Storage: **bucket privat**, tanpa policy publik, akses hanya via `core-api` yang menerbitkan signed URL berumur pendek.
- [ ] Panggilan `core-api` ↔ Vercel: **Cloudflare Access service token** + tanda tangan HMAC di setiap panggilan BFF; `core-api` menolak yang tidak valid.

### 10.5 LLM (Gemini)
- [ ] Parser sinyal: output wajib JSON ber-skema; **validasi deterministik** setelahnya (simbol hanya XAUUSD + suffix broker, arah, rentang harga masuk akal terhadap harga pasar, SL/TP konsisten, lot/risiko dalam batas). LLM tidak pernah memicu order langsung.
- [ ] Anggap isi pesan Telegram **tidak tepercaya**; tambahkan instruksi sistem anti-injeksi dan jangan menyertakan rahasia dalam prompt.
- [ ] Chat CS: mode publik tanpa data akun; mode klien hanya ringkasan akun milik sendiri; **tanpa token/secret**; batasi panjang input/output, kuota harian per pengguna, dan cache jawaban FAQ untuk menghemat biaya.
- [ ] Pisahkan API key parser dan CS (sudah) dan batasi dengan kuota.

### 10.6 Windows Server & Cloudflare
- [ ] Windows Firewall: **blokir semua inbound**; hanya outbound (Cloudflare Tunnel, Supabase pada port 5432/443, Telegram, broker). Port 8000 tetap bind `127.0.0.1`.
- [ ] RDP: jangan terbuka ke internet; akses via Cloudflare Access/VPN, NLA aktif, MFA, akun admin dengan nama non-default, lockout policy.
- [ ] Jalankan tiap service dengan **akun layanan non-admin**; folder data (`D:\algentra-data`) dan secret dengan ACL hanya akun tersebut.
- [ ] Windows Update terjadwal, Defender aktif, audit log logon.
- [ ] Cloudflare: WAF managed rules, rate limiting per hostname, Bot Fight, HSTS, TLS minimal 1.2, *Authenticated Origin Pulls*/Access untuk hostname admin; aturan khusus `ea.<domain>` (hanya metode & path EA).
- [ ] Audit log (siapa, apa, kapan, IP) untuk: login admin, perubahan settings/kill switch, persetujuan transaksi, rotasi token, aktivasi copy live, serta perubahan skema/akses DB.

## 11. Stabilitas & operasional

- **Service manager:** NSSM (atau Task Scheduler saat boot) untuk `core-api`, `signal-service`, `copier-worker`; restart otomatis dengan backoff, rotasi log, urutan start yang benar. Satu worker uvicorn, tanpa reload.
- **Health:** `GET /health` (proses hidup) dan `GET /ready` (state termuat, outbox tidak menumpuk, koneksi Supabase/Telegram — dilaporkan, bukan syarat hidup). Watchdog memantau heartbeat tiap service dan terminal MT5.
- **Startup tanpa DB:** `core-api` harus bisa start dari snapshot state terakhir (outbox/cache lokal) walau Supabase tidak terjangkau.
- **Graceful shutdown:** flush outbox, tutup pool dan WS dengan kode jelas, jangan memotong job worker di tengah order.
- **Satu instance Telegram bot (long polling):** pakai lock (file/lease) agar tidak ada dua proses (sesuai catatan README).
- **Rekonsiliasi berkala:** bandingkan posisi master vs follower, trade di DB vs posisi MT5, dan perbaiki/alert bila selisih.
- **Backup & restore:**
  - Backup otomatis Supabase (Pro, harian, 7 hari) sebagai lapisan pertama; pertimbangkan PITR bila RPO (batas kehilangan data) harus lebih kecil dari 24 jam.
  - Lapisan kedua milik sendiri: `pg_dump` harian (format custom/`-Fc`, role baca-saja) → enkripsi (age/GPG) → simpan di lokasi **terpisah** dari server dan dari Supabase (mis. object storage lain); simpan 14 salinan.
  - **Uji restore** ke project staging sebelum peluncuran dan tiap kuartal; catat waktu pemulihan (RTO).
- **Migrasi skema:** Alembic dijalankan eksplisit dari workstation development ke project Supabase development dengan role migrator terpisah. Jangan sertakan migrator credential atau perintah migrasi pada service/deploy production. Production menerima project/database yang sudah diprovisikan dan diverifikasi cocok dengan revision aplikasi; startup hanya membaca revision dan gagal cepat bila tidak cocok.
- **Observabilitas:** log JSON terstruktur dengan `request_id`; metrik minimum: latensi p50/p95 per endpoint, jumlah koneksi WS, ukuran outbox, ukuran DB/egress/pool, latensi & error rate Postgres/Gemini/Telegram, umur heartbeat EA, jumlah order gagal. Alert ke Telegram admin.
- **Staging:** akun MT5 **demo** + **project Supabase terpisah** (`algentra-staging`, boleh Free; bangunkan sebelum uji) agar tidak menguji di data live.
- **Runbook** di `docs/`: Supabase down/lambat/paused, pool habis, DB hampir penuh, Telegram logout, Gemini limit, EA tidak mengirim heartbeat, kebocoran token/kredensial DB, restore backup, rollback rilis & migrasi.

## 12. Performa web (Vercel)

- Next.js App Router, TypeScript. Halaman publik (`/`, `/performance`) dengan **ISR** (`revalidate` 60 dtk) + data ber-cache; komponen interaktif saja yang `use client`.
- Portal klien & admin: render dinamis, tetapi data lewat BFF → `core-api` (di-cache pendek per user) dan WS untuk pembaruan.
- Aset: `next/image`, font self-hosted/`next/font`, bundle kecil, *dynamic import* untuk grafik/dashboard, tanpa library yang tak dipakai. Musik latar: opsional, tidak autoplay, `preload="none"`.
- Grafik realtime memakai pustaka ringan dan tinggi tetap (perilaku README dipertahankan: tinggi grafik tidak mengubah layout).
- Minimalkan invocation fungsi Vercel: cache publik di edge/CDN, hindari polling dari browser ke fungsi serverless (gunakan WS ke `rt.<domain>`).
- **Cek plan Vercel:** paket Hobby ditujukan untuk penggunaan non-komersial menurut ketentuan Vercel (verifikasi di ToS terbaru); layanan ini komersial sehingga perlu plan yang sesuai.
- Lighthouse target: Performance ≥ 90, LCP < 2,5 dtk, CLS < 0,1 untuk halaman publik.

## 13. Testing & CI/CD

- **Unit:** `domain/` (parser entry zona/range, risk & lot, validasi SL/TP, mode single/partial, simbol suffix) — cakupan tinggi karena menyentuh uang.
- **Integration:** repository Postgres terhadap database uji (Postgres lokal via container di CI, plus project staging Supabase untuk uji akhir); outbox + circuit breaker (simulasikan Postgres mati/lambat); auth/otorisasi per objek; **tes migrasi** (naik dari skema kosong dan dari snapshot sebelumnya).
- **Contract:** skema OpenAPI di `packages/contracts` → klien TS dan model Python digenerate; CI gagal bila berbeda. Test simulator EA terhadap `/v1/ea/*` (HMAC, replay, idempotensi, 304).
- **Load test** (k6/locust): 1.000 koneksi WS publik, 200 follower polling 1 dtk, lonjakan sinyal; verifikasi **jumlah query/egress DB tetap datar** saat jumlah klien naik, dan pool tidak penuh.
- **Security test:** IDOR, brute force OTP, replay HMAC, injeksi prompt pada parser, header keamanan, akses `anon` key ke database harus ditolak.
- **CI (`ci.yml`):** lint → typecheck → test → secret scan → dependency audit → build web. **Deploy web:** Vercel Git integration (preview per PR, production dari `main`; preview **tidak** diberi kredensial DB). **Deploy server:** skrip `infra/windows/deploy.ps1` (tarik rilis bertag, jalankan migrasi, restart service berurutan, health check, rollback otomatis bila gagal).

## 14. Rencana fase

### Fase 0 — Audit & pengamanan darurat
- [x] Audit struktur, model/tabel, endpoint utama, dan jalur polling; catat hasilnya di bagian "Catatan audit". Inventaris perlu diperbarui lagi sebelum cutover karena endpoint dan arsitektur terus berubah.
- [x] Repo telah diubah menjadi privat, riwayat dipindai, dan aset sensitif dihapus dari working tree. Pembersihan riwayat lama dan rotasi semua secret production masih tertunda.
- [x] Baseline 132 test lama untuk backend dan worker pernah lulus sebelum refactor; hasil ini bukan verifikasi untuk revisi terbaru.
- [x] Tag lokal `pre-refactor` tersedia. Backup database production belum ada di workspace; file SQLite lokal adalah sumber development, bukan bukti backup production.
- [x] Project Supabase development telah dibuat dan dipakai untuk migrasi/impor. Project staging terpisah, MFA organisasi, region/plan production, dan batas biaya belum diverifikasi.
- **DoD:** tidak ada secret di repo/riwayat; baseline test hijau; inventaris endpoint & beban DB selesai.

### Fase 1 — Restrukturisasi tanpa perubahan perilaku
- [x] Pindahkan source backend, worker, dan EA ke struktur monorepo (bagian 4). Pemisahan domain dan use case belum selesai.
- [x] Ganti entrypoint `.bat` dengan skrip PowerShell; sediakan installer NSSM untuk `core-api` di `infra/windows/`. Managed worker tetap memakai Task Scheduler karena terminal MT5 membutuhkan sesi desktop interaktif.
- [x] Lengkapi lint/format/typecheck/CI: workflow memuat lint, pemeriksaan format, typecheck domain, test, contract generation, audit dependency Python/web, build web, dan secret scan Gitleaks untuk riwayat Git penuh. Semua job GitHub hijau pada commit `7bff7de`.
- [x] Pindahkan seluruh logika bisnis dari router ke service; use case EA, auth, dan MT5 telah diekstrak, termasuk 16 endpoint di `mt5.py`.
- **DoD:** [x] seluruh test berjalan lulus; kontrak API tetap tersinkron; tidak ada akses database atau logika bisnis di router. Pengujian staging/live tetap menjadi DoD fase 6–7.

### Fase 2 — Lapisan repository & kontrak API
- [ ] Definisikan interface repository dan adapter memory/SQLite/Postgres yang dapat dipertukarkan; repository SQLAlchemy sudah ada untuk beberapa domain, tetapi auth/EA/MT5 dan abstraksi memory masih tertunda.
- [x] Buat migrasi awal PostgreSQL dari 26 model dan terapkan pada Supabase development. Migrasi harus diperbarui bertahap dari revision yang telah diterapkan, bukan mengedit revision lama.
- [x] Terapkan prefix `/v1`, OpenAPI di `packages/contracts`, pembuatan tipe TypeScript, dan pemeriksaan sinkronisasi kontrak di CI.
- [ ] Pindahkan snapshot posisi, heartbeat, settings, dan klaim sinyal ke `state/` in-memory.
- **DoD:** tidak ada kode yang memanggil SQLite langsung di luar `repositories/`; kontrak CI hijau; migrasi awal berjalan di Postgres kosong pada project development.

### Fase 3 — Supabase Postgres + outbox
- [x] Development memakai schema PostgreSQL, role runtime/migrator/backup terpisah, dan RLS aktif di seluruh tabel `app`; policy hanya memberi akses pada role runtime dan backup.
- [ ] Audit grants/Data API dan ukur kapasitas; `pg_cron` serta partisi masih tertunda.
- [ ] Lengkapi pemantau kapasitas dan circuit breaker; pool PostgreSQL kecil dan TLS sudah dikonfigurasi.
- [ ] Implementasikan outbox dan flusher batch dengan retry/backoff.
- [x] Importer SQLite development mendukung dry-run, idempotensi, checksum per tabel, dan sudah memverifikasi 40.914 baris sumber pada project development.
- [x] Launcher production menolak `MIGRATION_DATABASE_URL`, tidak menjalankan Alembic, dan memeriksa marker/revision/tabel/kolom. Provisioning schema production dan paket data cutover belum dilakukan.
- [ ] Uji dual-write dan bandingkan hasil pada development.
- **DoD:** egress & ukuran DB sesuai target bagian 7 pada beban simulasi; sistem tetap melayani EA saat Supabase diputus; backup/restore teruji; tes `anon` key ditolak.

### Fase 4 — Realtime & protokol EA v1
- [ ] Hub WS/SSE + channel + tiket JWT + throttle/delta.
- [ ] Protokol EA: HMAC per perangkat, ETag/304, snapshot berbasis perubahan, idempotensi.
- [ ] Refactor kedua EA (`mt5/experts/`) + `include/` bersama; `build.ps1`; simulator EA.
- [x] Hapus pengungkapan token via chat. Token terminal di Supabase development kini hash-only; revision development menghapus kolom ciphertext lama.
- [ ] Tambahkan re-auth untuk rotasi token dan bersihkan kolom/data ciphertext di jalur cutover production.
- **DoD:** 200 follower polling tidak menambah query DB; p95 latensi 304 < 50 ms; test replay/HMAC lulus.

### Fase 5 — Web di Vercel
- [ ] Bangun `apps/web` (Next.js): beranda, performa, register/login/verify/reset, `/account`, `/transactions`, dashboard admin, `/telegram-setup`, `/parser-test`, chat CS.
- [ ] BFF → `core-api` dengan Access service token + HMAC; sesi cookie; tiket WS.
- [ ] Header keamanan, ISR, optimasi aset; hapus CDN Chart.js.
- **DoD:** Lighthouse target tercapai; tidak ada kredensial Supabase di Vercel; semua halaman lama punya padanan.

### Fase 6 — Hardening Windows, Cloudflare & database
- [ ] Terapkan checklist 10.6, akun layanan non-admin, firewall, Access untuk admin/RDP.
- [ ] Envelope encryption untuk kredensial sensitif; migrasi data terenkripsi lama; rotasi KEK teruji.
- [ ] Allowlist IP database (bila tersedia), review hak role Postgres, audit RLS.
- [ ] Watchdog, alert, runbook.
- **DoD:** pemindaian port eksternal menunjukkan tidak ada port terbuka; uji pemulihan (kill service, putus Supabase, putus Telegram) lulus.

### Fase 7 — Uji beban, UAT, cutover
- [ ] Load & security test (bagian 13) pada staging dengan akun demo.
- [ ] Uji terbatas di akun live kecil dengan batas lot minimum.
- [ ] Cutover DNS bertahap; pantau metrik 48 jam; rencana rollback ke tag `pre-refactor` siap.
- **DoD:** seluruh target di bagian 15 tercapai selama 7 hari tanpa insiden.

## 15. Target keberhasilan (terukur)

| Aspek | Target |
|---|---|
| Penggunaan Supabase | DB ≤ 60% ukuran plan; egress ≤ 30% kuota bulanan; pool ≤ 50%; **tidak naik** saat jumlah pengunjung/follower naik |
| Latensi | p95 endpoint publik ber-cache < 200 ms; 304 follower < 50 ms; pesan WS tiba < 1 dtk dari perubahan |
| Ketersediaan | Eksekusi sinyal & copy tetap jalan saat Supabase/Gemini/SMTP mati; service pulih otomatis < 60 dtk |
| Sumber daya server | `core-api` idle < 300 MB RAM dan CPU rata-rata < 10% (sesuaikan setelah pengukuran awal) |
| Keamanan | 0 secret di repo/riwayat; 0 port inbound; semua endpoint melewati test otorisasi per objek; 2FA admin aktif; `anon` key tidak bisa membaca data; MFA aktif di Supabase |
| Kualitas | CI hijau; cakupan `domain/` ≥ 90%; runbook lengkap; restore backup teruji (RTO tercatat) |
| Web | Lighthouse Performance ≥ 90 (publik) |

## 16. Larangan keras

- Jangan menyimpan data per-detik (snapshot, heartbeat, tick) ke database.
- Jangan membuat query DB di jalur request EA, follower, atau push realtime.
- Jangan menaruh kredensial Supabase (password DB, connection string, `service_role`/`anon` key), password broker, atau kunci enkripsi di Vercel, browser, log, repo, atau prompt LLM. Jangan memasang integrasi Vercel–Supabase yang menyuntikkan kunci ke Vercel.
- Jangan memakai plan Free untuk produksi.
- Jangan membiarkan tabel di schema yang terekspos Data API tanpa RLS; jangan mengubah skema manual lewat dashboard.
- Jangan membuka port inbound di Windows Server atau meng-ekspos port 8000.
- Jangan menjalankan lebih dari satu proses bot Telegram long polling.
- Jangan membiarkan LLM memicu eksekusi order tanpa validasi deterministik.
- Jangan menambah indeks tanpa alasan tercatat dan perkiraan dampaknya pada ukuran DB/beban tulis.
- Jangan mengubah perilaku selama Fase 1–2.
- Jangan meng-commit `.ex5`, `.env`, file sesi, backup, atau data klien.

## 17. Keputusan yang perlu dikonfirmasi pemilik proyek

1. Apakah mode **managed copy** (menyimpan password broker klien) tetap dipertahankan? Jika ya, wajib envelope encryption dengan KEK di luar DB (10.3); jika tidak, risiko turun drastis.
2. Lokasi fisik/region Windows Server (menentukan region fungsi Vercel; Supabase diasumsikan Singapore).
3. Spesifikasi Windows Server (RAM/CPU), dan apakah server punya **IPv6** (menentukan koneksi langsung vs pooler/add-on IPv4), serta jumlah terminal MT5 yang direncanakan (menentukan apakah `signal-service` digabung ke `core-api` dulu).
4. Plan Supabase produksi (disarankan **Pro**; perlu Small compute jika Micro kurang) dan anggaran bulanan; plan Vercel (lihat catatan Hobby di bagian 12).
5. Perkiraan jumlah klien/follower pada 3, 6, 12 bulan (untuk anggaran ukuran DB, egress, dan compute).
6. Apakah bukti pembayaran cukup disimpan sebagai `telegram_file_id` atau perlu Supabase Storage (bucket privat).
7. Apakah RPO 24 jam (backup harian) cukup, atau perlu PITR (add-on berbayar)?
8. Apakah Supabase Auth tetap tidak dipakai (default dokumen ini) atau dievaluasi setelah Fase 7?

## Catatan audit (isi saat Fase 0)

> Tulis di sini temuan nyata dari kode: struktur folder aktual, daftar endpoint dan frekuensi pemanggilannya, tabel SQLite, perbedaan dari asumsi dokumen ini. Catat juga angka batas plan Supabase yang tampil di dashboard saat project dibuat (ukuran DB, egress, batas koneksi compute) bila berbeda dari bagian 5.1.

### Hasil audit lokal (2026-10-12)

- **Struktur nyata:** aplikasi masih satu backend FastAPI dengan halaman Jinja di `backend/app`; modul layanan dan router memakai model/Session SQLAlchemy secara langsung. Worker Python MT5 ada di `copyTrading/src/copytrade`, EA source ada di `mt5/`, dan proses backend dijalankan dengan `run-service.bat`. Belum ada `core-api`/`signal-service` terpisah, `apps/web`, `packages/contracts`, migrasi Supabase, atau deployment CI/CD.
- **Database:** `backend/app/config.py` menolak URL selain SQLite; production mewajibkan jalur SQLite absolut. `backend/app/database.py` mengaktifkan WAL dan busy timeout, menjalankan migrasi kolom aditif, lalu startup membuat tabel dari metadata SQLAlchemy. Ada 26 model/tabel: `signals`, `ea_executions`, `trades`, `groups`, `logs`, `settings`, `admin_users`, `client_users`, `auth_sessions`, `email_verifications`, `password_reset_codes`, `mt5_accounts`, `managed_copy_profiles`, `payment_orders`, `cs_conversations`, `cs_chat_messages`, `mt5_account_state`, `mt5_history_deals`, `mt5_master_copy_state`, `mt5_master_account_state`, `mt5_master_market_state`, `mt5_master_equity_samples`, `mt5_master_equity_minute_samples`, `mt5_master_equity_candles`, `mt5_master_copy_positions`, dan `ea_status`. File `backend/data/app.db` tidak ada pada audit ini, sehingga volume, isi, indeks historis, serta skema database produksi belum diverifikasi.
- **Endpoint dan frekuensi panas:** terhitung 78 deklarasi route HTTP/WebSocket di `backend/app`. `TelegramSignalEA` memakai `PollIntervalMs=1000`; ia mengambil `/api/ea/pending` per siklus dan mengirim `/api/ea/master/snapshot` sekitar sekali per detik. Endpoint snapshot menghapus/menulis ulang posisi master ke SQLite setiap pemanggilan dan memperbarui state akun. `MT5FollowerCopyEA` meminta `/api/mt5/follower/positions` tiap detik. Managed worker memanggil `/api/mt5/copier/jobs` dengan default `poll_seconds=5`. Bot pembayaran long polling dan Telegram user client dimulai dalam lifespan backend, sehingga backend harus tetap satu proses.
- **Akses data:** endpoint EA, follower, copier, dashboard, portal, pembayaran, dan realtime memakai Session/query SQLAlchemy; belum ada repository interface atau state store in-memory untuk hot path. Snapshot, heartbeat, posisi follower, dan klaim sinyal perlu ditinjau agar query tidak terus meningkat mengikuti polling.
- **Keamanan dan operasional:** `EA_API_KEY` adalah satu key global; managed password dan token akun dienkripsi dengan key turunan `APP_SECRET_KEY`. Kill switch hanya menahan claim sinyal baru di endpoint EA; ia tidak menutup order yang sudah ada atau menghentikan managed-copy worker. `/health` hanya memeriksa proses aplikasi. Installer task managed worker saat ini memakai trigger `AtLogOn` dan hak tertinggi, bukan service non-admin saat boot. Folder `copyTrading/config` dan file `*.example.json` yang diharapkan helper tidak ada dalam snapshot.
- **Isi repo dan secret scan:** Git masih melacak dua binary `.ex5`, workbook `BLACKLIST GRUP.xlsx`, `CUSTOMER_SERVICE_KNOWLEDGE_BASE.md`, audio `musik.mp3`, dan gambar QRIS. Remote dikonfirmasi PUBLIC lalu diubah ke PRIVATE pada 2026-10-12. Gitleaks 8.30.1 memindai riwayat Git dan menemukan enam kecocokan `generic-api-key`; semuanya berada di fixture `backend/tests/conftest.py` atau `backend/tests/test_ea_api.py` dan nilai terkait memiliki penanda placeholder/test. Scan direktori atas README, `.env.example`, `intruksi.md`, dan `.vscode` tidak menemukan kecocokan. GitHub Secret Scanning nonaktif. Tidak ada `.env` atau file database lokal pada folder yang diperiksa; secret environment production tidak tersedia untuk audit. README semula merujuk `.env.example` yang tidak ada; contoh tanpa secret sudah ditambahkan dan panduan live production README sudah diperjelas.
- **Validasi baseline:** 132 test backend dan `copyTrading` lulus di Windows memakai venv Python 3.14.6, dengan dua warning deprecation. `.python-version` repo menetapkan 3.12, sehingga baseline target Python 3.12 masih perlu dijalankan.
- **Masih tertunda sebelum DoD fase 0:** rotasi secret environment production (tidak ada akses ke server/env live), pemindahan aset/data terlacak ke storage yang tetap melayani website/EA, pembersihan riwayat Git lama, backup database production (file DB tidak ada di workspace), dan verifikasi project/plan Supabase. Lightweight tag lokal `pre-refactor` sudah dibuat pada commit awal; belum didorong ke remote. GitHub Secret Scanning tidak tersedia pada repo ini. Belum ada angka batas atau penggunaan Supabase untuk dicatat.
- **Perubahan lanjutan (2026-10-12):** sesuai arahan pemilik, QRIS sekarang berstatus segera tersedia. Portal tidak lagi menawarkan order baru atau unggah bukti; API menolak order/bukti baru, endpoint gambar QRIS merespons HTTP 410, dan pesanan tertunda lama masih dapat dibatalkan serta transaksi lama tetap dapat dibaca. Aset QRIS dihapus setelah tidak lagi dirujuk. Audio lokal keluar dari repo; pemutar hanya tampil jika `PUBLIC_MUSIC_URL` HTTPS CDN/storage dikonfigurasi, memakai `preload="none"`, dan hanya memutar setelah klik. Basis pengetahuan CS dipindahkan ke `backend/app/services/chat/kb/`. File `.env.example` kini mencakup variabel konfigurasi yang tersedia. Perubahan perilaku sebelum fase 3 dibatasi pada permintaan pemilik untuk menonaktifkan pembayaran dan instruksi spesifik aset untuk menghapus musik lokal serta mencegah preload/autoplay; refactor berikutnya tetap mengikuti fase.
- **Pembersihan working tree lanjutan:** workbook blacklist dan dua binary `.ex5` tidak lagi masuk index Git; file lokal tetap ada dan di-ignore supaya tidak kehilangan salinan atau tanpa sengaja ter-commit. Binary memang sudah kedaluwarsa terhadap source dan endpoint akan mengembalikan HTTP 410; README kini mengarahkan operator compile dari source. File audio dan QRIS dihapus dari working tree. Riwayat Git lama belum ditulis ulang karena perubahan ref/commit baru akan diperlukan; repo sudah private, tetapi artefak lama masih dapat diakses lewat commit sebelumnya sampai riwayat dibersihkan.
- **Konfigurasi environment dan DB (2026-10-12):** development membaca `.env.local`; production menerima process environment yang diisi launcher dari environment file terlindungi di luar release. `DATABASE_URL` menerima PostgreSQL psycopg untuk Supabase dengan TLS (SQLite hanya untuk dev/test). Alembic awal dibuat dari 26 model dan hanya dapat dijalankan dari development lewat `infra/windows/migrate-development.ps1`. Bootstrap hanya membuat role/grant sementara dan marker comment database; revision Alembic pertama membuat schema `app`, mengatur grant default, dan menulis revision marker pada schema. Alembic menolak target yang marker project/environment-nya tidak cocok; startup production tidak menjalankan DDL, menolak `MIGRATION_DATABASE_URL`, dan memeriksa marker, tabel, serta revision. Ekspor schema-only dari development menghasilkan snapshot awal produksi tanpa data; importer SQLite ke Supabase development punya dry-run, checksum per tabel, konflik aman, dan dukungan mengulang tabel yang sudah identik. Backup PostgreSQL memakai `pg_dump` custom format dan enkripsi age; role backup read-only dipisahkan dari role runtime. Koneksi project Supabase development dan impor data nyata diverifikasi pada update di bawah; metrik plan dan database production masih belum tersedia.
- **Status koneksi dev terbaru (2026-10-12):** project development berhasil dihubungkan memakai session pooler `aws-0-ap-northeast-1`; koneksi langsung menyediakan IPv6 saja pada host ini. Bootstrap menandai database `algentra:development:<project-ref>` dan membuat role terpisah `algentra_api` serta `algentra_migrator`. Revision Alembic `0001_initial_postgres_schema` membuat schema `app` dan 26 tabel. Importer memindahkan 40.914 baris dari `backend/data/app.db`; dry-run setelah impor mengonfirmasi checksum semua tabel identik. Jalur config/runtime memeriksa schema Supabase dengan sukses. `.env.local` kini memakai URL role API dan migrator dengan TLS; `APP_SECRET_KEY` lokal dipulihkan dari key yang dapat mendekripsi dua kredensial managed-copy, dan password admin development acak disimpan lokal. Nilai secret tidak ditulis di dokumen ini. Grant `CREATE` sementara milik migrator masih perlu dicabut oleh project owner memakai `infra/supabase/revoke-dev-bootstrap-grants.sql`. Region development berbeda dari rekomendasi Singapore di bagian arsitektur; evaluasi latensi sebelum menentukan project production.

- **Pembaruan keamanan dan validasi (2026-10-13):** revision `0002_rls_and_token_redaction` diterapkan ke Supabase development melalui skrip migrasi development. Seluruh tabel `app` kini mengaktifkan RLS; role `algentra_api` memiliki policy runtime dan `algentra_backup` policy SELECT. Kolom ciphertext token EA dihapus; hash yang digunakan EA tetap ada. Importer tidak menyalin kolom ciphertext legacy dari SQLite. Dry-run setelah migrasi terhubung sebagai role API dan memverifikasi 40.914 baris pada 26 tabel sebagai identik setelah redaksi tersebut. Chat tidak lagi membaca atau mengungkap token, dan penanda token dari percakapan lama disamarkan. BFF Next.js membatasi body request ke 6 MiB, memvalidasi origin API, dan mewajibkan HTTPS untuk origin production. Ruff, typecheck, dan build web lulus; ekspektasi test chat diperbarui tanpa menjalankan test suite pada giliran ini.
- **Status pekerjaan per 2026-10-13:** belum ada migrasi atau deployment production, konfigurasi Vercel/Cloudflare live, backup/restore staging, serta cutover data production. `algentra_backup` belum diuji. Repository interface/memory, outbox, state in-memory untuk hot path, ETag/304, ticket realtime, HMAC per perangkat EA, dan rotasi KEK managed-copy masih belum selesai. Tidak ada contributor yang ditambahkan ke repository.
- **Probe readiness (2026-10-13):** `GET /ready` menguji koneksi database dengan `SELECT 1`; saat DB gagal, endpoint memberi HTTP 503 tanpa membocorkan detail koneksi. `GET /health` tetap probe liveness dan tidak memeriksa DB. Status Telegram/outbox belum termasuk karena coordinator/outbox belum tersedia.
- **Secret scan CI (2026-10-13):** job Gitleaks ditambahkan dan dipin ke commit action; checkout memuat riwayat penuh. Konfigurasi hanya mengizinkan dua nilai fixture test yang sudah diketahui secara exact-match. Job `secret-scan` lulus pada commit `efc1ccd`; workflow perlu kembali lulus setelah perubahan berikutnya.
- **Validasi readiness (2026-10-13):** `test_security.py` lulus 3/3; Ruff lint dan format lulus; OpenAPI serta tipe TypeScript diperbarui untuk endpoint baru dan respons 503.
- **Validasi CI readiness (2026-10-13):** workflow commit `7bff7de` lulus semua job: secret scan, Python 3.12 (lint, format, mypy, dependency audit, contract sync, test backend/worker), dan web (audit, typecheck, build). Audit menemukan tidak ada akses database atau logika bisnis langsung di router API; Fase 1 selesai.


### Progres restrukturisasi monorepo (2026-10-12)

- Source backend dipindahkan dari `backend/app` ke `apps/core-api/app`; router berada di `api/v1`, konfigurasi/keamanan/crypto/waktu berada di `core`, use case dan integrasi berada di `services`, dan realtime hub berada di `realtime`. Migrasi dan tests backend ikut pindah ke `apps/core-api`; folder `backend/data` sengaja dipertahankan karena berisi SQLite sumber dan state lokal.
- Worker dipindahkan dari `copyTrading/` ke `apps/copier-worker/`; source EA dan binary lokal berada di `mt5/experts/`. Entry point, installer worker, unduhan source EA, Alembic, dan README diperbarui untuk path baru. Contoh konfigurasi worker tanpa secret ditambahkan.
- Environment test kini memakai `APP_ENV=test`, SQLite terisolasi, dan tidak memuat `.env.local`; ini mencegah test memakai kredensial Supabase lokal. Seluruh 132 test backend dan worker lulus setelah restrukturisasi (2 warning deprecation). Dry-run importer memverifikasi 40.914 baris identik, dan skrip Alembic dari root berakhir pada revision head tanpa perubahan tambahan.
- Fase 1 belum selesai: pemisahan penuh domain/repository, pemindahan semua tanggung jawab router ke service, serta lint/typecheck/CI masih tertunda.

- **Lanjutan restrukturisasi (2026-10-12):** katalog paket client dipindahkan ke `apps/core-api/app/domain/client_plans.py`; entrypoint dev, migrasi dev, backup, dan service production kini memakai PowerShell. Installer NSSM membuat service core API dengan akun virtual terpisah, akses baca minimum ke kode/venv, akses tulis ke data/log, serta akses baca ke environment file terlindungi di luar release. Launcher production memaksa `APP_ENV=production` dan menolak `MIGRATION_DATABASE_URL`; ia tidak menjalankan Alembic. Launcher manual worker juga memakai PowerShell, sedangkan Task Scheduler tetap digunakan untuk worker yang memerlukan sesi MT5 interaktif. README menjelaskan konfigurasi service, ACL secret, restart dan backup live. Keamanan ACL dan perilaku service belum divalidasi pada Windows Server target; refactor domain/service/repository serta CI/typecheck masih tertunda.
- **Lapisan repository awal (2026-10-12):** `SettingsRepository` kini menjadi satu-satunya adapter SQLAlchemy untuk model `AppSetting`; route, lifecycle, pendaftaran, parser, sinyal, risk controls, dan copier menggunakan repository tersebut. Kode route dan service tidak lagi memanggil model pengaturan langsung. Repository ini menjadi awal fase 2, tetapi repository untuk domain lain, kontrak `/v1`, OpenAPI version control, dan pemindahan query dari service/router masih belum dikerjakan.
- **Ekstraksi use case publik (2026-10-12):** query dan pembentukan read-model halaman publik/performa dipindahkan dari router ke `services/public_service.py`; router kini mendelegasikan payload dan contact context ke service. WebSocket serta EA memakai service secara langsung. Endpoint dan bentuk payload lama dipertahankan. Ekstraksi service untuk domain lain serta repository PostgreSQL/memory masih belum selesai.
- **Pemisahan parser preview (2026-10-12):** `POST /api/parser/test` kini mendelegasikan parsing, baca setting, dan validasi ke `services/parser_service.py`; router hanya memetakan kegagalan parser menjadi respons HTTP. Integrasi Gemini dan skema payload tidak berubah.
- **Pengamanan worker Windows (2026-10-12):** installer standalone dan managed worker kini menjalankan Task Scheduler pada saat akun operator login dengan token hak standar; hal ini mempertahankan sesi desktop yang diperlukan MT5 tanpa hak admin. README worker dan root menjelaskan bahwa terminal template/runtime perlu dimiliki atau dapat ditulis akun tersebut.
- **CI Python (2026-10-13):** workflow kini menjalankan `ruff format --check` dan `pip-audit` pada environment Python yang memasang dependency backend serta worker; `pip-audit` ditambahkan ke requirements development. Sebanyak 48 file source diformat; `ruff format --check` (77 file) dan `ruff check` lulus lokal. Secret scanning CI dan bukti workflow penuh hijau masih tertunda; test suite dan `pip-audit` belum dijalankan pada giliran ini.
- **Ekstraksi use case EA (2026-10-13):** endpoint router EA kini mendelegasikan snapshot master, claim sinyal, laporan eksekusi, hasil trade, dan heartbeat ke `services/ea_service.py`; validasi autentikasi dan parameter HTTP tetap di router, bentuk payload dan respons dipertahankan. Pemeriksaan statis Pylance tidak menemukan error. Test EA yang sudah ada belum dijalankan; ekstraksi router auth/MT5, interface repository, dan verifikasi CI masih tertunda.
- **Ekstraksi use case autentikasi (2026-10-13):** login admin/client, pendaftaran dan verifikasi email, pengiriman ulang kode, reset password, dan logout kini didelegasikan router ke `services/auth_service.py`. Endpoint, status code, validasi request, serta respons tetap sama. Router tidak lagi menjalankan query atau mutasi DB. Pylance tidak menemukan error; test autentikasi belum dijalankan. Ekstraksi router MT5, interface repository, dan verifikasi CI masih tertunda.
- **Ekstraksi use case MT5 (2026-10-13):** seluruh 16 endpoint MT5 sekarang mendaftarkan handler dari `services/mt5_service.py`; operasi DB, validasi aturan akun, pengelolaan managed-copy, laporan follower, dan pembentukan payload tidak lagi berada di `api/v1/mt5.py`. Path, method HTTP, dependency, dan handler tetap dipertahankan. Tes managed-copy diarahkan untuk mengatur konfigurasi service setelah pemindahan; hasil validasi dicatat pada pembaruan berikutnya.
- **Validasi ekstraksi MT5 (2026-10-13):** OpenAPI dan tipe TypeScript tergenerate tanpa perubahan kontrak. Test core API lulus 74/74 dan copier-worker lulus 58/58 pada Python 3.14.6; lint, format, mypy domain, npm audit, web typecheck/build, serta pip-audit lulus. Pemeriksaan menemukan dan memperbaiki relative import yang salah pada `auth_service.py`/`ea_service.py` dan impor helper test yang tertinggal. Dependency dibatasi ke `cryptography>=50.0.0,<51` dan `pytest>=9.0.3,<10.0` agar lolos audit advisori terbaru; pip-audit menandai paket lokal `copytrade-mt5` sebagai tidak tersedia di PyPI untuk audit. Workflow GitHub CI run `38287010903` pada commit `63742f9` lulus untuk web dan Python 3.12. Secret scan CI belum ditambahkan.
