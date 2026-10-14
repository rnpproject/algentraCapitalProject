# Algentra Capital — instalasi production

Panduan ini khusus untuk memasang Algentra pada production. Targetnya adalah Core API FastAPI di Windows Server, Supabase PostgreSQL, Cloudflare Tunnel, dan web client di Vercel. Aplikasi tidak menjalankan migrasi schema saat startup; DBA harus menyiapkan schema production yang sesuai sebelum service dirilis.

Dokumentasi dipisahkan menurut tugas:

- [Development lokal dan Supabase development](docs/DEVELOPMENT.md)
- [Backup dan pemulihan database](docs/BACKUP.md)
- [Windows Service, Cloudflare/Vercel, managed worker, dan rilis](docs/SERVICE.md)
- [Operasi portal, EA, API, dan customer service](docs/OPERATIONS.md)

## Prasyarat production

- Windows Server terkelola, Python 3.12 64-bit, NSSM, dan akses administrator untuk instalasi.
- Core API di Windows hanya melayani API JSON; seluruh halaman web disajikan oleh Next.js di Vercel — tidak ada web yang disajikan dari Windows Server.
- Project Supabase production terpisah dengan TLS dan role runtime `algentra_api`; jangan gunakan Supabase `anon` atau `service_role`.
- DNS/domain dan named Cloudflare Tunnel menuju Core API lokal di `127.0.0.1:8000`. Jangan buka port API secara langsung ke internet.
- Schema database production sudah diprovisikan pada revision yang cocok dengan rilis. Ikuti prosedur snapshot schema dan pemeriksaan marker pada [panduan development](docs/DEVELOPMENT.md#menyiapkan-schema-awal-production); jalankan provisioning satu kali sebagai DBA pada database production kosong/awal, bukan pada database aktif.
- File environment production disimpan di luar repository dan dibatasi dengan ACL NTFS. Jangan menyalin `.env.local` development ke server.

## Persiapan release

Checkout release yang sudah diuji pada staging, lalu dari PowerShell di root repository pasang dependency:

```powershell
py -3.12 -m venv .venv
.\.venv\Scripts\python.exe -m pip install --upgrade pip
.\.venv\Scripts\python.exe -m pip install -r requirements.txt
.\.venv\Scripts\python.exe -m pip install -e .\apps\copier-worker
```

Buat file environment di luar repository, misalnya `D:\AlgentraSecure\core-api.env`. Atur minimal:

- `APP_SECRET_KEY` acak minimal 32 karakter.
- `ADMIN_PASSWORD` minimal 12 karakter.
- `APP_ENV=production` pada environment proses service (bukan hanya di file).
- `COOKIE_SECURE=true`, `SUPABASE_PROJECT_REF`, dan `DATABASE_URL` role `algentra_api` dengan TLS.
- `BFF_HMAC_SECRET` acak minimal 32 karakter, sama pada Core API dan Vercel.
- `EA_API_KEY` bila EA Telegram dipakai.

Jangan isi `MIGRATION_DATABASE_URL` atau `SUPABASE_DEV_PROJECT_REF` pada Core API production. Jangan ganti `APP_SECRET_KEY` tanpa rencana pemulihan password broker managed-copy dan data terenkripsi.

Fitur opsional:

- Pembayaran manual: isi `PAYMENT_MANUAL_INSTRUCTIONS` dengan rekening dan nominal resmi yang sudah diverifikasi; QRIS tetap nonaktif. Review pembayaran memerlukan `TELEGRAM_BOT_TOKEN` dan `TELEGRAM_ADMIN_CHAT_ID`.
- Managed copy: set `COPIER_WORKER_API_KEY` acak minimal 32 karakter. Nilai yang sama dipasang pada backend dan akun Windows worker; jangan taruh nilainya pada file JSON worker.
- Gemini/Telegram/SMTP: isi hanya jika fitur terkait dipakai. `GEMINI_API_KEY` dan `GEMINI_CS_API_KEY` harus berbeda. `TELEGRAM_API_ID`/`TELEGRAM_API_HASH` serta token/chat ID bot pembayaran harus diisi berpasangan.

## Pasang service dan aktifkan trafik

Langkah berikut memasang dan menjalankan Core API serta managed copier pada Windows Server. Gunakan PowerShell **Run as Administrator** untuk instalasi Core API. Detail Cloudflare/Vercel, rilis, dan troubleshooting ada di [panduan service production](docs/SERVICE.md).

### 1. Siapkan file environment Core API

Contoh konfigurasi production ada di [core-api.production.env.example](infra/windows/core-api.production.env.example). Salin isinya ke file di luar repo, lalu isi semua nilai required dan hanya nilai optional untuk fitur yang digunakan. Jangan menaruh nilai rahasia nyata pada file contoh:

```powershell
New-Item -ItemType Directory -Path 'D:\AlgentraSecure' -Force | Out-Null
icacls.exe 'D:\AlgentraSecure' /inheritance:r /grant 'SYSTEM:(OI)(CI)F' 'BUILTIN\Administrators:(OI)(CI)F'
New-Item -ItemType File -Path 'D:\AlgentraSecure\core-api.env' -Force | Out-Null
notepad.exe 'D:\AlgentraSecure\core-api.env'
```

Gunakan contoh production di atas (bukan `.env.example`, yang khusus development). `start-core-api.ps1` menetapkan `APP_ENV=production` untuk proses service. Jangan isi `MIGRATION_DATABASE_URL` atau `SUPABASE_DEV_PROJECT_REF`, dan jangan menyalin `.env.local` development ke server.

### 2. Pasang dan start Core API Windows Service

Pastikan release sudah memiliki virtual environment dan dependency dari [Persiapan release](#persiapan-release), database production sudah diprovisikan, serta NSSM tersedia. Jalankan installer berikut di PowerShell administrator dari root repository; sesuaikan lokasi `nssm.exe`:

```powershell
.\infra\windows\install-core-api-service.ps1 `
  -ProjectRoot (Get-Location).Path `
  -EnvironmentFile 'D:\AlgentraSecure\core-api.env' `
  -NssmPath 'C:\Tools\nssm\win64\nssm.exe'
```

Installer mendaftarkan service `AlgentraCoreApi` dengan akun virtual terbatas, ACL untuk environment/data/log, dan restart otomatis. **Installer belum men-start service.** Setelah instalasi sukses:

```powershell
Start-Service AlgentraCoreApi
Get-Service AlgentraCoreApi
Invoke-WebRequest http://127.0.0.1:8000/health
Invoke-WebRequest http://127.0.0.1:8000/ready
```

`/health` menguji proses API dan `/ready` menguji koneksi database. Keduanya harus sukses sebelum trafik publik diarahkan ke origin.

### 3. Pasang dan jalankan managed copier (opsional)

Managed copier bukan Windows Service. Installer membuat Scheduled Task interaktif karena MT5 perlu sesi desktop user. Jalankan installer pada akun Windows yang menjalankan MT5; siapkan terminal master yang sudah login, folder sinyal, dan folder template follower bersih yang berisi `terminal64.exe`. Ganti URL dan path contoh dengan nilai server:

```powershell
.\apps\copier-worker\scripts\install_managed_worker_task.ps1 `
  -ProjectRoot (Get-Location).Path `
  -PythonPath (Join-Path (Get-Location).Path '.venv\Scripts\python.exe') `
  -ApiBaseUrl 'https://api.example.com' `
  -TerminalTemplateDir 'D:\MT5Templates\CleanFollower' `
  -MasterTerminalPath 'C:\Program Files\MetaTrader 5\terminal64.exe' `
  -SignalDirectory 'C:\Algentra\signals'
```

Masukkan `COPIER_WORKER_API_KEY` yang sama dengan backend hanya pada prompt tersembunyi. Di `apps\copier-worker\config\managed-worker.json`, `worker_api_key_env` harus berisi nama variabel `COPIER_WORKER_API_KEY`, bukan nilai key. Installer menguji koneksi/key lalu mendaftarkan dan menjalankan task.

Task berjalan saat akun Windows tersebut login. Sesudah reboot, login kembali ke akun worker; bila task sudah terdaftar tetapi berhenti, jalankan dan periksa status:

```powershell
Start-ScheduledTask -TaskName 'Algentra Managed Copy Worker'
Get-ScheduledTask -TaskName 'Algentra Managed Copy Worker' |
  Select-Object TaskName, State
Get-ScheduledTaskInfo -TaskName 'Algentra Managed Copy Worker' |
  Select-Object LastRunTime, LastTaskResult
```

Worker online setelah berhasil polling backend. Log aplikasi ada di `logs\managed_worker.log`, error startup awal ada di `logs\managed_worker-task-error.log`. Pastikan tidak ada job live yang tidak ditinjau sebelum menjalankan worker; job managed aktif dapat mengirim order live. Untuk menghentikan task tanpa menghapus instalasinya, gunakan `Stop-ScheduledTask -TaskName 'Algentra Managed Copy Worker'`.

### 4. Aktifkan akses publik

Pasang named Cloudflare Tunnel ke origin `http://127.0.0.1:8000`, lalu konfigurasi Cloudflare Access dan environment Vercel sesuai [panduan service production](docs/SERVICE.md#cloudflare-tunnel-dan-vercel). Jangan membuka port 8000 langsung ke internet.

## Checklist sebelum production live

- `/health` dan `/ready` merespons sukses dari origin.
- Marker project database adalah `algentra:production:<project-ref>` dan revision schema cocok dengan kode release.
- Service Core API berjalan sebagai akun terbatas; tunnel meneruskan ke localhost dan firewall tidak membuka port 8000.
- Vercel Preview, Cloudflare Access policy, service token, dan HMAC sudah diuji sebelum domain Production dialihkan.
- Backup terenkripsi sudah berjalan dan restore sudah diuji pada staging; lihat [panduan backup](docs/BACKUP.md).
- Fitur trading diuji pada akun demo dahulu. EA Telegram mengirim order live saat diaktifkan; managed worker juga dapat mengirim order live untuk job yang client setujui.

Jangan cut over ke trading live sebelum database, backup/restore, login, pembayaran, EA/worker, dan prosedur penghentian sudah diverifikasi. Proyek belum divalidasi menyeluruh dengan kredensial production, akun hosting Vercel, atau akun broker live.
