# Backup dan pemulihan

Backup database production harus dijalankan terpisah dari Core API, dengan akun Windows khusus dan kredensial terbatas. Detail instalasi production utama ada di [README](../README.md); service Windows dan rilis ada di [SERVICE.md](./SERVICE.md).

Dump database tidak mencakup rahasia konfigurasi lokal. Sertakan `C:\Algentra\secrets\core-api.env`, `C:\Algentra\secrets\cloudflared-token`, `C:\Algentra\secrets\cloudflare-tunnel.json`, serta `C:\Algentra\app\backend\data\integration-secrets\app-secret.json`, `ea.json`, `copier-worker.json`, `gemini.json`, `telegram.json`, dan `optional-integrations.json` pada prosedur backup terenkripsi terpisah. Pertahankan ACL ketika memulihkan file dan jangan simpan salinan plaintext di folder backup bersama.

## Persiapan host backup

Pasang PostgreSQL client tools (`pg_dump`, `pg_restore`) dan `age` pada host backup. Simpan konfigurasi pada file terpisah di luar repository, misalnya `C:\Algentra\backup-config\backup.env`, dan batasi ACL NTFS untuk akun Task Scheduler backup serta administrator. Konfigurasi mencakup:

- `BACKUP_DIR`: folder tujuan backup terenkripsi.
- `BACKUP_DATABASE_URL`: koneksi Supabase dengan role `algentra_backup` yang read-only.
- `BACKUP_AGE_RECIPIENT`: public key age untuk enkripsi.
- `DATABASE_URL`: koneksi runtime yang digunakan script untuk memvalidasi target.

Project Supabase diverifikasi dari hostname direct atau username session pooler pada `DATABASE_URL`; variabel `SUPABASE_PROJECT_REF` opsional jika ingin mencantumkan identitas secara eksplisit.

Pada schema fresh production, role `algentra_backup` dibuat `NOLOGIN`. Aktifkan login dan set password interaktif hanya jika host backup benar-benar digunakan; role tersebut hanya mendapat hak baca.

Jangan letakkan private key age di server backup. Simpan private key pada lokasi terpisah dan terlindungi.

## Menjalankan backup

Uji manual:

```powershell
.\infra\windows\run-backup.ps1 -EnvironmentFile 'C:\Algentra\backup-config\backup.env'
```

Jadwalkan command yang sama setiap hari melalui Task Scheduler, dengan akun backup. Script membuat custom dump schema `app`, mengenkripsinya, menghapus dump plaintext sementara, dan merotasi 14 file terenkripsi. Pastikan folder backup punya ruang cukup dan lakukan pemeriksaan rutin atas hasil serta kode keluar task.

Sebelum menjadwalkan, build executable Go dari root repository dan salin bersama PostgreSQL client tools serta `age` ke host backup:

```powershell
go build -ldflags="-s -w" -o algentra-backup.exe ./cmd/backup-database
```

Simpan salinan tambahan di lokasi yang terpisah dari host backup dan Supabase. Folder backup pada disk yang sama bukan perlindungan terhadap kerusakan atau kehilangan host.

## Pemulihan dan rollback

Uji restore terlebih dahulu ke project Supabase staging, bukan langsung ke production:

1. Ambil private key age dari lokasi aman terpisah dan dekripsi backup pada host pemulihan yang terlindungi.
2. Pastikan file hasil dekripsi tidak dapat diakses oleh pengguna lain; hapus salinan plaintext setelah restore.
3. Restore ke database staging yang sesuai dan verifikasi tabel, jumlah baris, serta hubungan data.
4. Rekonsiliasi transaksi, order, posisi, dan deal dengan riwayat broker/MT5 sebelum merencanakan cutover.

Saat rollback rilis, periksa apakah database memerlukan pemulihan sebelum mengganti versi aplikasi. Cutover harus direncanakan setelah menghitung transaksi yang terjadi sejak waktu backup; restore lama dapat menghapus perubahan transaksi yang lebih baru.
