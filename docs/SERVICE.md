# Deploy dan operasi service production

Panduan ini mencakup Core API Windows Service, Cloudflare Tunnel/Vercel, worker managed copy, smoke test rilis, dan jeda darurat. Persiapan database production dan konfigurasi utama ada di [README](../README.md); development ada di [DEVELOPMENT.md](./DEVELOPMENT.md), backup/restore ada di [BACKUP.md](./BACKUP.md).

## Windows Server dan Core API

Gunakan Windows Server terkelola, Python 3.12 64-bit, NSSM, Cloudflare Tunnel, koneksi TLS ke Supabase production, dan akun service non-admin. Project production harus sudah memiliki schema/revision yang cocok; service tidak menjalankan DDL atau migrasi.

1. Letakkan release di path tetap dan pasang dependency:

   ```powershell
   py -3.12 -m venv .venv
   .\.venv\Scripts\python.exe -m pip install --upgrade pip
   .\.venv\Scripts\python.exe -m pip install -r requirements.txt
   .\.venv\Scripts\python.exe -m pip install -e .\apps\copier-worker
   ```

2. Buat `D:\AlgentraSecure\core-api.env` di luar repository dan batasi ACL NTFS. Isi variabel production berdasarkan `.env.example`; jangan menyalin `.env.local` development. Jangan isi `MIGRATION_DATABASE_URL` atau `SUPABASE_DEV_PROJECT_REF`. File tersebut berisi plaintext dan hanya boleh dibaca administrator, SYSTEM, serta akun service. Lindungi dan pertahankan `backend\data`.
3. Pasang Core API melalui PowerShell administrator:

   ```powershell
   .\infra\windows\install-core-api-service.ps1 `
     -EnvironmentFile 'D:\AlgentraSecure\core-api.env' `
     -NssmPath 'C:\Tools\nssm\win64\nssm.exe'
   ```

   Service `AlgentraCoreApi` memakai akun virtual terbatas `NT SERVICE\AlgentraCoreApi`, bind ke `127.0.0.1:8000`, berjalan satu worker tanpa reload, dan mengatur restart otomatis. Core API hanya melayani API JSON (`/api/*`, `/v1/*`), `/health`, `/ready`, `/downloads/*`, dan websocket — ia tidak menyajikan halaman web apa pun; seluruh web disajikan oleh Next.js di Vercel. Setelah konfigurasi siap:

   ```powershell
   Start-Service AlgentraCoreApi
   Invoke-WebRequest http://127.0.0.1:8000/health
   Invoke-WebRequest http://127.0.0.1:8000/ready
   ```

   Jangan jalankan development launcher atau backend kedua pada production.

### Start, stop, dan cek service

Jalankan Core API setelah pemasangan atau reboot:

```powershell
Start-Service AlgentraCoreApi
Get-Service AlgentraCoreApi
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

Task berjalan pada sesi interaktif akun Windows yang mendaftarkannya; login ke akun worker setelah reboot. `Start-ScheduledTask` tidak memasang task—jalankan installer managed worker lebih dulu. Worker dapat mengirim order live untuk profil aktif, sehingga pastikan demo atau tidak ada job live yang belum ditinjau sebelum start.

## Cloudflare Tunnel dan Vercel

Pasang named Cloudflare Tunnel sebagai service/task terpisah dengan ingress ke `http://127.0.0.1:8000`. Tunnel ini hanya mengekspos Core API (bukan web); jangan buka port 8000 ke internet atau bind backend ke interface publik. Aktifkan TLS/HSTS di Cloudflare.

Web disajikan sepenuhnya dan hanya oleh Next.js di Vercel (`apps/web`); tidak ada web kedua di Windows Server.

Pada Vercel:

1. Import repository dan set **Root Directory** ke `apps/web`; gunakan `npm ci` dan `npm run build`.
2. Set environment server-only `CORE_API_ORIGIN` ke origin HTTPS Core API di belakang Cloudflare Access, `CF_ACCESS_CLIENT_ID`/`CF_ACCESS_CLIENT_SECRET` ke service token khusus Vercel, dan `BFF_HMAC_SECRET` sama persis dengan secret pada Core API.
3. Deploy Preview dahulu. Uji policy Access, login, dashboard, dan request API; baru deploy Production dan pasang domain. Setiap perubahan environment memerlukan deployment baru.

Core API tetap berkomunikasi langsung dengan Postgres; browser tidak menerima secret atau mengakses Supabase.

## Managed copier worker

Worker dijalankan sebagai Scheduled Task interaktif, bukan Windows Service. MT5 memerlukan sesi desktop pengguna; service Session 0 tidak dipakai untuk worker. Siapkan MT5 master yang sudah login, folder signal, terminal template follower bersih berisi `terminal64.exe`, dan Python 3.12 64-bit.

1. Pastikan `COPIER_WORKER_API_KEY` sama pada environment backend dan akun Windows worker. Nilai key hanya disimpan di secure environment/User/Machine; `worker_api_key_env` pada `config\managed-worker.json` harus berisi nama variabel `COPIER_WORKER_API_KEY`, bukan nilai key. Jangan memuat seluruh file environment Core API ke worker.
2. Dari PowerShell pada root release jalankan installer dan masukkan key hanya pada prompt tersembunyi:

   ```powershell
   .\apps\copier-worker\scripts\install_managed_worker_task.ps1 `
     -ProjectRoot (Get-Location).Path `
     -PythonPath (Join-Path (Get-Location).Path '.venv\Scripts\python.exe') `
     -ApiBaseUrl 'https://api.example.com' `
     -TerminalTemplateDir 'D:\MT5Templates\CleanFollower' `
     -MasterTerminalPath 'C:\Program Files\MetaTrader 5\terminal64.exe' `
     -SignalDirectory 'C:\Algentra\signals'
   ```

   Ganti URL dan path contoh dengan nilai deployment sebenarnya. Installer memvalidasi endpoint/key, menyiapkan dependency dan Scheduled Task, lalu memulai worker. Task berjalan saat akun Windows login; sesudah reboot, login kembali ke akun worker agar MT5 dan task aktif.
3. Pantau `logs\managed_worker.log` dan `logs\managed_worker-task-error.log`. API menandai worker online setelah polling job berhasil dan menerima heartbeat. Worker yang online dapat mengeksekusi job managed aktif secara live; uji awal pada akun demo dan jangan mulai task dengan job live yang tidak ditinjau.

Worker membuat terminal MT5 terisolasi, memeriksa kredensial broker, lalu menjalankan copier Python managed—bukan memasang EA MQL5 pada terminal tersebut. Klien harus menyetujui aktivasi live dari portal. Detail tambahan ada di [panduan copier-worker](../apps/copier-worker/README.md#managed-copy-dari-portal-client).

## Rilis production

Setiap rilis memerlukan maintenance window. Uvicorn berjalan satu proses dan tidak menerapkan migrasi otomatis.

1. Uji kandidat pada staging/akun demo; jalankan tes sesuai [panduan development](./DEVELOPMENT.md). Tinjau perubahan kode, dependency, dan versi source EA.
2. Pastikan tidak ada order sedang diproses. Aktifkan kill switch untuk menghentikan klaim signal baru; tinjau akun, simbol, EA, pending order, posisi, dan managed follower.
3. Buat dan verifikasi backup sesuai [BACKUP.md](./BACKUP.md).
4. Hentikan Core API service; deploy kode tanpa mengganti file environment, database, `backend\data`, config MT5, state worker, atau folder terminal. Pasang dependency yang diperlukan dan mulai satu service.
5. Uji `/health` dan `/ready`, lalu login admin dan periksa dashboard, Telegram, EA, tunnel, worker, serta alur portal yang berubah. `/ready` hanya memeriksa database; ia tidak memastikan worker/EA/Telegram sehat.
6. Untuk EA Telegram, uji terlebih dahulu pada akun demo. Backend tidak mempunyai dry-run untuk order EA tersebut.
7. Lepas kill switch hanya setelah operator menyetujui trading live. Managed copy harus online, login follower valid, batas risiko diperiksa, dan client menyetujui **Mulai copy trading (live)**.

## Jeda darurat dan pemecahan masalah

Kill switch menghentikan distribusi signal baru ke EA Telegram, tetapi tidak menutup posisi/pending order dan tidak menjeda managed copy. Menjeda profil managed menghentikan salinan berikutnya, tetapi posisi follower yang sudah terbuka tetap ada. Kelola eksposur langsung dari MT5/broker.

Jika smoke test gagal, aktifkan kill switch dan hentikan service/worker terkait. Pulihkan kode rilis sebelumnya, uji kembali origin/dashboard/tunnel/terminal, lalu pulihkan database hanya jika diperlukan dan setelah menilai transaksi pasca-backup. Rekonsiliasi order, deal, posisi, dan pending order dengan riwayat broker sebelum membuka trafik lagi.

Untuk Cloudflare 502, periksa `/health` pada origin terlebih dahulu. Jika `/health` berhasil tetapi `/ready` HTTP 503, periksa `DATABASE_URL`, ACL akun service, koneksi Supabase, dan kapasitas database. Jika keduanya berhasil, periksa status/log `cloudflared`, DNS hostname, dan ingress. Pastikan pasangan Telegram bot terisi bersama-sama. Rotasi log NSSM Core API adalah 10 MB per file; pantau kapasitas log dan Task Scheduler worker. Alert terpusat belum disediakan repo.
