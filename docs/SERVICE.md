# Deploy dan operasi service production

Panduan ini mencakup installer Windows satu kali, Core API/portal/Cloudflare Tunnel services, worker managed copy, smoke check rilis, dan jeda darurat. Persiapan ada di [README](../README.md); development ada di [DEVELOPMENT.md](./DEVELOPMENT.md), backup/restore ada di [BACKUP.md](./BACKUP.md).

## Windows Server dan Core API

Installer utama memasang Go, Node.js, NSSM, cloudflared, dan dua terminal MetaTrader 5 di `C:\Algentra`; mengompilasi EA MQL5, membangun Core API, portal, serta tiga binary copier; membuat schema fresh di Supabase; mendaftarkan tiga service aplikasi dan managed copier task. Semua dependency, compiler, cache, source, template MT5, dan output build berada di C:. Installer memvalidasi login broker dengan MT5LoginProbe, lalu mengisi chart master dan preset EA otomatis; MT5 berjalan pada sesi desktop interaktif akun Windows dan task dijalankan saat akun itu login. Siapkan project Supabase kosong, domain aktif di Cloudflare, dan API token Cloudflare dengan izin **Account: Cloudflare Tunnel Edit** dan **Zone: DNS Edit + Zone Read**. Wizard membuat Tunnel remote-managed, route ke `http://127.0.0.1:3000`, dan record DNS CNAME secara otomatis; tidak perlu membuat Tunnel di dashboard lebih dahulu. Server hanya perlu koneksi keluar; bila firewall jaringan membatasi egress, izinkan TCP dan UDP port `7844` ke Cloudflare Tunnel. RDP dikelola terpisah dan tidak diubah installer portal. Klik dua kali launcher dari checkout repository; setelah installer dipublikasikan ke branch `main`, file `.cmd` tunggal juga dapat mengunduh source sementara. Launcher meminta elevasi Administrator:

API yang dipakai adalah [create tunnel](https://developers.cloudflare.com/api/resources/zero_trust/subresources/tunnels/subresources/cloudflared/methods/create/), [set tunnel ingress configuration](https://developers.cloudflare.com/api/resources/zero_trust/subresources/tunnels/subresources/cloudflared/subresources/configurations/methods/update/), serta [create DNS record](https://developers.cloudflare.com/api/resources/dns/subresources/records/). Token API dienkripsi DPAPI sementara agar wizard dapat dilanjutkan setelah gagal, lalu dihapus setelah Tunnel berhasil dibuat; token connector Tunnel yang dibuat Cloudflare disimpan pada file ACL terbatas untuk Windows service.

```powershell
.\install-one-click.cmd
```

Installer meminta hostname publik, API token Cloudflare, dan `DATABASE_ADMIN_URL` role `postgres` lewat prompt tersembunyi. Pilih URL Supabase langsung atau session pooler port 5432; transaction pooler port 6543 tidak cocok dengan koneksi persisten `pgxpool`. Project ref dibaca otomatis dari URL direct atau Session Pooler Supabase yang didukung. Sebelum build aplikasi atau membuat schema, installer memeriksa koneksi Supabase, project ref, dan marker schema tanpa mengubah database. Wizard membuat role runtime password acak dan token Tunnel, menyimpan kredensial server di `C:\Algentra\secrets` dengan ACL terbatas, lalu menampilkan password admin awal setelah Core API siap. Password bootstrap dihapus dari environment setelah hash akun tersimpan di database. Core API membuat application secret, EA API key, dan copier worker key otomatis pada folder rahasia lokal; dua API key terlihat pada Dashboard > Settings. Gemini cukup memakai API key dan model; wizard meminta keduanya dan menyimpan pengaturan di `gemini.json`. Midtrans, rekening transfer, dan SMTP disimpan di `optional-integrations.json`; URL notifikasi Midtrans dibentuk otomatis dari hostname portal. File integrasi hanya dapat dibaca admin sistem dan Core API. Task worker dibuat otomatis; key runtime disimpan pada file terpisah ber-ACL terbatas untuk sesi Windows interaktif dan hanya dimasukkan ke environment proses saat worker berjalan. Jangan jalankan installer ini pada database yang sudah memiliki schema `app`.

Installer menghapus environment aplikasi yang diwarisi dari akun/mesin sebelum build. Saat upgrade, file `.env` Next.js yang dikenali dipindahkan ke folder backup ber-ACL terbatas di `C:\Algentra\secrets\web-env-backup-*`; file tersebut tidak dipakai service production.

Installer memverifikasi portal lokal dan endpoint HTTPS `/ready` sebelum melaporkan selesai. Jika pemeriksaan gagal, periksa hostname dan konfigurasi Tunnel yang dibuat wizard, status konektor, serta log; environment, schema, dan state Cloudflare yang sudah dibuat akan dipakai kembali.

### Uninstall one-click

Jalankan `uninstall-one-click.cmd` dari folder repository, atau `C:\Algentra\app\uninstall-one-click.cmd` pada server, dengan akun Administrator. Ketik `UNINSTALL` untuk mengonfirmasi penghapusan semua `C:\Algentra`. Proses ini menghapus service `AlgentraCoreApi`, `AlgentraWeb`, `AlgentraCloudflared`, task `Algentra MT5 Master` dan `Algentra Managed Copy Worker`, lalu folder instalasi yang berisi MT5 master/follower, source/build, log, konfigurasi, dan secret. Salin dahulu file lokal yang masih diperlukan. Supabase, domain/nameserver, RDP, serta service/task dan file di luar `C:\Algentra` tidak dihapus. Tunnel/DNS hanya ditawarkan untuk dihapus jika state menandai Tunnel dibuat oleh installer; API token diminta tersembunyi saat cleanup Cloudflare dipilih. Jika token dikosongkan atau cleanup gagal, resource Cloudflare dibiarkan. State lama/adopsi Tunnel tanpa penanda kepemilikan juga dibiarkan.

Service yang dibuat:

- `AlgentraCoreApi`: Go API di `127.0.0.1:8000`.
- `AlgentraWeb`: Next.js di `127.0.0.1:3000`.
- `AlgentraCloudflared`: tunnel keluar menuju hostname publik Cloudflare.

Data Telegram lokal berada di `backend\data`; installer memberi API service hak tulis. Sesudah instalasi, cek status dan health:

```powershell
Get-Service AlgentraCoreApi, AlgentraWeb, AlgentraCloudflared
Invoke-RestMethod http://127.0.0.1:8000/health
Invoke-RestMethod http://127.0.0.1:8000/ready
Invoke-WebRequest https://portal.example.com
```

### Start, stop, dan cek service

Mulai, hentikan, atau periksa layanan portal:

```powershell
Start-Service AlgentraCoreApi
Start-Service AlgentraWeb
Start-Service AlgentraCloudflared
Get-Service AlgentraCoreApi, AlgentraWeb, AlgentraCloudflared
Stop-Service AlgentraCloudflared
Stop-Service AlgentraWeb
Stop-Service AlgentraCoreApi
Invoke-WebRequest http://127.0.0.1:8000/health
Invoke-WebRequest http://127.0.0.1:8000/ready
```

Managed copier adalah Scheduled Task, bukan Windows Service. Setelah task didaftarkan oleh installer, mulai/hentikan dan periksa statusnya dengan:

```powershell
Start-ScheduledTask -TaskName 'Algentra Managed Copy Worker'
Get-ScheduledTask -TaskName 'Algentra Managed Copy Worker' |
  Select-Object TaskName, State
Get-ScheduledTaskInfo -TaskName 'Algentra Managed Copy Worker' |
  Select-Object LastRunTime, LastTaskResult

Stop-ScheduledTask -TaskName 'Algentra Managed Copy Worker'
```

Task berjalan pada sesi interaktif akun Windows yang mendaftarkannya; login ke akun worker setelah reboot. `Start-ScheduledTask` tidak memasang taskâ€”jalankan installer managed worker lebih dulu. Worker dapat mengirim order live untuk profil aktif, sehingga pastikan demo atau tidak ada job live yang belum ditinjau sebelum start.

## Routing portal/API

Cloudflare Tunnel mengirim hostname publik ke Next.js lokal (`http://127.0.0.1:3000`); Next rewrite meneruskan `/api/core/*` ke Go `/v1/*` dan API kompatibilitas `/api/*` ke Go, setelah route pencarian broker Next.js. Health/readiness juga diteruskan ke Go. Port 3000/8000 hanya bind loopback; tidak ada port masuk aplikasi yang dibuka di firewall.

Installer portal tidak mengubah Remote Desktop, aturan firewall RDP, atau konfigurasi akses jarak jauh Windows.

## Managed copier worker

Worker dijalankan sebagai Scheduled Task interaktif, bukan Windows Service. MT5 memerlukan sesi desktop pengguna; service Session 0 tidak dipakai untuk worker. Siapkan MT5 master yang sudah login, folder signal, terminal template follower bersih berisi `terminal64.exe`, dan binary Go worker beserta EA yang sudah dikompilasi.

Form akun mencari nama broker/server melalui direktori pihak ketiga `mt5.mtapi.io`. Worker mencari ulang server terpilih dan menggunakan alamat akses yang dipublikasikan direktori, dengan `Config/servers.dat` lokal sebagai fallback bila resolver tidak tersedia. Pencarian mengirim nama server publik saja; login dan password tidak dikirim ke layanan pencarian. Nomor login tidak menunjukkan server akun, jadi pelanggan harus memilih server yang cocok dengan detail akun dari brokernya. Untuk memakai resolver lain, set `MT5_BROKER_DIRECTORY_URL` ke endpoint HTTPS yang kompatibel di lingkungan web dan worker.

1. Pada instalasi one-click, key worker dibuat di `backend\data\integration-secrets\copier-worker.json`, ditampilkan pada Dashboard > Settings, dan disimpan installer pada file rahasia lokal untuk akun task. `worker_api_key_env` pada `config\managed-worker.json` hanya berisi nama variabel `COPIER_WORKER_API_KEY`, bukan nilai key. Jangan memuat seluruh file environment Core API ke worker.
2. One-click installer membuat task worker otomatis. Untuk host worker Windows terpisah, jalankan installer task setelah menyiapkan binary dan terminal MT5:

   ```powershell
   .\apps\copier-worker\scripts\install_managed_worker_task.ps1 `
     -ProjectRoot (Get-Location).Path `
     -ApiBaseUrl 'https://portal.example.com' `
     -TerminalTemplateDir 'C:\Algentra\mt5\follower-template' `
     -SignalDirectory 'C:\Algentra\signals'
   ```

   Ganti URL dan path contoh dengan nilai deployment sebenarnya. Installer memvalidasi endpoint/key, menyiapkan konfigurasi dan Scheduled Task, lalu memulai worker. Task berjalan saat akun Windows login; sesudah reboot, login kembali ke akun worker agar MT5 dan task aktif.
3. Pantau `logs\managed_worker.log` dan `logs\managed_worker-task-error.log`. API menandai worker online setelah polling job berhasil dan menerima heartbeat. Worker yang online dapat mengeksekusi job managed aktif secara live; uji awal pada akun demo dan jangan mulai task dengan job live yang tidak ditinjau.

TelegramSignalEA pada terminal master mengirim snapshot ke Core API. Worker managed meneruskan snapshot ke publisher Go lokal, membuat terminal MT5 follower terisolasi, memeriksa kredensial broker, lalu menjalankan EA follower melalui feed lokal. Mode `telegram_signal` tidak memerlukan MT5MasterPublisherEA pada master. Klien harus menyimpan kredensial/risk dan menyetujui aktivasi live dari portal. Detail tambahan ada di [panduan copier-worker](../apps/copier-worker/README.md#managed-copy-dari-portal-client).

## Rilis production

Setiap rilis memerlukan maintenance window. Core API dijalankan sebagai satu binary Go dan tidak menerapkan migrasi database otomatis.

1. Uji kandidat pada staging/akun demo; jalankan tes sesuai [panduan development](./DEVELOPMENT.md). Tinjau perubahan kode, dependency, dan versi source EA.
2. Pastikan tidak ada order sedang diproses. Aktifkan kill switch untuk menghentikan klaim signal baru; tinjau akun, simbol, EA, pending order, posisi, dan managed follower.
3. Buat dan verifikasi backup sesuai [BACKUP.md](./BACKUP.md).
4. Hentikan Core API service; deploy binary Go baru tanpa mengganti file environment, database, `backend\data`, config MT5, state worker, atau folder terminal. Tidak ada runtime Python yang perlu dipasang. Mulai satu service.
5. Uji `/health` dan `/ready`, lalu login admin dan periksa dashboard, Telegram, EA, Cloudflare Tunnel, worker, serta alur portal yang berubah. `/ready` hanya memeriksa database; ia tidak memastikan worker/EA/Telegram sehat.
6. Untuk EA Telegram, uji terlebih dahulu pada akun demo. Backend tidak mempunyai dry-run untuk order EA tersebut.
7. Lepas kill switch hanya setelah operator menyetujui trading live. Managed copy harus online, login follower valid, batas risiko diperiksa, dan client menyetujui **Mulai copy trading (live)**.

## Jeda darurat dan pemecahan masalah

Kill switch menghentikan distribusi signal baru ke EA Telegram, tetapi tidak menutup posisi/pending order dan tidak menjeda managed copy. Menjeda profil managed menghentikan salinan berikutnya, tetapi posisi follower yang sudah terbuka tetap ada. Kelola eksposur langsung dari MT5/broker.

Jika smoke test gagal, aktifkan kill switch dan hentikan service/worker terkait. Pulihkan kode rilis sebelumnya, uji kembali Tunnel/dashboard/terminal, lalu pulihkan database hanya jika diperlukan dan setelah menilai transaksi pasca-backup. Rekonsiliasi order, deal, posisi, dan pending order dengan riwayat broker sebelum membuka trafik lagi.

Untuk 502, periksa `https://portal.example.com/health`, status `AlgentraCloudflared`, dan log di `C:\Algentra\app\logs\cloudflared`. Pastikan konfigurasi Tunnel mengarah ke `http://127.0.0.1:3000`. Jika `/health` berhasil tetapi `/ready` HTTP 503, periksa `DATABASE_URL`, ACL akun service, koneksi Supabase, dan kapasitas database. Jika API siap tetapi halaman tidak merespons, periksa `AlgentraWeb` serta koneksi keluar server. Pastikan pasangan Telegram bot terisi bersama-sama. Pantau rotasi log NSSM dan Task Scheduler worker. Alert terpusat belum disediakan repo.
