# Status Refactor Backend & Copier ke Go

Dokumen ini menggantikan checklist migrasi awal. Status di bawah berdasarkan source dan konfigurasi repository; deployment cloud/server harus diverifikasi pada lingkungan masing-masing.

## Selesai di repository

- [x] Modul Go menggantikan runtime Python untuk Core API dan copier. Kode Python lama sudah dihapus dari tree kerja; rilis tidak memerlukan Python.
- [x] Core API memakai Go `net/http`, koneksi PostgreSQL melalui `pgxpool`, readiness check, dan migrasi SQL eksplisit.
- [x] Logika auth/crypto, Midtrans, Telegram, Gemini, customer service, serta WebSocket tersedia dalam implementasi Go.
- [x] Copier master/follower, kalkulasi lot, format sinyal, state, dan managed worker supervisor tersedia di Go.
- [x] Integrasi MT5 menggunakan EA MQL5 serta local feed yang diimplementasikan di repository.
- [x] Frontend Next.js, BFF, kontrak OpenAPI checked-in, dan tipe TypeScript tersedia.
- [x] Skrip build Windows, installer Windows Service Core API, Scheduled Task worker, serta panduan operasi dan backup tersedia.
- [x] CI menjalankan pemeriksaan tipe/build web, tes Go, build executable Windows, dan pemeriksaan tipe kontrak terhadap file OpenAPI.

## Belum dapat dinyatakan selesai dari repository

- [ ] **Deployment Vercel production:** set `CORE_API_ORIGIN`, `BFF_HMAC_SECRET`, dan bila memakai Cloudflare Access, service token pada Vercel; deploy lalu validasi domain dan jalur BFF. Langkah manual ada di `docs/SERVICE.md`.
- [ ] **Deployment Windows/Supabase production:** provision schema production, environment file ber-ACL ketat, pasang/start service dan worker, lalu cek `/health`, `/ready`, EA, dan jalur copier. Status deployment tidak tersimpan di repository.
- [ ] **Validasi end-to-end pada staging/demo:** jalankan alur login/dashboard, realtime WebSocket, Telegram/Gemini, Midtrans sandbox, serta master/follower MT5 di Windows. CI tidak menggantikan uji integrasi ini.
- [ ] **Benchmark latensi sinyal:** requirement awal menyebut sub-milidetik, tetapi belum ada benchmark atau kriteria pengukuran yang dicatat. Ukur pada mesin Windows/MT5 target dan dokumentasikan hasil sebelum mengklaim requirement terpenuhi.
- [ ] **OpenAPI dari source Go:** saat ini `packages/contracts/openapi.json` adalah kontrak yang di-commit dan CI memastikan tipe TS dibuat darinya; tidak ada generator dari kode Go. Pilih satu workflow sumber-kontrak dan buat pemeriksaan drift endpoint/schema. Jangan mengklaim `swaggo/swag` sudah menghasilkan kontrak identik sebelum seluruh route dan schema Go direpresentasikan dan dibandingkan.

## Catatan perubahan pendekatan

- Database menggunakan `pgxpool` dan SQL/migrasi terkontrol, bukan ORM GORM. Router memakai `net/http`, bukan Fiber/Gin. Keduanya pilihan implementasi yang valid dan sudah terintegrasi dengan arsitektur yang ada.
- EA berkomunikasi melalui local feed/protokol yang ada di source; named pipe tidak menjadi prasyarat jika mekanisme tersebut memenuhi isolasi dan keamanan deployment.
- `refactor.md` lama merujuk file Python yang sudah dihapus dan menandai seluruh pekerjaan belum selesai. Checklist itu tidak lagi mencerminkan keadaan repository.

## Checklist rilis operator

1. Jalankan CI dan simpan artifact semua executable Windows dari commit rilis.
2. Deploy lebih dulu ke Supabase/staging dan Vercel Preview; cek konfigurasi BFF, Cloudflare Access, CORS/WebSocket, dan health/readiness.
3. Uji integrasi dengan Midtrans sandbox dan akun MT5 demo; jangan aktifkan job live sebagai bagian smoke test.
4. Terapkan ke production sesuai maintenance window, buat backup terverifikasi, lalu cek service, worker, tunnel, dan portal setelah deploy.
5. Catat commit, versi binary/EA, hasil benchmark, hasil smoke test, dan waktu deployment pada catatan rilis.
