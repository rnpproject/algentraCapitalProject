# Algentra Capital

Layanan copy trading berbasis AI dengan FastAPI, Supabase PostgreSQL, parser Gemini, koneksi Telegram, REST API untuk EA MT5, dashboard publik, dan workspace operasi privat.

## Menjalankan backend

1. Gunakan Python 3.12 64-bit atau lebih baru.
2. Siapkan project Supabase khusus development sesuai bagian [Bootstrap Supabase development](#bootstrap-supabase-development). Jangan gunakan project production untuk langkah ini.
3. Dari folder proyek, salin `.env.example` menjadi `.env.local`, lalu ganti `APP_SECRET_KEY` dengan nilai acak minimal 32 karakter, `ADMIN_PASSWORD` dengan password minimal 12 karakter, dan `EA_API_KEY` dengan nilai acak. Isi `DATABASE_URL` memakai role `algentra_api`, `MIGRATION_DATABASE_URL` memakai role `algentra_migrator`, dan `SUPABASE_DEV_PROJECT_REF` dengan project ref yang sama. Ambil host/port/database dari Supabase **Dashboard → Connect**; URL project `https://<project-ref>.supabase.co` bukan connection string PostgreSQL. Kedua URL harus menuju database Supabase development yang sama dan menggunakan TLS (`sslmode=require`). File `.env.local` diabaikan Git. Jangan masukkan Supabase `anon` atau `service_role` key; aplikasi terhubung langsung ke Postgres.
4. Buat virtual environment (`python -m venv .venv` di Windows atau `python3 -m venv .venv` di Linux/macOS), lalu pasang dependensi dengan `.venv\Scripts\python -m pip install -r apps/core-api/requirements.txt` di Windows atau `.venv/bin/python -m pip install -r apps/core-api/requirements.txt` di Linux/macOS.
5. Jalankan `infra\windows\migrate-development.ps1` untuk menerapkan revisi skema ke project Supabase development. Script memaksa `APP_ENV=development`, memeriksa project ref dan marker database, lalu memakai role migrator dari `.env.local`. Jika ada database SQLite lokal yang perlu dipindahkan, impor sebelum menjalankan aplikasi untuk pertama kali, sesuai langkah di bagian Bootstrap. Setelah migrasi/impor selesai, jalankan `infra\windows\run-development.ps1`; backend bind ke `127.0.0.1:8000` dan memuat ulang saat kode berubah. Migrasi tidak berjalan otomatis saat backend start.
6. Buka `http://127.0.0.1:8000/loginAdmin` untuk masuk sebagai admin. Admin dapat membuka/menutup pendaftaran dari dashboard; jika ditutup sebelum jadwal launching, pendaftaran otomatis terbuka pada 16 Januari 2027 pukul 09.00 WIB. Klien mendaftar dari `/register`, memverifikasi email, lalu masuk melalui `/login`. Portal akun dan riwayat transaksi masih tersedia, tetapi **pembelian paket dan pembayaran QRIS akan segera tersedia**: order baru dan bukti pembayaran ditolak oleh backend. Jangan melakukan transfer. Pesanan lama yang belum dibayar sebaiknya dibatalkan; bila transfer sudah terlanjur dilakukan, hubungi admin. Nota dan bukti lama yang sudah tersimpan tetap dapat dilihat. Reset kata sandi tersedia melalui `/forgot-password` setelah SMTP diisi.
7. Isi `GEMINI_API_KEY` di `.env.local` untuk Gemini parser sinyal. Chat customer service **ALGENTRA** memakai key terpisah `GEMINI_CS_API_KEY`; isi dengan API key CS yang berbeda. Model dan batas waktu CS dapat diatur lewat `GEMINI_CS_MODEL` dan `GEMINI_CS_TIMEOUT_SECONDS` (default 60 detik), terpisah dari parser sinyal. Kedua fitur tetap nonaktif sampai key masing-masing diisi.
8. Untuk Telegram, isi `TELEGRAM_API_ID` dan `TELEGRAM_API_HASH` dari [my.telegram.org](https://my.telegram.org) di `.env.local`, lalu restart backend dan buka `/telegram-setup` setelah login admin.
9. Bot Telegram masih dapat dipakai untuk menyelesaikan pemeriksaan transaksi lama yang sudah mengirim bukti. Buat bot melalui `@BotFather`, lalu isi `TELEGRAM_BOT_TOKEN` dan `TELEGRAM_ADMIN_CHAT_ID` (ID chat pribadi admin) di `.env.local`. Mulai percakapan pribadi dengan bot setidaknya sekali. Bot pembayaran memakai long polling, jadi jangan pasang webhook untuk bot tersebut dan jalankan satu proses backend saja. Order baru dan bukti baru tetap dinonaktifkan.

### Bootstrap Supabase development

Langkah ini dilakukan sekali pada project Supabase **development** yang terpisah. Gunakan region dan database yang memang akan dipakai untuk pengembangan; project ref terlihat pada URL project Supabase.

1. Buka `infra/supabase/bootstrap.sql`, ganti `REPLACE_WITH_SUPABASE_PROJECT_REF` dengan project ref development, lalu jalankan SQL melalui Supabase **SQL Editor** sebagai project owner. Script membuat role terpisah (`algentra_api`, `algentra_migrator`, `algentra_backup`), memberi grant sementara untuk revision pertama, dan menulis marker environment/ref sebagai comment database. Script ini tidak membuat tabel atau schema aplikasi; Alembic membuat schema `app`, tabel, dan grant runtime.
2. Aktifkan login hanya untuk role yang dipakai development di SQL Editor. `algentra_backup` tetap `NOLOGIN` sampai host backup khusus dikonfigurasi:

   ```sql
   ALTER ROLE algentra_api LOGIN;
   ALTER ROLE algentra_migrator LOGIN;
   ```

3. Set password role dari terminal `psql` yang tersambung sebagai project owner. Gunakan host/port/database dari panel **Connect** Supabase, pakai TLS, dan biarkan `psql` meminta password project owner. Di prompt `psql`, jalankan `\password algentra_api` dan `\password algentra_migrator`; masukkan password berbeda saat diminta. Dengan cara ini password role tidak ditulis ke SQL file atau command history.
4. Di `.env.local`, isi `DATABASE_URL` dengan bentuk `postgresql+psycopg://algentra_api.<project-ref>:<password-url-encoded>@<host>:<port>/postgres?sslmode=require`, lalu isi `MIGRATION_DATABASE_URL` dengan bentuk yang sama memakai username `algentra_migrator.<project-ref>` dan password role migrator. Dari koneksi langsung, username role tidak memakai akhiran project ref. Gunakan session pooler atau koneksi langsung; jangan gunakan transaction pooler untuk migrasi. URL-encode karakter khusus pada password. Project ref harus cocok persis dengan marker pada bootstrap.
5. Jalankan `infra\windows\migrate-development.ps1`. Revision Alembic berikutnya mengaktifkan RLS pada semua tabel `app`, memberi akses penuh hanya kepada role runtime `algentra_api`, akses baca kepada `algentra_backup`, serta meniadakan akses role lain. Revision `0002` juga menghapus kolom ciphertext token EA lama; autentikasi tetap memakai hash token. Setelah revision pertama berhasil membuat schema `app` dan bookkeeping Alembic di `public`, jalankan [revoke-dev-bootstrap-grants.sql](./infra/supabase/revoke-dev-bootstrap-grants.sql) sebagai project owner setelah mengganti project ref di marker. Ini menghapus grant sementara `CREATE` pada database dan schema `public`; role migrator tetap pemilik schema `app` dan dapat mengubah objek di sana.

   ```sql
   REVOKE CREATE ON DATABASE postgres FROM algentra_migrator;
   REVOKE CREATE ON SCHEMA public FROM algentra_migrator;
   ```

   Migrasi berikutnya mengubah objek dalam schema yang sudah ada. Jangan jalankan backend menggunakan project owner atau role migrator.

Untuk provisioning schema production awal, ekspor snapshot tanpa data dari development yang sudah berada di revision head:

```powershell
Set-Location apps\core-api
..\..\.venv\Scripts\python.exe -m app.export_schema_snapshot D:\Secure\algentra-production-schema.sql
```

Perintah ini memeriksa marker project dan revision Alembic, lalu memakai `pg_dump --schema-only`; ia tidak mengekspor tabel data development. Simpan hasilnya di lokasi aman di luar repo. DBA menerapkan file SQL satu kali pada project production sebelum service dirilis, sebagai role project owner. Contoh dari PowerShell, dengan host dari panel **Connect**:

```powershell
$dbHost = 'HOST_DARI_SUPABASE_CONNECT'
psql --host $dbHost --port 5432 --username postgres --dbname postgres --password --set ON_ERROR_STOP=1 --file D:\Secure\algentra-production-schema.sql
```

Setelah import, pastikan comment database bernilai `algentra:production:<project-ref>` dan comment schema `app` bernilai `algentra:revision:<revision-yang-dicetak-exporter>`. File ini hanya untuk provisioning database production kosong/awal. Ia bukan perintah Alembic atau DDL yang dijalankan aplikasi saat start/deploy. Jangan menggunakannya untuk menimpa database production yang sudah aktif.

Bootstrap menerima database tanpa comment atau comment standar Supabase `default administrative connection database`; comment lain ditolak agar marker tidak terganti tanpa pemeriksaan. Jangan hapus atau ubah comment pada database aktif untuk mengakali pemeriksaan target.

Jika file SQLite lokal berisi data yang ingin dipindahkan ke Supabase development, lakukan setelah migrasi skema berhasil. Jalankan dry-run lebih dulu. Importer membandingkan checksum data tiap tabel, melewati tabel yang sudah identik, menambah baris yang belum ada, dan membatalkan seluruh transaksi bila benturan membuat data target tidak sama dengan sumber. Kolom legacy `token_ciphertext` dari SQLite sengaja tidak diimpor karena autentikasi EA menggunakan `token_hash`; token mentah hanya ditampilkan satu kali saat dibuat/dirotasi. Importer menolak tabel atau kolom tak dikenal selain kolom legacy yang diredaksi. Hentikan backend selama impor agar tidak ada penulisan bersamaan:

```powershell
Set-Location apps\core-api
..\..\.venv\Scripts\python.exe -m app.import_sqlite_to_supabase --dry-run
..\..\.venv\Scripts\python.exe -m app.import_sqlite_to_supabase
```

Secara default sumbernya `backend/data/app.db`; path lain dapat diberikan sebagai argumen. Periksa jumlah baris dan status checksum per tabel yang dilaporkan, lalu buat backup terpisah sebelum memindahkan database lokal penting.

Kontak publik pada beranda diatur melalui variabel `CONTACT_*` di `.env.example` dan `.env.local`. Tautan email, Instagram, TikTok, grup/admin Telegram, dan website ditampilkan dengan ikon; alamat sosial harus berupa URL HTTPS.

## Akses publik melalui Cloudflare Tunnel

Quick Tunnel untuk pengembangan membuat URL sementara pada domain trycloudflare.com. Untuk menggunakannya:

1. Jalankan backend dengan `infra\windows\run-development.ps1`.
2. Buka terminal kedua dari folder proyek dan jalankan **cloudflared tunnel --url http://127.0.0.1:8000**.
3. Buka URL HTTPS yang dicetak cloudflared. URL berhenti berlaku ketika proses tunnel dihentikan.

Quick Tunnel ditujukan untuk pengembangan. Untuk domain tetap, buat named tunnel di Cloudflare, atur public hostname menuju http://127.0.0.1:8000, lalu jalankan **cloudflared tunnel run --token TOKEN_DARI_DASHBOARD** dengan token dari dashboard Cloudflare. Atur `COOKIE_SECURE=true` pada environment Windows Service untuk akses HTTPS production. Backend tetap mendengarkan di localhost; tunnel meneruskan trafik tanpa membuka port 8000 ke internet. Lihat [Quick Tunnels](https://developers.cloudflare.com/tunnel/get-started/quick-tunnels/) dan [panduan tunnel bernama](https://developers.cloudflare.com/tunnel/features/locally-managed-tunnels/create-local-tunnel/).

Musik latar tidak dikirim dari server aplikasi. Jika ingin menampilkannya, unggah audio ke CDN/object storage publik dan isi `PUBLIC_MUSIC_URL` dengan URL HTTPS di `.env.local` untuk development atau environment Windows Service untuk production; kosongkan nilainya untuk menyembunyikan pemutar. Audio menggunakan `preload="none"` dan hanya diputar setelah pengunjung menekan tombol.

## Live production di Windows Server

Target database aplikasi adalah Supabase PostgreSQL. Project development menerima migrasi Alembic eksplisit; aplikasi tidak menjalankan DDL atau migrasi saat start. Production harus sudah memiliki skema yang cocok dengan revisi aplikasi sebelum binary dirilis. Jalur pemindahan data produksi SQLite ke Supabase, provisioning project production, Vercel, deploy otomatis, dan rollback otomatis belum selesai; jangan cut over live sampai runbook menyatakan semuanya siap.

Sebelum mengikuti langkah deploy, siapkan host, rahasia, backup, dan prosedur aktivasi/rollback berikut.

### Persiapan host dan konfigurasi

1. Gunakan Windows Server yang dikelola sendiri, Python 3.12 64-bit, dan akun layanan non-admin. Pasang MT5 pada host bila managed copy dipakai. Aplikasi backend dan tunnel harus berjalan dengan akun yang aksesnya dibatasi.
2. Buat project Supabase production terpisah dari development. Sebelum menjalankan `infra/supabase/bootstrap.sql`, ganti marker SQL menjadi `'algentra:production:<project-ref-production>'`; hapus dua baris `GRANT CREATE` karena migrasi pertama hanya dijalankan pada development. Di SQL Editor, aktifkan hanya `algentra_api` dan `algentra_backup`, set password terpisah lewat prompt `psql` dengan `\password`, dan biarkan `algentra_migrator` tanpa LOGIN. Provisioning schema production dilakukan dari snapshot awal development sebelum rilis. Role runtime memakai hak minimum, TLS (`sslmode=require` atau verifikasi lebih ketat), dan tidak menggunakan `service_role`/`anon` key.
3. Simpan konfigurasi service pada file environment terpisah di luar paket rilis, misalnya `D:\AlgentraSecure\core-api.env`. Buat file kosong dan batasi ACL NTFS sebelum mengisi kredensial:

   ```powershell
   New-Item -ItemType Directory -Path 'D:\AlgentraSecure' -Force | Out-Null
   icacls.exe 'D:\AlgentraSecure' /inheritance:r /grant 'SYSTEM:(OI)(CI)F' 'BUILTIN\Administrators:(OI)(CI)F'
   New-Item -ItemType File -Path 'D:\AlgentraSecure\core-api.env' -Force | Out-Null
   notepad.exe 'D:\AlgentraSecure\core-api.env'
   ```

   Isi satu `KEY=VALUE` per baris memakai nama variabel dari `.env.example`: `SUPABASE_PROJECT_REF`, `COOKIE_SECURE=true`, `DATABASE_URL`, `APP_SECRET_KEY`, `ADMIN_PASSWORD`, `BFF_HMAC_SECRET`, dan secret untuk fitur yang digunakan. `BFF_HMAC_SECRET` harus berupa secret acak minimal 32 karakter dan sama dengan environment Vercel. Jangan isi `MIGRATION_DATABASE_URL`, `SUPABASE_DEV_PROJECT_REF`, atau `APP_ENV`; launcher menetapkan environment production dan menolak migrator. File tetap plaintext, jadi aksesnya dibatasi pada administrator, SYSTEM, dan akun service; installer memberi akun service akses baca. Launcher memuat file tersebut ke environment proses dan tidak menjalankan Alembic. Sesi Telethon dan bukti pembayaran masih ditulis ke `backend\data`; lindungi dan pertahankan folder itu saat mengganti rilis.
4. Isi `EA_API_KEY` minimal 24 karakter untuk EA Telegram. Untuk managed copy, gunakan `COPIER_WORKER_API_KEY` acak minimal 32 karakter dan pasang nilai yang sama pada backend serta akun worker. Jangan ganti `APP_SECRET_KEY` tanpa rencana migrasi ulang password broker dan token terenkripsi.
5. Isi key Gemini/Telegram/SMTP hanya jika fitur tersebut dipakai. `TELEGRAM_API_ID` dan `TELEGRAM_API_HASH` harus berpasangan; begitu juga `TELEGRAM_BOT_TOKEN` dan `TELEGRAM_ADMIN_CHAT_ID`. Bot pembayaran memakai long polling sehingga hanya boleh ada satu proses backend.

Pembelian dan pembayaran QRIS belum tersedia pada rilis ini. Endpoint order baru menjawab HTTP 503, endpoint QRIS menjawab HTTP 410, dan unggah bukti ditolak. Jangan mengaktifkan atau mengiklankan alur pembayaran sampai kanal pembayaran aktif dan telah diuji dari portal sampai rekonsiliasi persetujuan admin.

Tidak ada kredensial migrator, perintah Alembic, atau DDL pada langkah deploy aplikasi production. Sebelum cutover, provision database production dengan snapshot awal dari [prosedur export schema development](#bootstrap-supabase-development) yang sudah diverifikasi, dan pastikan comment schema `app` berisi revision yang cocok dengan rilis serta comment database menunjukkan project production. Service production tidak memiliki akses atau kode untuk mengubah struktur database. Setiap revision baru harus memperbarui comment schema sebagai bagian revision Alembic; DDL yang sudah diverifikasi di development harus diprovisikan oleh DBA sebelum kode baru dirilis.

### Web Vercel dan BFF

Web berada di `apps/web` dan memakai API same-origin `/api/core/*`. Vercel harus diarahkan ke folder tersebut sebagai Root Directory. Isi environment server-only `CORE_API_ORIGIN` dengan origin API yang dilindungi Cloudflare Access, `CF_ACCESS_CLIENT_ID` dan `CF_ACCESS_CLIENT_SECRET` dengan service token khusus Vercel, serta `BFF_HMAC_SECRET` yang sama dengan secret pada Windows Server. Browser tidak menerima nilai-nilai tersebut dan tidak menghubungi Supabase. Core API mewajibkan HMAC dengan timestamp, nonce sekali pakai, method, path/query, dan SHA-256 body untuk request `/v1`; endpoint lama `/api` tetap tersedia selama klien berpindah.

Web lokal memakai [contoh environment web](apps/web/.env.example). Atur `CORE_API_ORIGIN=http://127.0.0.1:8000` dan gunakan `BFF_HMAC_SECRET` yang sama pada root `.env.local` dan `apps/web/.env.local`. Jalankan `npm ci` lalu `npm run dev` dari `apps/web`. Service token Cloudflare hanya diperlukan pada production.

Deploy Vercel production, DNS, service token Cloudflare, dan integrasi penuh halaman portal belum divalidasi pada akun hosting. Jangan arahkan domain live sebelum semua environment Vercel dan allow-list Access diuji di preview deployment.

Pasang dependency pada folder rilis dari PowerShell:

```powershell
py -3.12 -m venv .venv
.\.venv\Scripts\python.exe -m pip install --upgrade pip
.\.venv\Scripts\python.exe -m pip install -r requirements.txt
.\.venv\Scripts\python.exe -m pip install -e .\apps\copier-worker
```

### Menjalankan backend, tunnel, dan backup

1. Pasang NSSM, lalu dari PowerShell administrator jalankan `infra\windows\install-core-api-service.ps1 -EnvironmentFile 'D:\AlgentraSecure\core-api.env' -NssmPath 'C:\Tools\nssm\win64\nssm.exe'`. Script memasang service `AlgentraCoreApi` dengan akun virtual terbatas `NT SERVICE\AlgentraCoreApi`, memberi akses minimum ke kode, data, log, dan environment file, lalu mengatur restart otomatis. Setelah database, environment, izin folder, dan tunnel siap, mulai dengan `Start-Service AlgentraCoreApi`. Service bind ke `127.0.0.1:8000`, memakai satu worker, dan tanpa reload. Jangan jalankan development launcher atau instance backend lain di production.
2. Pasang named Cloudflare Tunnel sebagai service/task terpisah dengan ingress ke `http://127.0.0.1:8000`. Aktifkan TLS dan HSTS di Cloudflare. Jangan buka port 8000 pada firewall atau bind backend ke alamat publik.
3. Pasang PostgreSQL client tools (`pg_dump`, `pg_restore`) dan `age` pada host backup. Buat environment file backup terpisah dan batasi ACL kepada akun task backup; isinya mencakup `BACKUP_DIR`, `BACKUP_DATABASE_URL` (role backup read-only), `BACKUP_AGE_RECIPIENT` (public key age), dan `DATABASE_URL`. Jadwalkan `infra\windows\run-backup.ps1 -EnvironmentFile 'D:\AlgentraSecure\backup.env'` setiap hari dengan Task Scheduler pada akun backup. Script membuat custom dump schema `app`, mengenkripsinya, menghapus dump plaintext sementara, dan merotasi 14 file terenkripsi. Simpan private key age di luar server dan uji restore ke project staging. Untuk tes manual:

   ```powershell
   .\infra\windows\run-backup.ps1 -EnvironmentFile 'D:\AlgentraSecure\backup.env'
   ```

   Simpan salinan tambahan di lokasi terpisah dari host dan Supabase; folder backup pada disk yang sama bukan perlindungan terhadap kerusakan host.
4. Jika managed copy digunakan, siapkan `apps\copier-worker\config\managed-worker.json`, `master.json`, terminal master, template follower yang bersih, dan folder runtime. Jalankan helper sekali untuk menyalin file contoh konfigurasi, lalu isi path lokal yang valid sebelum menjalankan worker. Installer task worker memakai akun standar dan trigger **AtLogOn** karena MT5 membutuhkan sesi desktop interaktif; setelah reboot worker baru berjalan saat akun masuk ke Windows. Untuk pemulihan otomatis tanpa operator, siapkan akun terminal khusus dengan auto-logon terbatas dan kontrol fisik server. Pastikan akun memiliki izin tulis hanya pada direktori worker/runtime dan terminal yang dipakai. Jangan menyalakan managed worker bersamaan dengan standalone launcher.
5. Installer NSSM mengatur restart otomatis dan rotasi stdout/stderr core API saat mencapai 10 MB per file. Pantau kapasitas folder log, hasil Task Scheduler worker, dan status `cloudflared`; alert terpusat belum dikonfigurasi di repo.

### Prosedur setiap rilis dan aktivasi live

Rilis sekarang memerlukan maintenance window. Uvicorn berjalan satu proses dan tidak menjalankan DDL/migrasi database saat startup. Cutover SQLite produksi ke Supabase dan uji restore belum selesai.

1. Uji kandidat rilis lebih dulu pada staging/akun demo, dengan salinan database. Dari root repo, jalankan test backend dan worker seperti pada bagian [Test](#test). Tinjau perubahan kode, konfigurasi, dependency, dan versi source EA sebelum menjadwalkan maintenance.
2. Pilih window saat tidak ada order yang sedang diproses. Aktifkan kill switch admin untuk menghentikan klaim sinyal baru. Periksa akun/simbol, EA, pending order, posisi terbuka, dan semua managed follower. Jika posisi broker masih terbuka, pastikan SL/TP broker sudah benar dan operator siap memantau langsung dari MT5/broker selama downtime.
3. Buat backup dengan `infra\windows\run-backup.ps1 -EnvironmentFile 'D:\AlgentraSecure\backup.env'`, verifikasi file terenkripsi keluar dan hasil backup dapat dibaca, lalu catat versi lama, versi baru, waktu backup, dan lokasi file backup.
4. Hentikan service backend. Ganti kode sambil mempertahankan environment Windows Service, database Supabase, `backend\data`, konfigurasi MT5, state worker, serta folder terminal. Pasang dependency rilis, kemudian mulai kembali satu service.
5. Uji origin dari server:

   ```powershell
   Invoke-WebRequest http://127.0.0.1:8000/health
   ```

   HTTP 200 hanya membuktikan proses aplikasi menjawab; endpoint ini tidak memeriksa database atau integrasi lain. Lanjutkan dengan login admin, dashboard, status Telegram, heartbeat EA, status worker, tunnel, dan alur portal yang berubah pada rilis. Pastikan log startup bersih sebelum membuka trafik.
6. Untuk EA Telegram, cek `ServerURL`, `EA_API_KEY`, allow-list WebRequest MT5, Algo Trading, akun/server/simbol, quote, SL/TP, lot, dan Experts/Journal. Backend tidak memiliki demo/dry-run switch untuk EA sinyal dan tidak membatasi jumlah posisi EA. Uji EA lebih dulu pada akun demo; halaman parser test hanya menguji parsing/validasi dan tidak membuat order.
7. Lepas kill switch hanya setelah operator memutuskan sinyal valid berikutnya boleh menjadi order live. Untuk managed copy, pastikan worker online, pemeriksaan login follower berstatus valid, dan batas lot/jumlah posisi sudah ditinjau; client harus menyetujui **Mulai copy trading (live)** di portal.

### Jeda darurat dan rollback

Kill switch dashboard menghentikan distribusi sinyal baru ke EA Telegram; ia tidak menutup posisi/pending order yang sudah ada dan tidak menjeda managed copy. Menjeda managed profile menghentikan salinan selanjutnya, tetapi posisi follower yang sudah terbuka tetap berada di akun broker. Untuk menghentikan eksposur, kelola atau tutup posisi dan pending order langsung dari MT5/broker, lalu verifikasi status pada akun.

Jika smoke test gagal, aktifkan kill switch dan hentikan service/worker terkait. Pulihkan kode rilis sebelumnya dan ulangi pemeriksaan origin, dashboard, tunnel, dan terminal. Bila database perlu dipulihkan, decrypt backup ke host pemulihan terproteksi, restore ke project Supabase staging terlebih dahulu, lalu rencanakan cutover setelah menghitung transaksi setelah waktu backup. Rekonsiliasi order, deal, posisi, dan pending order terhadap riwayat broker/MT5 sebelum mengaktifkan kembali trading.

Untuk Cloudflare **502**, periksa origin dengan `Invoke-WebRequest http://127.0.0.1:8000/health` lebih dulu. Jika origin gagal, tinjau status service dan log startup. Jika origin berhasil, periksa status/log `cloudflared`, DNS hostname, dan ingress. Untuk gangguan database, periksa lokasi `DATABASE_URL`, izin akun service, kapasitas disk, dan backup terbaru. Pastikan pasangan variabel Telegram bot tetap sama-sama terisi atau sama-sama kosong.


## Layanan managed copy untuk akun client

Mode **Dikelola server** membolehkan client yang sudah memiliki akun terdaftar mengatur follower melalui `/account`; client tidak perlu memasang MT5 atau EA di komputernya. Pembelian akun baru sementara dinonaktifkan. Untuk akun yang sudah ada, setelah client menyimpan detail dan password broker, worker otomatis memeriksa login MT5 tanpa mengirim order. Portal menampilkan hasilnya dan tombol live hanya aktif setelah login serta izin trading terminal terverifikasi. Worker mengaktifkan Auto Trading pada salinan terminal follower saat menyiapkannya. Akun tetap dijeda sampai client menetapkan batas lot, lalu menekan **Mulai copy trading (live)** dan menyetujui konfirmasi. Password broker disimpan terenkripsi menggunakan key turunan `APP_SECRET_KEY`, tidak dikembalikan ke halaman client, dan hanya diberikan ke worker yang memakai `COPIER_WORKER_API_KEY`.

Worker berada di [apps/copier-worker/](./apps/copier-worker/) dan membutuhkan Windows dengan terminal MT5 serta package MetaTrader5. Untuk VPS, atur `COPIER_WORKER_API_KEY` yang sama pada backend dan akun service Windows, gunakan terminal template follower yang bersih dan terpisah dari master, lalu ikuti [panduan managed worker](apps/copier-worker/README.md#managed-copy-dari-portal-client). Tanpa worker online, permintaan tersimpan tetapi akun tidak menyalin posisi. Jika `APP_SECRET_KEY` diganti, password managed yang tersimpan tidak dapat dibuka; client perlu menyimpan ulang kredensial dari portal.

Untuk menjalankan di lokal, mulai backend dengan `infra\windows\run-development.ps1`, login-kan terminal MT5 sumber, lalu dari root folder proyek jalankan `.venv\Scripts\python.exe apps\copier-worker\scripts\run_managed_worker.py`. Helper membaca worker key dari `.env.local` hanya di development; pada server, atur key pada environment akun Windows. `apps/copier-worker/config/managed-worker.json` harus mengarah ke backend lokal dan `apps/copier-worker/config/master.json` ke terminal sumber yang login. Lalu buka `/account` sebagai client. Akun follower baru diproses setelah pembelian disetujui, detail broker dan batas risiko disimpan, dan client secara sadar mengaktifkan copy live.

## Endpoint API dan kompatibilitas

OpenAPI versi baru tersedia pada `packages/contracts/openapi.json` dan tipe TypeScript pada `packages/contracts/src/generated.d.ts`. Gunakan `/v1/auth`, `/v1/cs`, `/v1/mt5`, `/v1/ea`, `/v1/payments`, `/v1/settings`, `/v1/parser`, `/v1/telegram`, serta `/v1/dashboard`. Endpoint lama `/api/...` tetap dipertahankan agar EA, worker, dan website lama tetap kompatibel selama migrasi klien.

- `GET /health`
- Public site: `GET /` dan `/performance`; data performa anonim di `GET /api/public/performance` hanya memakai trade yang sudah ditutup.
- `POST /api/auth/login` untuk admin, `POST /api/auth/client-login` untuk klien, `POST /api/auth/logout`, pendaftaran melalui `POST /api/auth/register`, `POST /api/auth/resend-code`, dan `POST /api/auth/verify-email`, serta reset kata sandi melalui `POST /api/auth/password-reset/request` dan `POST /api/auth/password-reset/confirm`. Halaman reset: `GET /forgot-password`.
- Portal klien: `/account`; pengelolaan terminal melalui `GET/POST /api/mt5/accounts`, `PUT /api/mt5/accounts/{id}`, `PUT /api/mt5/accounts/{id}/active`, dan `POST /api/mt5/accounts/{id}/rotate-token`.
- Copy posisi MT5: `POST /api/ea/master/snapshot` memakai `X-API-Key` dari EA Telegram; `GET /api/mt5/follower/positions` memakai `X-Account-Token` khusus untuk setiap terminal follower.
- Managed copy: client membuat pesanan melalui `/api/payments/orders`, mengatur `/api/mt5/accounts/{id}/managed-copy`, dan mengaktifkan atau menjeda lewat `/api/mt5/accounts/{id}/managed-copy/active`. Worker Windows memakai `GET /api/mt5/copier/jobs` dan endpoint status/laporan dengan `X-Copier-Worker-Key`.
- `GET /api/stats`
- `GET /api/signals?limit=100&offset=0`
- `GET /api/logs?limit=100&offset=0&level=INFO&search=...`
- `GET /api/settings` dan `PUT /api/settings` dengan body `{"values":{"confidence_threshold":0.75}}`
- `POST /api/parser/test` untuk menguji klasifikasi dan validasi. Halaman admin tersedia di `/parser-test`.
- Telegram: `GET /api/tg/status`, `POST /api/tg/send-code`, `POST /api/tg/verify-code`, `POST /api/tg/verify-2fa`, `POST /api/tg/logout`, `POST /api/tg/reconnect`, `GET /api/tg/groups`, dan `POST /api/tg/groups/select`.
- EA: `GET /api/ea/pending`, `POST /api/ea/report`, `POST /api/ea/result`, `POST /api/ea/heartbeat`, dan `POST /api/ea/master/snapshot` memakai header `X-API-Key` yang nilainya sama dengan `EA_API_KEY`.

## Website publik dan workspace operasi

Beranda publik Algentra Capital tersedia di `/`; bagian performa menampilkan P&L agregat aktual dari trade tertutup. Database saat ini belum mengaitkan trade dengan akun klien terpisah, jadi angka publik tidak diklaim sebagai hasil beberapa akun. Workspace privat tetap tersedia di `/dashboard` setelah login admin. Tabel signal terbaru dan log sistem menampilkan paling banyak lima baris.

Grafik Indeks Equity publik menampilkan grafik garis dari laporan equity akun utama MT5. Riwayat dimulai ulang pada sesi harian pukul 07.00 WIB, dan celah data ditampilkan tanpa membuat data sintetis. Tinggi grafik tetap dibatasi agar perubahan equity tidak mengubah tata letak halaman.

## Chat AI customer service ALGENTRA

Tombol chat ALGENTRA tampil pada halaman publik dan portal klien, tetapi tidak pada workspace admin. Server menentukan mode dari sesi login: pengunjung mendapat jawaban umum, sedangkan klien hanya mendapat ringkasan akun miliknya. Riwayat publik memakai cookie pengunjung bertanda tangan; percakapan publik dan klien disimpan terpisah. Riwayat tidak dapat dibaca lagi setelah 24 jam dari awal percakapan. Pesan yang kedaluwarsa dihapus saat endpoint chat dipanggil dan oleh pembersih berkala setiap 15 menit.

Chat memakai `GEMINI_CS_API_KEY` dan `GEMINI_CS_MODEL`; key parser sinyal `GEMINI_API_KEY` tidak digunakan oleh layanan CS. Pesan dan ringkasan label/status akun milik klien dikirim ke Gemini untuk menjawab pertanyaan. Token EA tidak dimasukkan ke prompt Gemini dan tidak pernah ditampilkan melalui chat. Token mentah hanya ditampilkan satu kali di Portal Klien saat dibuat atau dirotasi; bila hilang, klien harus merotasinya dari portal. Riwayat chat lama yang pernah berisi penanda token kini menampilkan pesan bahwa token disamarkan dan perlu dirotasi.

`APP_SECRET_KEY` masih melindungi password broker managed-copy yang tersimpan. Token terminal disimpan sebagai hash; token lama yang sebelumnya dienkripsi sedang dihapus dari project development melalui migrasi data. Jangan ganti `APP_SECRET_KEY` tanpa rencana pemulihan password broker managed-copy.

## Dashboard admin

Setelah login admin, buka `/dashboard` untuk melihat statistik signal dan trade, heartbeat EA, status Telegram/Gemini, signal terbaru, statistik grup, grafik profit, log, dan riwayat trade. WebSocket `/ws` mengirim pembaruan berkala; dashboard mencoba menyambung ulang otomatis. Log dan riwayat trade dapat difilter serta diekspor ke CSV. Halaman settings mengatur confidence, symbol mapping, deviasi harga, mode entry, update signal, kill switch, dan konfigurasi Gemini. Sinyal yang dieksekusi selalu dikirim sebagai order live; tidak ada demo mode atau dry run. **Single entry** membuka satu order; untuk zona harga, order memakai titik tengah dan TP pertama. **Partial entry** mempertahankan pembagian order per TP; zona tanpa TP dibagi menjadi dua order di batas zona. Perubahan mode berlaku untuk signal baru yang diklaim EA; signal yang sudah diklaim menyimpan mode sebelumnya agar laporan entry tetap cocok. Signal dibatasi ke XAUUSD dan kedaluwarsa setelah satu jam. Chart.js dimuat dari CDN sehingga grafik memerlukan akses jaringan browser.

Endpoint dashboard (semuanya memerlukan sesi admin): `GET /api/stats`, `GET /api/signals`, `GET /api/groups/stats`, `GET /api/logs`, `GET /api/logs/export.csv`, `GET /api/trades`, `GET /api/trades/export.csv`, `GET/PUT /api/settings`, `PUT /api/settings/gemini-config`, serta `WS /ws`.

Sebelum memanggil Gemini, filter lokal hanya meneruskan pesan yang memiliki arah transaksi dan harga; percakapan serta laporan hasil trade dilewati tanpa permintaan Gemini. Backend dan EA hanya menerima XAUUSD, termasuk nama broker yang memakai suffix.

## Uji login Telegram manual

1. Buat API ID dan API hash pada `my.telegram.org`, simpan di `.env.local` untuk development atau environment Windows Service untuk production, lalu restart backend.
2. Login admin di `/loginAdmin`, lalu buka `/telegram-setup`.
3. Masukkan nomor telepon dengan kode negara, minta kode, lalu masukkan kode yang diterima di aplikasi Telegram.
4. Jika akun memakai 2FA, masukkan password 2FA. Status harus berubah ke `connected`.
5. Muat daftar grup/channel, aktifkan whitelist yang diinginkan, isi alias bila perlu, lalu simpan.
6. Kirim pesan uji di salah satu grup whitelist dan periksa hasil di API `/api/signals` dan `/api/logs`.

Session Telethon berada di `backend/data/telegram_user.session`, tidak disajikan oleh FastAPI, dan folder `backend/data` masuk `.gitignore`. Logout dari halaman Telegram Setup mencabut session yang tersimpan.

## Memasang EA MT5

1. Buat `EA_API_KEY` acak minimal 24 karakter di `.env.local` untuk development atau environment Windows Service untuk production, lalu restart backend.
2. Login sebagai admin dan unduh source EA utama dari `/downloads/TelegramSignalEA.mq5`.
3. Buka file `.mq5` dengan MetaEditor dari MT5, tekan **F7** untuk compile, lalu pasang file `.ex5` hasil compile ke `MQL5/Experts`.
4. Di MT5, buka **Tools > Options > Expert Advisors**, aktifkan **Allow WebRequest for listed URL**, lalu tambahkan URL yang sama dengan `ServerURL` (default publik: `https://algentracapital.my.id`; untuk server lokal: `http://127.0.0.1:8000`).
5. Pasang EA ke chart akun MT5 live, isi `ApiKey` dengan nilai `EA_API_KEY`, dan tetapkan `LotMode` serta `RiskPercent` sesuai batas risiko yang diinginkan. EA langsung mengirim order live; pastikan pengaturan sinyal, lot, SL/TP, dan izin trading terminal benar sebelum mengaktifkannya.
6. Periksa tab Experts/Journal untuk heartbeat, status koneksi, dan laporan signal.

Resolusi simbol mencoba nama persis, akhiran broker yang unik, lalu pemetaan eksplisit. Untuk broker yang menambahkan akhiran umum, isi `SymbolSuffix` pada EA Telegram dan EA Copy Trading, misalnya `c`. Untuk nama berbeda atau beberapa kandidat yang sama-sama cocok, isi `SymbolMapCsv` dalam format `XAUUSD=XAUUSDc` pada EA terkait. `MarketWatchlistCsv` mengatur simbol yang dipublikasikan EA utama untuk panel harga.

`GET /api/ea/pending` mengklaim signal selama 90 detik. Jika simbol atau quote belum tersedia, izin trading mati, atau spread melewati batas, EA mencoba signal kembali sampai kedaluwarsa satu jam. Jika signal lawan datang saat posisi EA di XAUUSD masih profit, signal baru diabaikan. Jika posisi lawan sedang floating minus, EA menutup posisi tersebut sebelum membuka arah baru. Jika jarak risiko SL melebihi TP terdekat tetapi masih tercakup oleh TP terjauh, backend menyimpan TP terjauh saja; EA lalu memvalidasi jarak SL terhadap target itu. Sinyal tetap ditolak jika TP terjauh pun tidak cukup. Untuk sinyal MARKET tanpa entry, backend memilih TP terjauh agar EA dapat memvalidasinya terhadap quote live. EA menyimpan penanda idempotensi per entry dan mengirim ulang laporan bila perlu agar polling/restart tidak membuat order ganda. Pada mode Partial, jika sinyal memiliki beberapa TP, EA membuat satu order untuk setiap TP. Mode Single memaksa satu order dan hanya menggunakan TP pertama. Semua order memakai SL dari sinyal; bila SL tidak tersedia, EA memakai `DefaultSLPoints`. Dengan `LotMode=LOT_RISK_PERCENT` dan `RiskPercent=1.0`, anggaran risiko seluruh entry dari satu sinyal adalah sekitar 1% ekuitas akun jika semua entry mencapai SL. Anggaran uang itu dibagi rata per entry, lalu lot tiap entry dihitung dari estimasi P/L MT5 antara harga entry dan SL serta dibulatkan ke bawah sesuai langkah lot broker. Jumlah posisi tidak dibatasi oleh EA.

## Copy posisi dari akun utama ke akun follower

Pendaftaran dan reset kata sandi memakai email OTP enam digit yang berlaku 10 menit. Pengiriman kode dibatasi satu kali per menit dan maksimal tiga kali berturut-turut; setelah pengiriman ketiga, pengiriman berikutnya menunggu satu jam. Kode menerima maksimal tiga percobaan. Gmail bisa dipakai sebagai pengirim: buat akun Gmail khusus, aktifkan 2-Step Verification, lalu buat App Password dari pengaturan Google Account. Atur `EMAIL_SMTP_HOST`, `EMAIL_SMTP_PORT`, `EMAIL_SMTP_USERNAME`, `EMAIL_SMTP_PASSWORD`, `EMAIL_FROM`, dan `EMAIL_SMTP_STARTTLS` di `.env.local` untuk development atau environment Windows Service untuk production memakai App Password, bukan password Gmail biasa, lalu restart backend. Endpoint juga dibatasi per alamat IP.

```env
EMAIL_SMTP_HOST=smtp.gmail.com
EMAIL_SMTP_PORT=587
EMAIL_SMTP_USERNAME=algentra.sender@gmail.com
EMAIL_SMTP_PASSWORD=app-password-google
EMAIL_FROM="Algentra Capital <algentra.sender@gmail.com>"
EMAIL_SMTP_STARTTLS=true
```

### Panduan klien: menyalin posisi MT5

EA Telegram yang berjalan pada terminal akun utama Algentra mengeksekusi sinyal Telegram sekaligus membagikan posisi terbuka yang dibuat EA itu. Backend menyediakan snapshot tersebut untuk semua terminal follower klien. Follower menyalin posisi, perubahan SL/TP, dan penutupan posisi. Portal tidak meminta password broker. Masuk ke `/account`, daftarkan setiap akun follower, lalu simpan token yang muncul karena token lengkap hanya ditampilkan saat dibuat atau dirotasi.

Unduh EA setelah login ke portal:

- Unduh [source EA follower](https://algentracapital.my.id/downloads/MT5FollowerCopyEA.mq5) — compile di MetaEditor dan pasang hasil `.ex5` pada setiap terminal MT5 akun klien.

Untuk memasang EA:

1. Di MT5, pilih **File → Open Data Folder**, lalu buka `MQL5/Experts`.
2. Buka `MT5FollowerCopyEA.mq5` dengan MetaEditor, tekan **F7** untuk compile, lalu salin file `.ex5` hasil compile ke folder tersebut.
3. Di MT5, pilih **Tools → Options → Expert Advisors**. Aktifkan **Allow WebRequest for listed URL** dan tambahkan `https://algentracapital.my.id`.
4. Dari **Navigator → Expert Advisors**, tarik EA follower ke chart. Isi `AccountToken` dengan token akun itu dan pastikan `ServerURL` berisi `https://algentracapital.my.id`.
5. Aktifkan **Algo Trading**. EA Telegram di terminal utama Algentra harus berjalan agar posisi sumber terus diperbarui. EA follower menyalin langsung ke akun live jika terminal dan akun mengizinkan trading.

EA Telegram menerbitkan posisi dengan magic number miliknya setiap detik; posisi manual atau EA lain di terminal utama tidak dibagikan. Follower hanya menyalin posisi terbuka, perubahan SL/TP, dan penutupan, bukan pending order sebelum terisi. `VolumeMultiplier` menentukan volume salinan relatif terhadap volume sumber, bukan persentase ekuitas. Lot disesuaikan dengan maksimum dan langkah volume broker. `MaxEntryDeviationPercent=0` menonaktifkan batas keterlambatan entry; isi angka positif bila ingin membatasi salinan yang jauh dari harga entry master. Akun follower tetap harus memakai mode **hedging** karena follower mengelola tiap posisi sumber secara terpisah. Jika data sumber tidak diperbarui selama 20 detik, follower berhenti menyinkronkan. Jika EA berhenti, posisi follower yang sudah terbuka tetap berada di terminal dan perlu dikelola di MT5. Rotasi token jika hilang atau terekspos, lalu masukkan token baru ke EA.

Semua terminal follower klien menerima sumber posisi Algentra yang sama. Akun MT5 klien hanya perlu didaftarkan sebagai follower; terminal sumber tidak perlu didaftarkan lewat portal klien.

Repo menyimpan source `.mq5`; file `.ex5` hasil compile tidak disertakan. Compile source dengan MetaEditor sebelum instalasi. Endpoint unduh `.ex5` hanya melayani binary yang tanggal modifikasinya tidak lebih lama dari source; compile ulang jika endpoint mengembalikan HTTP 410.

### Entry berupa rentang harga

Pesan seperti `XAUUSD BUY NOW 4100:::4093`, `XAUUSD BUY 4000-4010`, atau `BUY : ENTRY ZONE : 4185–4190 | TP : 4195, 4200, 4205, 4210, 4215 | SL : 4175` disimpan sebagai zona entry. Parser menyimpan batas bawah dan atas; rentang eksplisit mengesampingkan kata BUY NOW/SELL NOW. Pada mode Partial, bila zona memiliki beberapa TP, EA membuat jumlah order sesuai jumlah TP dan membagi entry merata di dalam zona. Untuk dua TP atau lebih, BUY dimulai dari batas bawah dan SELL dari batas atas; satu TP memakai titik tengah zona. Pada mode Single, satu order memakai titik tengah zona dan TP pertama. Harga entry diselaraskan dengan tick symbol dan dijaga tetap di dalam zona. Tiap order memakai TP sesuai urutannya; jenis order (Limit atau Stop) mengikuti posisi harga pasar MT5. Bila zona tidak memiliki TP, mode Partial membuat dua order di batas zona dengan TP default, sementara mode Single membuat satu order di titik tengah.

Sinyal dengan lebih dari satu entry memerlukan akun MT5 mode hedging agar tiap order menjadi posisi terpisah; akun netting ditolak. Pending order berlaku satu jam lalu dihapus jika belum terisi. EA memakai expiration broker bila tersedia dan penghapusan berkala saat EA berjalan sebagai fallback.

SL dan setiap TP divalidasi terhadap seluruh zona: SL BUY harus di bawah batas bawah dan TP BUY di atas batas atas; aturan SELL kebalikannya. Jarak risiko dari entry ke SL juga tidak boleh lebih besar daripada jarak ke TP terdekat. EA menghormati kill switch, spread, dan persentase risiko per signal. Lot mengikuti batas dan langkah volume broker; bila hasil perhitungan risiko lebih kecil dari lot minimum broker, entry ditolak agar anggaran risiko tidak terlampaui.

## Test

Jalankan test backend dari folder proyek: `.venv\Scripts\python -m pytest apps/core-api/tests` di Windows atau `.venv/bin/python -m pytest apps/core-api/tests` di Linux/macOS. Jalankan worker tests di Windows dengan `$env:PYTHONPATH="apps/copier-worker/src"; .venv\Scripts\python.exe -m pytest apps/copier-worker/tests`.

## Belum teruji

Koneksi Gemini, Telegram, dan SMTP dengan kredensial sungguhan belum diuji. EA Telegram dan EA copy MT5 perlu dikompilasi ulang dari source di MetaEditor sebelum dipasang. Keduanya belum diuji pada akun live; setelah dipasang, order dieksekusi secara live ketika Algo Trading dan izin trading MT5 aktif.
