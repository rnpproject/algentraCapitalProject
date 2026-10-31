# Operasi dan fitur aplikasi

Panduan installasi production ada di [README](../README.md); deployment service di [SERVICE.md](./SERVICE.md), development di [DEVELOPMENT.md](./DEVELOPMENT.md), dan backup/restore di [BACKUP.md](./BACKUP.md).

## Portal client dan managed copy

Pendaftaran client dan provisioning akun setelah pembayaran QRIS sudah otomatis: client daftar/verifikasi email, mengisi detail akun MT5 dan memilih paket, lalu menyelesaikan QRIS. Notifikasi Midtrans yang sah membuat akun trading tanpa review admin. Transfer manual tetap melalui review dan tidak memenuhi alur tanpa operator. Untuk managed copy, client memasukkan password broker serta batas risiko di portal; password dienkripsi dan pemeriksaan login berjalan otomatis. TelegramSignalEA pada terminal master mengirim snapshot yang dipakai worker copier, sehingga `MT5MasterPublisherEA` terpisah tidak diperlukan dalam mode managed. Copy live tetap dimulai oleh client setelah meninjau risiko. Lihat [panduan managed copier](../apps/copier-worker/README.md#managed-copy-dari-portal-client).

Mode **Pasang EA sendiri** menggunakan [MT5FollowerCopyEA.mq5](../mt5/experts/MT5FollowerCopyEA.mq5). Installer mengompilasi binary; client mengunduh `.ex5` dari `/downloads/MT5FollowerCopyEA.ex5`, menyalinnya ke folder `MQL5/Experts`, mengizinkan URL Core API di WebRequest, lalu mengaktifkan Algo Trading. Source `.mq5` tetap tersedia dari portal jika client perlu mengubah atau mengompilasi sendiri. EA menghitung lot/risk menggunakan kalkulasi profit dan margin MT5 dalam mata uang akun serta spesifikasi instrumen. Atur `SymbolSuffix`/`SymbolMapCsv` untuk nama instrumen broker yang berbeda. Broker tetap dapat menolak simbol, volume, margin, atau level stop yang tidak sesuai spesifikasinya. Token hanya ditampilkan saat dibuat/dirotasi; jangan bagikan token. Posisi tetap berada pada broker ketika EA berhenti.

Untuk onboarding otomatis, aktifkan pembayaran QRIS Midtrans production (`MIDTRANS_SERVER_KEY`, `MIDTRANS_IS_PRODUCTION=true`) dan arahkan notifikasi Midtrans ke `/api/payments/midtrans/notification`. Backend memverifikasi signature, nominal, status settlement, dan merchant opsional sebelum membuat akun. Bila QRIS tidak dikonfigurasi, portal masih mengizinkan transfer manual yang memerlukan review admin; jangan gunakan metode itu untuk alur tanpa operator. Bot Telegram hanya diperlukan untuk notifikasi/review transfer manual.

## API dan kontrak

OpenAPI tersedia di [packages/contracts/openapi.json](../packages/contracts/openapi.json), dengan tipe TypeScript di [generated.d.ts](../packages/contracts/src/generated.d.ts). API versioned memakai `/v1/auth`, `/v1/cs`, `/v1/mt5`, `/v1/ea`, `/v1/payments`, `/v1/settings`, `/v1/parser`, `/v1/telegram`, dan `/v1/dashboard`; endpoint `/api/...` dipertahankan selama migrasi klien.

- Health: `GET /health` dan `GET /ready`.
- Auth client/admin: `/api/auth/*`; halaman client `/account` dan `/transactions` disajikan Next.js pada hostname portal.
- Akun MT5: `GET/POST /api/mt5/accounts`, `PUT /api/mt5/accounts/{id}`, `/active`, dan `/rotate-token`.
- Pesanan: `/api/payments/orders`; managed setup dan aktivasi: `/api/mt5/accounts/{id}/managed-copy`.
- EA: `POST /api/ea/master/snapshot`, `GET /api/ea/pending`, `POST /api/ea/report`, `/result`, dan `/heartbeat`.
- Admin: stats/signals/logs/settings di `/api/stats`, `/api/signals`, `/api/logs`, dan `/api/settings`.
- Telegram: `/api/telegram/status`, send/verify code, groups dan whitelist; halaman admin `/telegram-setup` disajikan pada hostname portal.

## Website, customer service, dan admin

Website publik dan portal disajikan oleh Next.js Windows Service di belakang Cloudflare Tunnel. Core API tetap terpisah pada loopback; Tunnel mengarah ke Next.js, yang meneruskan request API ke Go. Installer portal tidak mengubah pengaturan RDP host. Performa agregat memakai trade tertutup dan bukan klaim hasil dari akun klien individual. Admin login melalui `/admin/login`, lalu memakai `/dashboard` untuk statistik, signal, trade, log, kill switch, settings, dan status integrasi. Halaman `/parser-test` menguji parsing/validasi saja; halaman itu tidak mengirim order. EA sinyal tetap live saat dijalankan; tidak tersedia demo/dry-run backend.

Chat ALGENTRA tersedia untuk pengunjung dan client; ringkasan account client dibatasi ke milik client tersebut. Chat dan parser memakai key/model Gemini yang sama dari konfigurasi admin. Jangan kirim password broker atau token EA melalui chat.

Untuk Telegram, isi `TELEGRAM_API_ID` dan `TELEGRAM_API_HASH`, lalu login admin di portal dan buka `/telegram-setup`. Core API Go menyimpan session MTProto di `backend/data/telegram_user.gotd.session`; file itu berisi kredensial login dan harus tetap privat. Instalasi production fresh tidak membutuhkan file session SQLite lama: login pertama membuat session Go baru. Konversi `backend/data/telegram_user.session` hanya untuk mempertahankan login Telegram pada upgrade host yang memang memiliki session Telethon sebelumnya; ini terpisah dari schema/database aplikasi.

## Batas beban Supabase

Core API memakai pool PostgreSQL maksimum 5 koneksi dan minimum 0. Ringkasan dashboard dibagi antar-tab melalui cache memory 10 detik (endpoint stats 5 detik); performa publik dibagi antar-pengunjung 10 detik, termasuk stream SSE/WebSocket, dan status pendaftaran 5 detik. Cache berlaku per proses Core API, jadi jalankan satu instance pada arsitektur satu server ini. Polling halaman juga sudah diperjarang: detail dashboard 30 detik, daftar transaksi 20 detik, dan status QRIS 15 detik. Endpoint health/readiness dan data publik tidak membaca session DB; request session tidak menulis `last_seen_at` berulang.

Snapshot EA menyimpan satu candle per akun per menit dan retensi 45 hari; pembersihan data lama berjalan paling sering sekali per jam dengan indeks waktu. Performa client mengagregasi deal menjadi satu baris per posisi di PostgreSQL sebelum hasil dikirim ke aplikasi, bukan mengirim seluruh baris riwayat pada setiap refresh stream. Snapshot performa client juga dibagi antar-tab milik akun yang sama selama 15 detik. Perubahan ini membatasi query berulang dan egress database, tetapi angka aktual tetap bergantung pada jumlah pengunjung, ukuran kurva candle, dan pola penggunaan. Pantau metrik database Supabase setelah deployment.
