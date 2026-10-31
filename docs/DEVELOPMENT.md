# Panduan development

Panduan ini untuk lingkungan lokal/development saja. Gunakan project Supabase development khusus; jangan arahkan development launcher atau migrator ke production.

## Menyiapkan backend lokal

1. Gunakan Go 1.25 atau lebih baru.
2. Salin `.env.example` menjadi `.env.local`. Isi `DATABASE_URL` dan project ref Supabase development. Application secret dibuat otomatis dan disimpan di `backend/data/integration-secrets`; pada database baru password admin acak dicetak sekali oleh backend. `COOKIE_SECURE` otomatis nonaktif pada development lokal. EA API key dan copier worker key dibuat otomatis dan tersedia pada Dashboard > Settings.
3. Isi `DATABASE_URL` dengan role runtime `algentra_api`. Project ref development dibaca dari hostname Supabase atau username session pooler; isi `SUPABASE_DEV_PROJECT_REF` hanya jika ref tidak dapat dideteksi otomatis. `MIGRATION_DATABASE_URL` dengan role `algentra_migrator` hanya diperlukan untuk menjalankan migrasi development. Ambil detail koneksi dari Supabase **Dashboard → Connect**; URL project `https://<project-ref>.supabase.co` bukan connection string PostgreSQL. Pakai direct connection atau session pooler pada port 5432 untuk kedua URL, dan wajibkan TLS (`sslmode=require`). Transaction pooler port 6543 tidak didukung oleh runtime persisten ini. Jangan masukkan Supabase `anon` atau `service_role` key.
4. Bootstrap project Supabase development satu kali sesuai bagian berikut.
5. Jalankan migrasi eksplisit lalu backend:

   ```powershell
   infra\windows\migrate-development.ps1
   infra\windows\run-development.ps1
   ```

   Runner migrasi Go memeriksa project ref dan marker database, lalu memakai role migrator untuk menerapkan revision schema. Backend Go bind ke `127.0.0.1:8000` dan tidak menjalankan migrasi otomatis saat start.
7. Buka web development di `http://localhost:3000` (lihat "Web lokal" di bawah; admin di `/admin/login`). Client mendaftar di `/register`, memverifikasi email, lalu masuk melalui `/login`. Backend di `127.0.0.1:8000` hanya melayani API JSON, bukan halaman web. Untuk pembayaran development, gunakan detail uji dan isi `PAYMENT_MANUAL_INSTRUCTIONS`; jangan memasukkan rekening production ke lingkungan uji.
8. Integrasi opsional: `GEMINI_API_KEY` dan `GEMINI_MODEL` dipakai parser dan chat customer-service. Pada production, key/model juga dapat disimpan melalui dashboard admin tanpa restart; perubahan disimpan lokal di `backend/data/integration-secrets/gemini.json` pada folder ber-ACL khusus. `TELEGRAM_API_ID`/`TELEGRAM_API_HASH` mengaktifkan koneksi Telegram, sedangkan `TELEGRAM_BOT_TOKEN`/`TELEGRAM_ADMIN_CHAT_ID` mengaktifkan review pembayaran. Isi pasangan Telegram bot bersama-sama; bot pembayaran memakai long polling dan hanya boleh dijalankan oleh satu backend.

## Bootstrap Supabase development

Gunakan project Supabase terpisah dari production.

1. Pada `infra/supabase/bootstrap.sql`, ganti `REPLACE_WITH_SUPABASE_PROJECT_REF` dengan ref development, lalu jalankan SQL melalui Supabase **SQL Editor** sebagai project owner. Script menyiapkan role dan marker environment; runner Go membuat schema `app`, tabel, dan grant runtime.
2. Aktifkan login hanya untuk role development yang diperlukan:

   ```sql
   ALTER ROLE algentra_api LOGIN;
   ALTER ROLE algentra_migrator LOGIN;
   ```

   `algentra_backup` tetap `NOLOGIN` sampai host backup khusus dikonfigurasi.
3. Dari `psql` sebagai project owner, set password role secara interaktif dengan `\password algentra_api` dan `\password algentra_migrator`. Gunakan password berbeda dan jangan menulisnya ke SQL atau command history.
4. Isi connection string role pada `.env.local` dalam format `postgresql://...`, URL-encode karakter khusus password, dan gunakan direct/session pooler—bukan transaction pooler untuk migrasi.
5. Jalankan `infra\windows\migrate-development.ps1`. Setelah revisi pertama berhasil membuat schema `app` dan tabel pencatatan revisi yang kompatibel di `public`, jalankan `infra/supabase/revoke-dev-bootstrap-grants.sql` sebagai project owner setelah mengganti marker project. Migrasi berikutnya hanya mengubah objek pada schema yang sudah ada. Jangan menjalankan backend memakai project owner atau role migrator.

### Production

Dokumen ini hanya untuk development lokal. Production baru memakai [installer schema final](../infra/supabase/fresh-production.sql) pada project Supabase kosong; data SQLite tidak diimpor ke production. Runner migration dan importer SQLite yang disebut di bawah hanya untuk project development dan tidak boleh dijalankan pada production.

### Membersihkan sinyal IGNORED

Perintah Go ini menghitung sinyal `IGNORED` yang tidak terhubung ke trade atau eksekusi EA beserta lognya. Perintah berjalan dalam mode dry-run secara default dan memakai `DATABASE_URL` dari environment atau `.env.local` development:

```powershell
go run ./cmd/cleanup-ignored-signals
go run ./cmd/cleanup-ignored-signals --apply
```

`--apply` menghapus baris dalam satu transaksi. Koneksi PostgreSQL development harus memakai role `algentra_api`, TLS, dan `SUPABASE_DEV_PROJECT_REF` yang cocok. SQLite hanya diterima di luar production.

### Impor database SQLite lokal ke Supabase development

Hentikan backend dan jalankan dry-run dahulu:

```powershell
Set-Location apps\core-api
$env:APP_ENV = 'development'
Set-Location ..\..
go run ./cmd/import-sqlite-to-supabase --dry-run
go run ./cmd/import-sqlite-to-supabase
```

Secara default sumbernya `backend/data/app.db`; path lain dapat diberikan sebagai argumen. Importer Go memeriksa marker project dan revision schema dengan role migrator, lalu memakai role runtime untuk membaca dan menulis data. Importer membandingkan checksum tabel, melewati data yang sama, menambah baris yang belum ada, dan membatalkan transaksi bila hasilnya tidak cocok. Kolom legacy token ciphertext sengaja tidak diimpor.

## Web lokal

Web berada di `apps/web`; Next.js development rewrites `/api/core/*` langsung ke Core API lokal. Tidak perlu origin API atau secret proxy terpisah. Jalankan backend development lebih dulu, lalu:

```powershell
Set-Location apps\web
npm ci
npm run dev
```

Portal development berjalan di `http://localhost:3000` dan memakai rewrite lokal ke Core API `http://127.0.0.1:8000`. Untuk preview eksternal sementara, gunakan Cloudflare Quick Tunnel; jangan membuka port API 8000 ke internet.

## Validasi backend Go

Jalankan pemeriksaan Go dari root project:

```powershell
gofmt -w cmd internal
go test ./...
go build ./...
```

CI menjalankan tes paket Go dan membangun executable Windows untuk backend serta copier.

Koneksi Gemini, Telegram, SMTP, EA, dan MT5 dengan kredensial sungguhan harus diuji terpisah. Gunakan akun demo untuk semua pengujian trading.
