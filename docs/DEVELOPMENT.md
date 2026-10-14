# Panduan development

Panduan ini untuk lingkungan lokal/development saja. Gunakan project Supabase development khusus; jangan arahkan development launcher atau migrator ke production.

## Menyiapkan backend lokal

1. Gunakan Python 3.12 64-bit atau lebih baru.
2. Salin `.env.example` menjadi `.env.local`. Ganti `APP_SECRET_KEY` dengan nilai acak minimal 32 karakter dan `ADMIN_PASSWORD` dengan password minimal 12 karakter. Isi `EA_API_KEY` bila menguji EA.
3. Isi `DATABASE_URL` dengan role runtime `algentra_api`, `MIGRATION_DATABASE_URL` dengan role `algentra_migrator`, dan `SUPABASE_DEV_PROJECT_REF` dengan project ref yang sama. Ambil detail koneksi dari Supabase **Dashboard → Connect**; URL project `https://<project-ref>.supabase.co` bukan connection string PostgreSQL. Kedua URL harus menuju database development yang sama dan mewajibkan TLS (`sslmode=require`). Jangan masukkan Supabase `anon` atau `service_role` key.
4. Bootstrap project Supabase development satu kali sesuai bagian berikut.
5. Buat virtual environment dan pasang dependency:

   ```powershell
   py -3.12 -m venv .venv
   .\.venv\Scripts\python.exe -m pip install -r apps\core-api\requirements.txt
   ```

   Linux/macOS:

   ```sh
   python3 -m venv .venv
   .venv/bin/python -m pip install -r apps/core-api/requirements.txt
   ```

6. Jalankan migrasi eksplisit lalu backend:

   ```powershell
   infra\windows\migrate-development.ps1
   infra\windows\run-development.ps1
   ```

   Script migrasi memaksa `APP_ENV=development`, memeriksa project ref dan marker database, lalu memakai role migrator. Backend bind ke `127.0.0.1:8000`, memuat ulang saat kode berubah, dan tidak menjalankan migrasi otomatis saat start.
7. Buka web development di `http://localhost:3000` (lihat "Web lokal dan tunnel uji" di bawah; admin di `/admin/login`). Client mendaftar di `/register`, memverifikasi email, lalu masuk melalui `/login`. Backend di `127.0.0.1:8000` hanya melayani API JSON, bukan halaman web. Untuk pembayaran development, gunakan detail uji dan isi `PAYMENT_MANUAL_INSTRUCTIONS`; jangan memasukkan rekening production ke lingkungan uji.
8. Integrasi opsional: `GEMINI_API_KEY` untuk parser, key berbeda `GEMINI_CS_API_KEY` untuk chat, `TELEGRAM_API_ID`/`TELEGRAM_API_HASH` untuk koneksi Telegram, serta `TELEGRAM_BOT_TOKEN`/`TELEGRAM_ADMIN_CHAT_ID` untuk review pembayaran. Isi pasangan Telegram bot bersama-sama; bot pembayaran memakai long polling dan hanya boleh dijalankan oleh satu backend.

## Bootstrap Supabase development

Gunakan project Supabase terpisah dari production.

1. Pada `infra/supabase/bootstrap.sql`, ganti `REPLACE_WITH_SUPABASE_PROJECT_REF` dengan ref development, lalu jalankan SQL melalui Supabase **SQL Editor** sebagai project owner. Script menyiapkan role dan marker environment; Alembic membuat schema `app`, tabel, dan grant runtime.
2. Aktifkan login hanya untuk role development yang diperlukan:

   ```sql
   ALTER ROLE algentra_api LOGIN;
   ALTER ROLE algentra_migrator LOGIN;
   ```

   `algentra_backup` tetap `NOLOGIN` sampai host backup khusus dikonfigurasi.
3. Dari `psql` sebagai project owner, set password role secara interaktif dengan `\password algentra_api` dan `\password algentra_migrator`. Gunakan password berbeda dan jangan menulisnya ke SQL atau command history.
4. Isi connection string role pada `.env.local`. Gunakan format `postgresql+psycopg://...`, URL-encode karakter khusus password, dan gunakan direct/session pooler—bukan transaction pooler untuk migrasi.
5. Jalankan `infra\windows\migrate-development.ps1`. Setelah revisi pertama berhasil membuat schema `app` dan bookkeeping Alembic di `public`, jalankan `infra/supabase/revoke-dev-bootstrap-grants.sql` sebagai project owner setelah mengganti marker project. Migrasi berikutnya hanya mengubah objek pada schema yang sudah ada. Jangan menjalankan backend memakai project owner atau role migrator.

### Menyiapkan schema awal production

Ekspor snapshot schema tanpa data dari development yang sudah berada di revision head:

```powershell
Set-Location apps\core-api
..\..\.venv\Scripts\python.exe -m app.export_schema_snapshot D:\Secure\algentra-production-schema.sql
```

Exporter memeriksa marker project dan revision Alembic lalu menjalankan `pg_dump --schema-only`. DBA menerapkan snapshot satu kali pada project production kosong/awal sebagai project owner. Pastikan comment database menjadi `algentra:production:<project-ref>` dan comment schema `app` menjadi revision yang dicetak exporter. Jangan memakai snapshot ini untuk menimpa database production aktif.

### Impor database SQLite lokal ke Supabase development

Hentikan backend dan jalankan dry-run dahulu:

```powershell
Set-Location apps\core-api
..\..\.venv\Scripts\python.exe -m app.import_sqlite_to_supabase --dry-run
..\..\.venv\Scripts\python.exe -m app.import_sqlite_to_supabase
```

Secara default sumbernya `backend/data/app.db`; path lain dapat diberikan sebagai argumen. Importer membandingkan checksum tabel, melewati data yang sama, menambah baris yang belum ada, dan membatalkan transaksi bila hasilnya tidak cocok. Kolom legacy token ciphertext sengaja tidak diimpor.

## Web lokal dan tunnel uji

Web berada di `apps/web` dan disajikan sepenuhnya oleh Next.js — backend hanya melayani API JSON. Salin `apps/web/.env.example`, set `CORE_API_ORIGIN=http://127.0.0.1:8000`, dan gunakan `BFF_HMAC_SECRET` yang sama dengan backend development. Jalankan:

```powershell
Set-Location apps\web
npm ci
npm run dev
```

Untuk akses HTTPS sementara, jalankan backend development lalu dari terminal lain:

```powershell
cloudflared tunnel --url http://127.0.0.1:8000
```

Quick Tunnel memakai URL sementara dan hanya untuk development. Jangan buka port backend ke internet.

## Tes

Jalankan dari root project:

```powershell
.\.venv\Scripts\python.exe -m pytest apps\core-api\tests
$env:PYTHONPATH="apps\copier-worker\src"
.\.venv\Scripts\python.exe -m pytest apps\copier-worker\tests
```

Koneksi Gemini, Telegram, SMTP, EA, dan MT5 dengan kredensial sungguhan harus diuji terpisah. Gunakan akun demo untuk semua pengujian trading.
