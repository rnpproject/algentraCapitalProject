# Operasi dan fitur aplikasi

Panduan installasi production ada di [README](../README.md); deployment service di [SERVICE.md](./SERVICE.md), development di [DEVELOPMENT.md](./DEVELOPMENT.md), dan backup/restore di [BACKUP.md](./BACKUP.md).

## Portal client dan managed copy

Mode **Dikelola server** memakai worker Windows: setelah pembayaran manual diperiksa dan disetujui admin, client mengisi password broker serta batas risiko. Password dienkripsi di backend dan hanya diberikan kepada worker terautentikasi. Worker menyiapkan terminal MT5 terisolasi dan memeriksa login tanpa order; copy tetap dijeda sampai client meninjau batas lot dan menyetujui aktivasi live. Managed mode menjalankan copier Python worker, bukan memasang EA MQL5. Lihat [panduan managed copier](../apps/copier-worker/README.md#managed-copy-dari-portal-client).

Mode **Pasang EA sendiri** menggunakan [MT5FollowerCopyEA.mq5](../mt5/experts/MT5FollowerCopyEA.mq5). Client mengunduh source dari `/downloads/MT5FollowerCopyEA.mq5` pada origin Core API, compile di MetaEditor, memasang `.ex5` pada terminal, mengizinkan URL Core API di WebRequest, lalu mengaktifkan Algo Trading. EA menghitung lot/risk menggunakan kalkulasi profit dan margin MT5 dalam mata uang akun serta spesifikasi instrumen. Atur `SymbolSuffix`/`SymbolMapCsv` untuk nama instrumen broker yang berbeda. Broker tetap dapat menolak simbol, volume, margin, atau level stop yang tidak sesuai spesifikasinya. Token hanya ditampilkan saat dibuat/dirotasi; jangan bagikan token. Posisi tetap berada pada broker ketika EA berhenti.

Pembayaran sementara memakai instruksi transfer manual dan review admin via bot Telegram; QRIS nonaktif. Bukti JPG/PNG/WebP maksimal 5 MB disimpan privat. Akun hanya dibuat setelah admin menyetujui bukti. Jangan transfer sebelum instruksi resmi dikonfirmasi. Konfigurasikan `PAYMENT_MANUAL_INSTRUCTIONS`, `TELEGRAM_BOT_TOKEN`, dan `TELEGRAM_ADMIN_CHAT_ID` pada environment backend.

## API dan kontrak

OpenAPI tersedia di [packages/contracts/openapi.json](../packages/contracts/openapi.json), dengan tipe TypeScript di [generated.d.ts](../packages/contracts/src/generated.d.ts). API versioned memakai `/v1/auth`, `/v1/cs`, `/v1/mt5`, `/v1/ea`, `/v1/payments`, `/v1/settings`, `/v1/parser`, `/v1/telegram`, dan `/v1/dashboard`; endpoint `/api/...` dipertahankan selama migrasi klien.

- Health: `GET /health` dan `GET /ready`.
- Auth client/admin: `/api/auth/*`; halaman client `/account` dan `/transactions` disajikan oleh web di Vercel.
- Akun MT5: `GET/POST /api/mt5/accounts`, `PUT /api/mt5/accounts/{id}`, `/active`, dan `/rotate-token`.
- Pesanan: `/api/payments/orders`; managed setup dan aktivasi: `/api/mt5/accounts/{id}/managed-copy`.
- EA: `POST /api/ea/master/snapshot`, `GET /api/ea/pending`, `POST /api/ea/report`, `/result`, dan `/heartbeat`.
- Admin: stats/signals/logs/settings di `/api/stats`, `/api/signals`, `/api/logs`, dan `/api/settings`.
- Telegram: `/api/telegram/status`, send/verify code, groups dan whitelist; halaman admin `/telegram-setup` ada di web Vercel.

## Website, customer service, dan admin

Website publik hanya disajikan oleh Next.js di Vercel — Core API di Windows tidak lagi menyajikan halaman web apa pun (semua path non-API mengembalikan JSON 404). Performa agregat memakai trade tertutup dan bukan klaim hasil dari akun klien individual. Admin login melalui `/admin/login` di web Vercel, lalu memakai `/dashboard` untuk statistik, signal, trade, log, kill switch, settings, dan status integrasi. Halaman `/parser-test` di web Vercel menguji parsing/validasi saja; halaman itu tidak mengirim order. EA sinyal tetap live saat dijalankan; tidak tersedia demo/dry-run backend.

Chat ALGENTRA tersedia untuk pengunjung dan client; ringkasan account client dibatasi ke milik client tersebut. Konfigurasikan key terpisah `GEMINI_CS_API_KEY`; key parser `GEMINI_API_KEY` tidak dipakai oleh CS. Jangan kirim password broker atau token EA melalui chat.

Untuk Telegram, isi `TELEGRAM_API_ID` dan `TELEGRAM_API_HASH`, lalu login admin di web Vercel dan buka `/telegram-setup`. Session Telethon disimpan privat di `backend/data/telegram_user.session`; jangan masukkan folder `backend/data` ke Git.
