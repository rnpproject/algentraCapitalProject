# Algentra Capital — Panduan Production

Runbook ini memandu instalasi production dari database dan API sampai portal, EA, start/stop, dan update. Simpan rahasia di secret store atau file environment di luar repository. Ganti semua nilai contoh sebelum menjalankan perintah. Uji rilis dan EA lebih dulu pada staging, Midtrans sandbox, dan akun MT5 demo.

## Arsitektur production

| Komponen | Lokasi/runtime | Fungsi |
| --- | --- | --- |
| Portal | Next.js di Vercel, root `apps/web` | Website publik dan portal admin/client |
| Core API | Go Windows Service, `127.0.0.1:8000` | API, pembayaran, Telegram, Gemini, EA, dan akses PostgreSQL |
| Database | Supabase PostgreSQL | Data aplikasi; schema final dibuat oleh installer production fresh |
| Copier | Binary Go dan terminal MT5 pada sesi Windows interaktif | Managed copier dari akun master ke follower |
| Signal EA | `TelegramSignalEA.mq5` pada terminal master | Mengirim snapshot untuk copier managed dan dapat mengeksekusi sinyal Telegram |

Vercel mengakses API melalui HTTPS origin di belakang Cloudflare Tunnel/Access dan BFF Next.js. Jangan membuka port 8000 ke internet, mengirim kredensial Supabase ke Vercel/browser, atau menjalankan worker MT5 sebagai Windows Service Session 0. Detail operasi khusus ada di [docs/SERVICE.md](docs/SERVICE.md), fitur/risiko ada di [docs/OPERATIONS.md](docs/OPERATIONS.md), dan copier standalone/managed ada di [apps/copier-worker/README.md](apps/copier-worker/README.md).

## 1. Siapkan project Supabase production

1. Buat project Supabase production baru dan catat project ref.
2. Dari file [fresh production schema](infra/supabase/fresh-production.sql), ganti kedua kemunculan `REPLACE_WITH_SUPABASE_PROJECT_REF` dengan project ref tersebut. Jalankan seluruh file satu kali melalui SQL Editor sebagai role `postgres`. File ini membentuk schema final backend langsung, memberi marker revision `0004_account_daily_profit`, dan menolak berjalan bila schema `app` sudah ada. Setelah berhasil, verifikasi marker sebelum menghubungkan API:

   ```sql
   SELECT shobj_description(oid, 'pg_database') AS database_marker
   FROM pg_database WHERE datname = current_database();

   SELECT obj_description('app'::regnamespace, 'pg_namespace') AS schema_revision;
   ```

   Hasil yang diharapkan adalah `algentra:production:<project-ref>` dan `algentra:revision:0004_account_daily_profit`.
3. File tersebut membuat role `algentra_api` dan `algentra_backup` sebagai `NOLOGIN`. Aktifkan login sekaligus tetapkan password role runtime di SQL Editor. Ganti placeholder dengan password kuat dan unik sebelum menjalankan:

   ```sql
   ALTER ROLE algentra_api LOGIN PASSWORD 'GANTI_DENGAN_PASSWORD_KUAT';
   ```

   Jika menyiapkan host backup, aktifkan role itu dengan cara yang sama dan gunakan password terpisah. Jangan menyimpan password di source code atau file SQL. Gunakan connection string TLS (`sslmode=require` minimal) untuk role runtime; jangan gunakan role `postgres` atau Supabase `service_role` pada aplikasi.
4. Isi `SUPABASE_PROJECT_REF` di environment Core API dengan ref yang sama. Gunakan `DATABASE_URL` dengan role runtime `algentra_api`. Database production baru tidak mengambil data dari SQLite; importer SQLite tetap hanya alat development dan tidak diperlukan untuk deployment fresh.

> API tidak menjalankan DDL ketika startup. Installer fresh hanya untuk project Supabase baru yang belum memiliki schema `app`; jangan jalankan pada database yang berisi data atau schema lama. Untuk rilis berikutnya, siapkan perubahan schema sebagai SQL upgrade terpisah dan tinjau/backup sebelum diterapkan. Prosedur migrasi database development dan importer SQLite lokal dijelaskan di [docs/DEVELOPMENT.md](docs/DEVELOPMENT.md), bukan bagian dari instalasi production fresh.

## 2. Build dan pasang Core API di Windows Server

### Build release

Build pada mesin rilis Windows dengan Go versi yang sesuai `go.mod` dan dari commit yang akan dirilis:

```powershell
$env:CGO_ENABLED = '0'
go test ./...
go build -ldflags='-s -w' -o algentra-backend.exe ./cmd/algentra-backend
go build -ldflags='-s -w' -o algentra-copier-master.exe ./cmd/copier-master
go build -ldflags='-s -w' -o algentra-copier-launcher.exe ./cmd/copier-launcher
go build -ldflags='-s -w' -o algentra-copier-worker.exe ./cmd/copier-worker
```

Alternatifnya, ambil artifact Windows dari CI untuk commit yang sama. Jangan build langsung di folder release aktif. Simpan executable, source EA, checksum/commit, dan catatan versi sebagai satu paket rilis.

### Environment dan service

1. Root project production adalah `D:\Algentra\algentraCapitalProject`. Siapkan Windows Server terkelola, NSSM, dan Cloudflare Tunnel. Salin binary backend ke root project tersebut. Folder `backend\data` adalah data runtime, bukan source code, sehingga tidak tersedia di GitHub dan akan dibuat pada server.
2. Salin [infra/windows/core-api.production.env.example](infra/windows/core-api.production.env.example) ke `D:\AlgentraSecure\core-api.env`. Isi `SUPABASE_PROJECT_REF`, `DATABASE_URL`, `APP_SECRET_KEY`, password admin, dan `BFF_HMAC_SECRET` acak minimal 32 karakter. Untuk akun otomatis setelah pembayaran, isi `MIDTRANS_SERVER_KEY`, `MIDTRANS_MERCHANT_ID`, set `MIDTRANS_IS_PRODUCTION=true`, dan konfigurasi notifikasi Midtrans ke `https://<api-origin>/api/payments/midtrans/notification`. Untuk managed copier, set `COPIER_WORKER_API_KEY` dan `EA_API_KEY`; salin key worker yang sama ke akun Windows worker dan gunakan key EA pada TelegramSignalEA master. Tambahkan integrasi opsional seperlunya.
3. Dari PowerShell administrator pada root project, daftarkan service (installer menunjuk binary di `$ProjectRoot`):

   ```powershell
   .\infra\windows\install-core-api-service.ps1 `
     -ProjectRoot 'D:\Algentra\algentraCapitalProject' `
     -EnvironmentFile 'D:\AlgentraSecure\core-api.env' `
     -NssmPath 'C:\Tools\nssm\win64\nssm.exe'
   ```

   Installer membuat service `AlgentraCoreApi` dengan restart otomatis dan membatasi ACL file environment. Setelah service terdaftar, buat folder data yang belum ada dan batasi ACL file environment serta data. Jalankan perintah ini dari PowerShell administrator:

   ```powershell
   $envFile = 'D:\AlgentraSecure\core-api.env'
   $dataDir = 'D:\Algentra\algentraCapitalProject\backend\data'
   $serviceAccount = 'NT SERVICE\AlgentraCoreApi'

   New-Item -ItemType Directory -Path $dataDir -Force | Out-Null

   icacls.exe $envFile /inheritance:r /grant:r `
     'SYSTEM:F' 'BUILTIN\Administrators:F' "${serviceAccount}:R"

   icacls.exe $dataDir /inheritance:r /grant:r `
     'SYSTEM:(OI)(CI)F' 'BUILTIN\Administrators:(OI)(CI)F' `
     "${serviceAccount}:(OI)(CI)M" /T /C
   icacls.exe $envFile
   icacls.exe $dataDir
   ```

   `backend\data` menyimpan session Telegram dan data lokal aplikasi. Pertahankan folder ini pada path tersebut selama update agar datanya persisten. Service di-bind ke `127.0.0.1:8000`.
4. Pasang Cloudflare Tunnel sebagai service terpisah dengan origin lokal `http://127.0.0.1:8000`; lindungi hostname API dengan Cloudflare Access service token khusus Vercel. Jangan expose port backend secara publik.
5. Mulai dan periksa API:

   ```powershell
   Start-Service AlgentraCoreApi
   Get-Service AlgentraCoreApi
   Invoke-RestMethod 'http://127.0.0.1:8000/health'
   Invoke-RestMethod 'http://127.0.0.1:8000/ready'
   ```

   `/health` memeriksa proses. `/ready` memeriksa koneksi, marker project, revision, serta struktur database.

## 3. Deploy portal Next.js ke Vercel

1. Import repository Git ke Vercel. Set **Root Directory** ke `apps/web`, install command `npm ci`, build command `npm run build`, dan gunakan Node.js 22.
2. Isi environment server-only di Vercel Preview terlebih dahulu:

   | Variable | Nilai |
   | --- | --- |
   | `CORE_API_ORIGIN` | Origin HTTPS API di Cloudflare, tanpa path tambahan |
   | `BFF_HMAC_SECRET` | Sama persis dengan secret Core API |
   | `CF_ACCESS_CLIENT_ID` | Service token client ID bila origin dilindungi Cloudflare Access |
   | `CF_ACCESS_CLIENT_SECRET` | Service token secret yang berpasangan |

   Jangan awali variabel rahasia dengan `NEXT_PUBLIC_`. Vercel/browser tidak perlu `DATABASE_URL` atau Supabase key.
3. Deploy Preview. Uji login, dashboard, permintaan BFF, upload bukti pembayaran, dan WebSocket/realtime dari domain Preview. Atur origin WebSocket yang benar di `WEBSOCKET_ALLOWED_ORIGINS` backend dan policy Access sesuai arsitektur deployment.
4. Setelah lulus, set variabel Production, deploy, pasang domain portal, lalu uji kembali. Setiap perubahan environment membutuhkan deployment baru. Jika nama domain portal berubah, perbarui allow-list WebSocket backend dan redeploy API.

## 4. Pasang MT5 dan EA

Gunakan akun demo dahulu. Pasang MT5 pada sesi desktop Windows khusus yang tetap login; terminal MT5, Algo Trading, EA, dan worker harus aktif bersama.

### A. Copier managed dari portal (master memakai TelegramSignalEA)

TelegramSignalEA mengirim snapshot master ke Core API; worker managed meneruskan snapshot tersebut ke publisher Go dan membuat terminal follower dari profil yang dibayar. EA publisher master terpisah tidak diperlukan untuk mode ini. Worker production tetap harus dipasang satu kali dan membutuhkan template MT5 follower yang bersih. Set `EA_API_KEY` dan `COPIER_WORKER_API_KEY` pada backend, lalu gunakan key EA yang sama pada TelegramSignalEA di terminal master.

1. Pasang MT5 master, login ke akun master, compile dan pasang `mt5/experts/TelegramSignalEA.mq5` pada chart. Jika MT5 master berada di host API, set `ServerURL` ke `http://127.0.0.1:8000`; izinkan URL tersebut di WebRequest MT5 dan isi `ApiKey` dengan `EA_API_KEY` backend. Siapkan folder terminal follower bersih berisi `terminal64.exe` dan folder sinyal yang dapat ditulis worker.
2. Compile EA follower dan login probe dengan MetaEditor; file `.ex5` harus tersedia pada lokasi yang dijelaskan di panduan copier:
   - `mt5/experts/MT5FollowerCopyEA.mq5` pada terminal follower template.
   - `mt5/experts/Algentra/MT5LoginProbe.mq5` untuk validasi login managed.
3. Pastikan `COPIER_WORKER_API_KEY` pada environment Core API dan akun worker sama. Worker otomatis menambahkan WebRequest `http://127.0.0.1:8766` pada terminal follower setelah startup; template tidak perlu disetel manual. Ini memakai desktop Windows interaktif, yang disediakan oleh Scheduled Task installer. Dari PowerShell akun worker jalankan installer task:

   ```powershell
   .\apps\copier-worker\scripts\install_managed_worker_task.ps1 `
     -ProjectRoot 'D:\Algentra\algentraCapitalProject' `
     -ApiBaseUrl 'https://api.example.com' `
     -TerminalTemplateDir 'D:\MT5Templates\CleanFollower' `
     -SignalDirectory 'D:\Algentra\signals'
   ```

   Task bernama `Algentra Managed Copy Worker`, berjalan saat akun worker login, dan membutuhkan sesi interaktif. Periksa `managed-worker.json`/`master.json` hasil installer, lalu login kembali setelah reboot agar MT5 follower aktif. Source master default `telegram_signal`; installer tidak meminta path terminal master.
4. User mendaftar, membuat pesanan, dan membayar dengan QRIS. Settlement otomatis membuat akun. User memasukkan kredensial broker follower serta batas lot di portal; worker memeriksa login, lalu user menyetujui aktivasi live. Uji alur dahulu dengan akun demo dan risk limit yang ditetapkan.

### B. Copier standalone dari posisi MT5 (publisher master + follower EA di host sendiri)

1. Buat satu instalasi MT5 portable terpisah untuk setiap follower, login ke akun yang benar lalu tutup terminal. Compile/pasang `MT5FollowerCopyEA.mq5` pada terminal masing-masing.
2. Siapkan config lokal dari `apps/copier-worker/config/master.example.json`, lalu buat `akun*.json` dan `launcher.json` sesuai [panduan copier](apps/copier-worker/README.md). Tetapkan path terminal unik, folder sinyal yang sama, symbol mapping, mode lot, `dry_run`, dan batas `max_lot_per_order`, `max_posisi_terbuka`, `max_lot_total`.
3. Pasang `MT5MasterPublisherEA.mq5` pada chart mana pun di terminal MT5 yang login ke **akun master sumber posisi**. EA ini membaca posisi terbuka di akun itu dan mengirim snapshot ke publisher Go lokal; EA tidak membuka order. Compile EA dengan MetaEditor, set `MASTER_PUBLISHER_TOKEN` di environment akun Windows yang menjalankan publisher, lalu isi input `PublisherToken` EA dengan secret yang sama (minimal 32 karakter). Allow-list `http://127.0.0.1:8765` pada terminal master dan `http://127.0.0.1:8766` pada terminal follower.
4. Mulai dari root release: `apps\copier-worker\scripts\start-all.ps1`. Task opsional saat login dapat dipasang melalui `apps\copier-worker\scripts\install_startup_task.ps1`. Jangan jalankan publisher master kedua bila managed worker atau launcher sudah menjalankannya.

Pasang EA publisher ini hanya jika ingin menyalin posisi yang dibuka di MT5 master secara langsung (standalone/`master_source: publisher_ea`). Untuk managed copy yang menggunakan sinyal Telegram, pasang `TelegramSignalEA.mq5` di terminal master; `MT5MasterPublisherEA.mq5` tidak dipakai karena worker menerima snapshot dari Core API.

### C. Telegram Signal EA (mengirim order dari sinyal Telegram)

1. Gunakan terminal MT5 khusus pada sesi interaktif, compile `mt5/experts/TelegramSignalEA.mq5`, dan pasang pada chart simbol yang sesuai.
2. Bila EA berjalan pada Windows host yang sama dengan API, set `ServerURL` ke `http://127.0.0.1:8000`; jika berjalan di host berbeda gunakan origin HTTPS khusus yang dapat dijangkau. Allow-list URL itu pada **Tools → Options → Expert Advisors → Allow WebRequest**. EA ini hanya mengirim `X-API-Key`, jadi hostname di belakang Cloudflare Access yang mewajibkan service-token header tidak dapat dipakai langsung oleh EA; siapkan jalur EA terpisah dengan policy yang sesuai dan verifikasi pembatasan endpoint sebelum mengaktifkan akses.
3. Isi input `ApiKey` dengan nilai `EA_API_KEY` backend. Atur watchlist, symbol mapping, batas/default SL/TP, volume dan opsi trading; cocokkan pula whitelist grup Telegram pada halaman admin `/telegram-setup`.
4. Verifikasi pada akun demo dan pantau log Experts/Journal. EA ini dapat mengirim order live dan backend tidak menyediakan dry-run untuk eksekusi EA tersebut. Kill switch menghentikan distribusi signal baru, tetapi tidak menutup posisi terbuka.

### D. Copier EA mandiri milik client

Portal client dapat menyediakan source `MT5FollowerCopyEA.mq5` dan token akun. Client mengunduh/compile `.mq5`, memasang EA pada terminal follower, allow-list origin API pada MT5, memasukkan token dan parameter risiko pada input EA, lalu mengaktifkan Algo Trading. Token hanya ditampilkan saat dibuat/dirotasi dan harus diperlakukan sebagai rahasia. Lihat [docs/OPERATIONS.md](docs/OPERATIONS.md) dan panduan akun copier untuk instruksi yang dilihat client.

## 5. Start, stop, dan status operasional

### Core API Windows Service

```powershell
Start-Service AlgentraCoreApi
Stop-Service AlgentraCoreApi
Restart-Service AlgentraCoreApi
Get-Service AlgentraCoreApi
Invoke-RestMethod 'http://127.0.0.1:8000/health'
Invoke-RestMethod 'http://127.0.0.1:8000/ready'
```

Log NSSM berada di `<ProjectRoot>\logs\core-api\stdout.log` dan `stderr.log`.

### Managed copier task

```powershell
Start-ScheduledTask -TaskName 'Algentra Managed Copy Worker'
Stop-ScheduledTask -TaskName 'Algentra Managed Copy Worker'
Get-ScheduledTask -TaskName 'Algentra Managed Copy Worker' | Select-Object TaskName, State
Get-ScheduledTaskInfo -TaskName 'Algentra Managed Copy Worker' | Select-Object LastRunTime, LastTaskResult
```

Task membutuhkan akun Windows worker login. Periksa `<ProjectRoot>\logs\managed_worker.log`, `managed_worker-task-error.log`, status heartbeat API, dan terminal MT5. Posisi yang sudah terbuka tetap berada pada broker saat task berhenti.

### Copier standalone

Mulai `apps\copier-worker\scripts\start-all.ps1` pada sesi akun worker. Hentikan proses melalui `Ctrl+C` pada konsol yang menjalankannya; jika memakai Task Scheduler, gunakan task yang sudah didaftarkan dan verifikasi semua proses MT5 terkait sebelum stop. Jangan matikan terminal master/follower yang sedang mengelola posisi tanpa memeriksa eksposur di broker.

### Telegram EA dan MT5

EA berjalan di terminal MT5, bukan sebagai Windows Service. Gunakan tombol Algo Trading/Expert Advisor untuk menghentikan eksekusi baru, kemudian kelola posisi dan pending order langsung dari MT5/broker. Menghentikan EA tidak menutup posisi yang sudah terbuka.

## 6. Backup dan update aplikasi/service

### Backup sebelum perubahan

Pastikan backup terenkripsi berhasil dan uji restore pada staging. Ikuti [docs/BACKUP.md](docs/BACKUP.md). Simpan terpisah dari host Windows dan database.

### Update schema

1. Backup database, tinjau SQL upgrade baru, terapkan pada target production melalui operator/DBA, lalu verifikasi schema/revision marker dan data.
2. Jangan jalankan `migrate-development.ps1` atau `fresh-production.sql` pada production yang sudah berjalan. Deploy API hanya setelah schema kompatibel. Hindari rollback DDL/data tanpa rencana pemulihan.

### Update backend dan copier

1. Pilih commit rilis, jalankan CI, simpan binary dan source EA/versinya. Buat backup, periksa tidak ada order/proses yang sedang diproses, dan aktifkan penghentian signal baru bila diperlukan.
2. Stage binary baru di folder release terpisah dan verifikasi checksum. Hentikan `Algentra Managed Copy Worker` dan `AlgentraCoreApi`; pastikan operator memahami bahwa MT5/EA mengelola posisi live dan jangan menutup terminal dengan membabi buta.
3. Ganti file executable dalam release sesuai prosedur cutover, tetapi pertahankan `core-api.env`, `backend\data`, config worker/master, state, folder terminal MT5, dan secret. Jika menggunakan folder versi, update `ProjectRoot`/service secara terencana.
4. Start Core API, periksa `/health` dan `/ready`, cek login admin dan integrasi. Login pada akun worker, start task managed atau launcher standalone, lalu periksa heartbeat, log, publisher master, dan terminal follower.
5. Compile/deploy EA baru secara terpisah pada terminal terkait dan verifikasi input/token/URL allow-list. Jangan aktifkan kembali live trading sampai operator mengesahkan smoke test staging/demo dan production checks.
6. Untuk update portal, deploy commit yang sama ke Vercel Preview, uji BFF dan integrasi API, lalu promote/deploy Production. Environment variable yang berubah memerlukan redeploy.

### Rollback

Hentikan trafik/update yang gagal, pulihkan binary dan frontend versi sebelumnya, lalu cek schema compatibility. Pulihkan database hanya jika diperlukan setelah menghitung transaksi sejak backup; rekonsiliasi order, posisi, dan deal lewat riwayat broker. Prosedur lengkap ada di [docs/SERVICE.md](docs/SERVICE.md) dan [docs/BACKUP.md](docs/BACKUP.md).

## 7. Checklist setelah install/update

- [ ] `/health` sukses dan `/ready` melaporkan database siap.
- [ ] Vercel memakai API origin yang benar; BFF HMAC dan Cloudflare Access valid; login dan request portal berhasil.
- [ ] Telegram/Gemini/email/payment sesuai integrasi yang diaktifkan; Midtrans sandbox diuji sebelum production.
- [ ] Untuk copier managed: TelegramSignalEA mengirim snapshot master; worker meneruskan feed dan follower membaca perubahan posisi.
- [ ] Terminal follower memakai simbol dan parameter risiko yang benar; status broker/heartbeat sehat.
- [ ] Uji pertama menggunakan akun demo. Trading live hanya setelah persetujuan operator/client dan review risk limit.
- [ ] Backup, commit, versi binary/EA, hasil smoke test, dan waktu deployment tercatat.

## Referensi

- [Instalasi service dan Cloudflare/Vercel](docs/SERVICE.md)
- [Fitur portal, EA, dan batas operasi](docs/OPERATIONS.md)
- [Copier standalone dan managed](apps/copier-worker/README.md)
- [Backup dan restore](docs/BACKUP.md)
- [Prosedur lokal development](docs/DEVELOPMENT.md)
- [Kontrak OpenAPI dan tipe frontend](packages/contracts/README.md)
