# Algentra Capital — Panduan Production

Portal dan Core API dipasang pada satu Windows Server di `C:\Algentra`. Cloudflare Tunnel menerbitkan portal HTTPS tanpa IPv4 publik atau port masuk; Supabase menyediakan PostgreSQL. MT5 tetap berjalan pada sesi Windows interaktif.

## Arsitektur production

| Komponen | Lokasi/runtime | Fungsi |
| --- | --- | --- |
| Portal | Next.js Windows Service, root `apps/web` | Website publik dan portal admin/client |
| Core API | Go Windows Service, `127.0.0.1:8000` | API, pembayaran, Telegram, Gemini, EA, dan akses PostgreSQL |
| HTTPS publik | Cloudflare Tunnel Windows Service | HTTPS publik tanpa IPv4, DNS A/AAAA, atau port masuk |
| Database | Supabase PostgreSQL | Data aplikasi; schema fresh dibuat oleh installer |
| Copier | Binary Go dan terminal MT5 pada sesi Windows interaktif | Managed copier dari akun master ke follower |
| Signal EA | `TelegramSignalEA.mq5` pada terminal master | Mengirim snapshot untuk copier managed dan dapat mengeksekusi sinyal Telegram |

Browser dan API berbagi origin; Cloudflare Tunnel meneruskan request ke Next.js lokal, yang merutekan `/api/core/*` ke Go. Jangan buka port 3000/8000 atau RDP 3389 ke internet. Tunnel saat ini hanya menerbitkan portal publik; jangan jalankan worker MT5 sebagai Windows Service Session 0. Detail operasi ada di [docs/SERVICE.md](docs/SERVICE.md), fitur/risiko ada di [docs/OPERATIONS.md](docs/OPERATIONS.md), dan copier standalone/managed ada di [apps/copier-worker/README.md](apps/copier-worker/README.md).

## 1. Persiapan sebelum instalasi

Contoh `DATABASE_ADMIN_URL` (ganti placeholder dan URL-encode karakter khusus pada password):

```text
postgresql://postgres:<URL_ENCODED_PASSWORD>@db.<PROJECT_REF>.supabase.co:5432/postgres?sslmode=require
```

Untuk **Session pooler**, pilih mode Session pada Dashboard > Connect dan gunakan format host/username yang ditampilkan di sana, port `5432`, serta `sslmode=require`. Jangan gunakan Transaction pooler port `6543`.

URL runtime di bawah hanya contoh format; installer membuat password role `algentra_api` dan nilai `DATABASE_URL` secara otomatis:

```text
postgresql://algentra_api:<GENERATED_PASSWORD>@db.<PROJECT_REF>.supabase.co:5432/postgres?sslmode=require
```

Jangan gunakan password owner untuk koneksi runtime.

### Domain Cloudflare Tunnel tanpa VCC

Cloudflare Tunnel bisa dipakai pada paket Free dan akun Cloudflare Free tidak membutuhkan kartu kredit. Domain khusus tetap perlu didaftarkan, tetapi Anda tidak perlu membayar dengan VCC jika registrar menerima metode lain. Sebagai contoh, [Rumahweb mencantumkan transfer bank, virtual account, e-wallet, QRIS, dan pembayaran minimarket](https://www.rumahweb.com/pembayaran/); biaya dan metode pembayaran domain mengikuti registrar yang dipilih.

1. Daftarkan domain pada registrar, atau gunakan domain yang sudah Anda miliki. Pilih nama untuk portal, misalnya `portal.domainanda.com`.
2. Buat akun Cloudflare gratis, pilih **Add a site**, masukkan domain utama (`domainanda.com`), lalu pilih paket **Free**. Cloudflare akan memberi dua nameserver.
3. Buka panel registrar dan ganti nameserver domain ke dua nameserver Cloudflare tersebut. Tunggu sampai status zone/domain di Cloudflare menjadi **Active**. Cloudflare menjelaskan bahwa nameserver perlu diarahkan ke Cloudflare agar Cloudflare menjadi DNS otoritatif untuk zone tersebut ([panduan setup zone](https://developers.cloudflare.com/dns/zone-setups/full-setup/)).
4. Buat API token melalui **My Profile → API Tokens → Create Token → Create Custom Token**. Beri izin **Account → Cloudflare Tunnel → Edit**, **Zone → DNS → Edit**, dan **Zone → Zone → Read**. Batasi Account ke akun Cloudflare Anda dan Zone ke domain portal saja. Salin token saat dibuat; jangan kirim token ke siapa pun.
5. Saat wizard bertanya hostname, masukkan `portal.domainanda.com` saja—tanpa `https://`. Installer membuat Tunnel, konfigurasi route ke portal, serta CNAME DNS secara otomatis. Tidak perlu membeli IPv4 atau membuka port masuk; server hanya membuat koneksi keluar ke Cloudflare. Lihat [prasyarat dan izin API Tunnel Cloudflare](https://developers.cloudflare.com/tunnel/get-started/).

Jika belum punya domain, Quick Tunnel gratis dapat memberi URL `trycloudflare.com` sementara tanpa akun/domain, tetapi nama URL berubah saat tunnel dibuat ulang, tidak memiliki jaminan uptime, dan tidak mendukung SSE ([batasan Quick Tunnel](https://developers.cloudflare.com/tunnel/get-started/quick-tunnels/)). Dashboard Algentra memakai SSE untuk pembaruan langsung, jadi installer production tetap menggunakan domain khusus dan Tunnel bernama yang stabil.

Tidak perlu mengedit file `.env` untuk instalasi production. Siapkan nilai berikut; wizard akan meminta semuanya di satu alur dan menyimpan konfigurasi sensitif secara lokal dengan ACL/DPAPI.

| Input wizard | Kapan diperlukan | Catatan |
| --- | --- | --- |
| Hostname portal dan Cloudflare API token | Wajib | Domain harus aktif di Cloudflare. Token perlu izin Tunnel Edit dan DNS Edit/Read; Tunnel/DNS dibuat otomatis. Tunnel hanya menerbitkan portal. |
| URL koneksi owner Supabase | Wajib untuk instalasi baru | Gunakan role `postgres`, koneksi direct atau Session Pooler port 5432, dan `sslmode=require`. Schema dan password role runtime dibuat otomatis. |
| Login, password, dan server MT5 master | Wajib | Login diuji dengan `MT5LoginProbe`; chart master dan preset EA disiapkan otomatis. Password terenkripsi lokal dan tidak diminta ulang saat repair install. |
| Gemini API key dan model | Opsional | Model default `gemini-3.5-flash-lite`; dapat dikosongkan lalu diatur dari Settings. |
| Telegram API ID/hash dan nomor akun sumber | Opsional, untuk sumber sinyal Telegram | Kode OTP/2FA dimasukkan saat wizard menghubungkan akun; OTP dapat diminta lagi bila sesi terputus. |
| Telegram bot token dan admin chat ID | Opsional, untuk bot notifikasi/review | Tidak diperlukan jika fitur bot pembayaran tidak digunakan. |
| Midtrans Server Key dan Merchant ID **atau** detail rekening | Pilih salah satu | Midtrans membuat notifikasi pembayaran otomatis. Jika memakai transfer, admin tetap perlu meninjau pembayaran. |
| SMTP host, port, username, password, dan pengirim | Opsional | Diperlukan untuk verifikasi email dan reset password. Tanpa SMTP, fitur email tidak aktif. |

Secret aplikasi, password admin awal, password role runtime, cookie sesi, serta API key EA/worker dibuat otomatis. API key EA/worker dapat dilihat di Dashboard > Settings. RDP tetap dikelola oleh setup Anda sendiri. Jika instalasi terhenti, konfigurasi wizard yang terenkripsi memungkinkan proses dilanjutkan; setelah sukses file sementara dihapus.

1. Buat project Supabase baru dan siapkan koneksi role `postgres` project owner dari **Dashboard → Connect**. Installer membuat schema `app`, role runtime `algentra_api`, dan marker schema. Database harus baru dan belum memiliki schema `app`.
2. Pastikan domain portal aktif pada Cloudflare dan siapkan API token dengan izin **Account: Cloudflare Tunnel Edit** dan **Zone: DNS Edit + Zone Read**. Gunakan Windows Server 2016 atau lebih baru, atau Windows 10/11 64-bit, dengan koneksi internet keluar; IPv4 publik dan port masuk tidak dibutuhkan untuk portal. Installer memasang Go, Node.js, NSSM, cloudflared, dan MetaTrader 5 master serta template follower di `C:\Algentra`; semua cache/build tools berada di lokasi yang sama. Core API, portal, tiga binary copier, helper validasi login MT5, dan empat EA MQL5 dibangun otomatis. RDP tidak disentuh installer ini. [Cloudflare mencantumkan izin token untuk membuat Tunnel dan DNS](https://developers.cloudflare.com/cloudflare-one/networks/connectors/cloudflare-tunnel/get-started/create-remote-tunnel-api/); [MetaTrader mendukung pemasangan otomatis dengan `/auto` dan `/path`](https://www.metatrader5.com/en/terminal/help/start_advanced/installation).
3. Dari checkout repository, klik dua kali `install-one-click.cmd`. Setelah installer ini dipublikasikan pada branch `main`, file `.cmd` juga dapat dipakai sendiri: ia akan mengunduh source ke direktori sementara dan menghapusnya sesudah wizard ditutup. Launcher meminta elevasi Administrator dan menjaga jendela wizard tetap terbuka agar password awal dapat disimpan:

   ```powershell
   .\install-one-click.cmd
   ```

   Wizard meminta hostname portal, API token Cloudflare, dan URL owner Supabase. Project ref dibaca otomatis dari URL direct atau Session Pooler pada contoh di atas. Wizard juga meminta Gemini API key (opsional; model default `gemini-3.5-flash-lite`), Telegram API ID/hash dan nomor sumber (opsional), bot token/chat ID (opsional), kredensial akun MT5 master, serta Midtrans/SMTP bila fitur tersebut digunakan. Setelah portal siap, masukkan kode OTP Telegram dan pilih chat ID sumber yang diizinkan. Installer menerapkan [fresh production schema](infra/supabase/fresh-production.sql), membuat Tunnel/DNS, secret database/aplikasi, memasang semua service, menguji login broker dengan `MT5LoginProbe`, lalu mengisi chart master otomatis. Wizard meminta persetujuan terpisah untuk mengizinkan order live; default-nya nonaktif. Installer menolak schema lama yang marker-nya berbeda.
4. Simpan password admin yang ditampilkan setelah Core API siap. Installer menghapus password bootstrap dari environment setelah hash akun tersimpan di Supabase. Setelah login, buka Dashboard > Settings untuk melihat EA key dan copier worker key bila perlu. Installer sudah memasukkan EA key ke preset master dan memasang key worker ke file rahasia lokal. File environment rahasia berada di `C:\Algentra\secrets\core-api.env`; key tersimpan di folder `C:\Algentra\app\backend\data\integration-secrets`. Setelah selesai, verifikasi `https://portal.example.com`, `/health`, `/ready`, dan login admin.

### Menghapus instalasi

Jalankan `uninstall-one-click.cmd` dari folder repository, atau `C:\Algentra\app\uninstall-one-click.cmd` pada server, lalu ketik `UNINSTALL` saat diminta. Uninstaller menghentikan dan menghapus service serta Scheduled Task Algentra, kemudian menghapus seluruh `C:\Algentra`, termasuk data terminal MT5/follower, konfigurasi, log, dan secret. Pastikan data lokal yang diperlukan sudah disalin sebelum melanjutkan. Database Supabase, domain/nameserver, konfigurasi RDP, dan service atau aplikasi Windows lain tetap ada. Jika Tunnel Cloudflare dibuat oleh installer ini, uninstaller menawarkan penghapusan DNS CNAME dan Tunnel secara terpisah; siapkan API token dengan izin Tunnel Edit dan DNS Edit. Tunnel yang sudah ada sebelumnya atau state instalasi lama tanpa penanda kepemilikan dibiarkan.

Gemini key/model dan Telegram yang Anda masukkan saat instalasi disimpan otomatis di folder rahasia lokal; kredensial Telegram tidak disimpan lagi di file environment. Midtrans, rekening transfer, dan SMTP disimpan di `backend/data/integration-secrets/optional-integrations.json`, sehingga environment production tidak perlu memuat nilai-nilai itu. EA dan worker key tersedia di Dashboard > Settings. Cookie sesi login dibuat per sesi dan tidak perlu dikonfigurasi atau disalin. Kode OTP/2FA Telegram tetap dimasukkan saat wizard memintanya; wizard juga meminta chat ID sumber yang akan diizinkan. Jika Midtrans tidak diisi, wizard meminta detail rekening untuk transfer manual dan pembayaran tetap memerlukan review admin. Jika SMTP tidak diisi, pendaftaran dan reset password lewat email tidak aktif. Jangan menyalin Supabase owner URL ke environment runtime. Untuk update schema production berikutnya, siapkan SQL upgrade terpisah dan backup lebih dulu.

Detail service dan pemulihan ada di [docs/SERVICE.md](docs/SERVICE.md), development dan migrasi ada di [docs/DEVELOPMENT.md](docs/DEVELOPMENT.md), dan backup/restore ada di [docs/BACKUP.md](docs/BACKUP.md).

## 2. Pasang MT5 dan EA

Gunakan akun demo dahulu. Installer membuat MT5 master di `C:\Algentra\mt5\master`, template follower bersih di `C:\Algentra\mt5\follower-template`, mengompilasi EA, memvalidasi login master dengan `MT5LoginProbe`, lalu membuka chart XAUUSD M1 dengan `TelegramSignalEA`. Scheduled Task menjalankan MT5 pada sesi desktop akun Windows saat login.

### A. Copier managed dari portal (master memakai TelegramSignalEA)

TelegramSignalEA mengirim snapshot master ke Core API; worker managed meneruskan snapshot tersebut ke publisher Go dan membuat terminal follower dari profil yang dibayar. EA publisher master terpisah tidak diperlukan untuk mode ini. One-click installer memasang scheduled task worker, template MT5 follower, EA master, dan key worker secara otomatis. API key EA dimasukkan ke preset TelegramSignalEA oleh installer; key tetap dapat dilihat di Dashboard > Settings.

1. Wizard meminta nomor login, password, dan server broker akun master, lalu menjalankan `MT5LoginProbe` untuk memvalidasi login dan memilih chart emas M1. Terminal master dijadwalkan berjalan saat akun Windows masuk. EA memakai Core API lokal dan key yang sudah dibuat installer. Installer juga menyiapkan template follower bersih dan folder sinyal di `C:\Algentra`.
2. Keempat EA dikompilasi otomatis ke `.ex5` oleh installer; tidak perlu membuka MetaEditor untuk memasangnya pada host ini. File juga disalin ke folder aplikasi:
   - `mt5/experts/MT5FollowerCopyEA.ex5` pada terminal follower template.
   - `mt5/experts/Algentra/MT5LoginProbe.ex5` untuk validasi login managed.
3. Worker otomatis menambahkan WebRequest `http://127.0.0.1:8766` pada terminal follower setelah startup; template tidak perlu disetel manual. Task `Algentra Managed Copy Worker` dibuat dan mulai oleh one-click installer. Task memakai sesi Windows interaktif; setelah reboot, login ke Windows agar MT5 follower aktif. Source master default `telegram_signal`; installer tidak meminta path terminal master. Copier worker API key dapat disalin dari Dashboard > Settings.
4. User mendaftar, membuat pesanan, dan membayar dengan QRIS. Settlement otomatis membuat akun. User memasukkan kredensial broker follower serta batas lot di portal; worker memeriksa login, lalu user menyetujui aktivasi live. Uji alur dahulu dengan akun demo dan risk limit yang ditetapkan.

### B. Copier standalone dari posisi MT5 (publisher master + follower EA di host sendiri)

1. Buat satu instalasi MT5 portable terpisah untuk setiap follower di bawah `C:\Algentra\mt5\followers`, login ke akun yang benar lalu tutup terminal. Salin `MT5FollowerCopyEA.ex5` yang sudah disiapkan installer ke folder data MT5 masing-masing. MetaEditor hanya diperlukan jika ingin mengubah source EA.
2. Siapkan config lokal dari `apps/copier-worker/config/master.example.json`, lalu buat `akun*.json` dan `launcher.json` sesuai [panduan copier](apps/copier-worker/README.md). Tetapkan path terminal unik, folder sinyal yang sama, symbol mapping, mode lot, `dry_run`, dan batas `max_lot_per_order`, `max_posisi_terbuka`, `max_lot_total`.
3. Pasang `C:\Algentra\app\mt5\experts\MT5MasterPublisherEA.ex5` pada chart di terminal master yang login ke **akun master sumber posisi**. EA ini membaca posisi terbuka dan mengirim snapshot ke publisher Go lokal; EA tidak membuka order. Compile EA sudah dilakukan installer. Jika memakai `install_startup_task.ps1`, token publisher dibuat atau dimigrasikan ke file rahasia lokal dan ditampilkan untuk diisi pada input `PublisherToken`; token tidak disimpan permanen di environment Windows. Allow-list `http://127.0.0.1:8765` pada terminal master dan `http://127.0.0.1:8766` pada terminal follower.
4. Mulai dari root release: `apps\copier-worker\scripts\start-all.ps1`. Task opsional saat login dapat dipasang melalui `apps\copier-worker\scripts\install_startup_task.ps1`. Jangan jalankan publisher master kedua bila managed worker atau launcher sudah menjalankannya.

Pasang EA publisher ini hanya jika ingin menyalin posisi yang dibuka di MT5 master secara langsung (standalone/`master_source: publisher_ea`). Untuk managed copy yang menggunakan sinyal Telegram, pasang `TelegramSignalEA.mq5` di terminal master; `MT5MasterPublisherEA.mq5` tidak dipakai karena worker menerima snapshot dari Core API.

### C. Telegram Signal EA (mengirim order dari sinyal Telegram)

1. Gunakan terminal MT5 khusus pada sesi interaktif dan pasang `TelegramSignalEA.ex5` yang sudah dikompilasi installer pada chart simbol yang sesuai.
2. Bila EA berjalan pada Windows host yang sama dengan API, set `ServerURL` ke `http://127.0.0.1:8000`; jika berjalan di host berbeda gunakan origin HTTPS khusus yang dapat dijangkau. Allow-list URL itu pada **Tools → Options → Expert Advisors → Allow WebRequest**. Batasi akses jaringan ke hostname API melalui firewall/reverse proxy dan pertahankan `X-API-Key` EA yang kuat.
3. Isi input `ApiKey` dengan nilai `EA_API_KEY` backend. Atur watchlist, symbol mapping, batas/default SL/TP, volume dan opsi trading; cocokkan pula whitelist grup Telegram pada halaman admin `/telegram-setup`.
4. Verifikasi pada akun demo dan pantau log Experts/Journal. EA ini dapat mengirim order live dan backend tidak menyediakan dry-run untuk eksekusi EA tersebut. Kill switch menghentikan distribusi signal baru, tetapi tidak menutup posisi terbuka.

### D. Copier EA mandiri milik client

Portal client menyediakan EA `MT5FollowerCopyEA.ex5` yang dikompilasi installer, serta source `.mq5` sebagai pilihan. Client menyalin `.ex5` ke folder `MQL5/Experts`, allow-list origin API pada MT5, memasukkan token dan parameter risiko pada input EA, lalu mengaktifkan Algo Trading. Tidak perlu memasang MetaEditor atau compile EA pada terminal client. Token hanya ditampilkan saat dibuat/dirotasi dan harus diperlakukan sebagai rahasia. Lihat [docs/OPERATIONS.md](docs/OPERATIONS.md) dan panduan akun copier untuk instruksi yang dilihat client.

## 3. Start, stop, dan status operasional

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

## 4. Backup dan update aplikasi/service

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
6. Untuk update portal, jalankan `npm ci` dan `npm run build` di `apps/web`, lalu restart service `AlgentraWeb` setelah build berhasil.

### Rollback

Hentikan trafik/update yang gagal, pulihkan binary dan frontend versi sebelumnya, lalu cek schema compatibility. Pulihkan database hanya jika diperlukan setelah menghitung transaksi sejak backup; rekonsiliasi order, posisi, dan deal lewat riwayat broker. Prosedur lengkap ada di [docs/SERVICE.md](docs/SERVICE.md) dan [docs/BACKUP.md](docs/BACKUP.md).

## 5. Checklist setelah install/update

- [ ] `/health` sukses dan `/ready` melaporkan database siap.
- [ ] Portal `AlgentraWeb`, API `AlgentraCoreApi`, dan `AlgentraCloudflared` berjalan; login dan request portal berhasil.
- [ ] Telegram/Gemini/email/payment sesuai integrasi yang diaktifkan; Midtrans sandbox diuji sebelum production.
- [ ] Untuk copier managed: TelegramSignalEA mengirim snapshot master; worker meneruskan feed dan follower membaca perubahan posisi.
- [ ] Terminal follower memakai simbol dan parameter risiko yang benar; status broker/heartbeat sehat.
- [ ] Uji pertama menggunakan akun demo. Trading live hanya setelah persetujuan operator/client dan review risk limit.
- [ ] Backup, commit, versi binary/EA, hasil smoke test, dan waktu deployment tercatat.

## Referensi

- [Instalasi service dan HTTPS](docs/SERVICE.md)
- [Fitur portal, EA, dan batas operasi](docs/OPERATIONS.md)
- [Copier standalone dan managed](apps/copier-worker/README.md)
- [Backup dan restore](docs/BACKUP.md)
- [Prosedur lokal development](docs/DEVELOPMENT.md)
- [Kontrak OpenAPI dan tipe frontend](packages/contracts/README.md)
