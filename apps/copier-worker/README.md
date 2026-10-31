# Copy Trade MT5

Sistem ini memantau posisi pada satu terminal MT5 master dan menyalin perubahan ke satu terminal MT5 terpisah untuk setiap follower. Untuk managed mode, TelegramSignalEA mengirim snapshot ke Core API, worker meneruskannya ke publisher Go, lalu follower mengeksekusi melalui EA MT5. Mode standalone tetap dapat memakai EA publisher master. Gunakan akun demo untuk seluruh pengujian awal.

## Arsitektur

```text
Terminal MT5 master + EA publisher
             │ snapshot HTTP loopback
             ▼
 algentra-copier-master.exe ── JSONL harian ──┬─ feed Go + EA terminal #1
                                              ├─ feed Go + EA terminal #2
                                              └─ feed Go + EA terminal #N
                          algentra-copier-launcher.exe / algentra-copier-worker.exe
```

Setiap akun follower memakai satu terminal MT5 terpisah. Supervisor Go menolak konfigurasi yang memakai ulang path terminal, login follower, atau nama proses. TelegramSignalEA pada master hanya membaca posisi dan menerbitkan snapshot; EA follower yang mengirim order ke akun follower.

## Persiapan Windows VPS

1. Installer one-click menyiapkan Go, binary copier, MT5 master, template follower, dan EA `.ex5` di `C:\Algentra`. Python tidak diperlukan untuk copier standalone maupun managed.
2. Sediakan satu folder instalasi MT5 portable di bawah `C:\Algentra\mt5\followers` untuk setiap akun dan path terminal yang unik. Jangan arahkan dua proses ke terminal yang sama.
3. Jalankan setiap `terminal64.exe /portable`, login ke akun follower yang sesuai, lalu tutup terminal dengan normal. Pastikan terminal master sudah login. Mode managed menyalin template MT5 bersih ke runtime per akun; template tidak perlu berisi sesi login.
4. Installer mengompilasi `MT5FollowerCopyEA.mq5` dan `Algentra\MT5LoginProbe.mq5` secara otomatis. Worker mengaktifkan izin Auto Trading dan setelah terminal follower dibuka, otomatis menambahkan `http://127.0.0.1:8766` ke daftar WebRequest lewat dialog MT5. Proses ini membutuhkan desktop Windows interaktif (sesuai Scheduled Task installer); hasilnya dicatat di `config\runtime\logs\webrequest_<account_id>.log`.
5. Jika worker dipasang pada mesin terpisah yang tidak menjalankan installer one-click, buka PowerShell pada root repo dan build binary Go:

   ```powershell
   go build -ldflags="-s -w" -o algentra-copier-master.exe ./cmd/copier-master
   go build -ldflags="-s -w" -o algentra-copier-launcher.exe ./cmd/copier-launcher
   ```

Launcher Go menyalin EA ke folder terminal, membuat preset follower, dan menghapus file startup sementara yang berisi password setelah terminal berjalan.

## Konfigurasi rahasia

File `master.example.json` dan `managed-worker.example.json` ada di `apps/copier-worker/config/`. Contoh `follower.example.json` dan `launcher.example.json` tidak disertakan; file `master.json`, `managed-worker.json`, `launcher.json`, dan konfigurasi follower dibuat secara lokal dan tidak dilacak Git. Dari folder `apps/copier-worker`, buat file konfigurasi master. `.gitignore` mengabaikan `config/*.json`.

```powershell
Set-Location apps\copier-worker
Copy-Item config\master.example.json config\master.json
$env:MT5_PASS_AKUN1 = "password-akun-demo"
```

`master.json` harus mempunyai `terminal_path` dan `folder_sinyal`; opsionalnya `filter_magic`, `filter_simbol`, `abaikan_komentar_mengandung`, dan `poll_interval_ms`. Untuk mode standalone, buat `config/akun1.json` dan `config/launcher.json` secara lokal. Contoh minimum:

```json
{
  "nama": "akun1",
  "terminal_path": "C:/MT5_F1/terminal64.exe",
  "portable": true,
  "login": 12345678,
  "password_env": "MT5_PASS_AKUN1",
  "server": "Broker-Demo",
  "folder_sinyal": "C:/Algentra/signals",
  "map_simbol": {"XAUUSD": "XAUUSDm"},
  "mode_lot": "rasio",
  "rasio_lot": 1,
  "max_lot_per_order": 0.05,
  "max_posisi_terbuka": 3,
  "max_lot_total": 0.1,
  "dry_run": true,
  "aktif": true,
  "heartbeat_folder": "runtime/heartbeats"
}
```

Untuk file `config/launcher.json`, gunakan JSON berikut:

```json
{
  "master_config": "master.json",
  "followers": ["akun1.json"],
  "check_interval_seconds": 5,
  "heartbeat_timeout_seconds": 30,
  "max_restarts_per_10_minutes": 10
}
```

Mode lot yang tersedia: `tetap` memakai `lot_tetap`, `rasio` memakai `rasio_lot`, dan `saldo` membandingkan saldo follower dengan `saldo_master`. Untuk `saldo`, isi `saldo_master`. Nilai lot dibulatkan turun mengikuti `volume_step` broker; worker melewati order jika hasilnya di bawah `volume_min`.

Ini adalah format **launcher standalone**; password follower dibaca dari environment variable bernama `password_env`, bukan dari JSON, argumen proses, atau log. Task managed pada portal one-click memakai alur lain: client menyimpan password di backend terenkripsi dan tidak perlu menyiapkan environment variable Windows. Untuk menjalankan launcher standalone lewat Task Scheduler, environment password harus tersedia pada proses task.

Atur `terminal_path` dan `folder_sinyal` di master dan follower. Gunakan folder sinyal yang sama pada semua config. Jika broker memakai nama simbol berbeda, isi `map_simbol`, misalnya `{"XAUUSD":"XAUUSDm"}`. Config follower wajib menetapkan `max_lot_per_order`, `max_posisi_terbuka`, dan `max_lot_total`. Contoh konfigurasi di atas menggunakan `dry_run: true`; sesuaikan batas dengan akun.

## Menjalankan

Jalankan tahapan ini pada akun demo terlebih dahulu:

1. Jika belum dibuat, dari root repo build publisher dan launcher Go:

   ```powershell
   go build -ldflags="-s -w" -o algentra-copier-master.exe ./cmd/copier-master
   go build -ldflags="-s -w" -o algentra-copier-launcher.exe ./cmd/copier-launcher
   ```

2. Pastikan follower menggunakan `dry_run: true`. Jalankan `apps\copier-worker\scripts\start-all.ps1` dari root repo. Dry run tidak mengirim order. Supervisor Go menjalankan publisher dan terminal follower melalui EA.
3. Setelah log dan mapping diperiksa, uji order dengan akun demo kosong, `dry_run: false`, serta volume kecil. Jangan hapus state snapshot atau Global Variables terminal saat masih ada posisi copier yang terbuka.
4. Pindah ke akun sebenarnya hanya setelah verifikasi demo dan pemeriksaan aturan broker selesai. Periksa batas lot dan posisi untuk setiap akun.

Worker tidak memerlukan Python atau package MetaTrader5. Supervisor, publisher, transport sinyal, validasi risiko, dan probe login dijalankan oleh binary Go; eksekusi order dilakukan EA MT5 pada tiap terminal follower.

## Publisher master Go

Mode publisher EA dipakai untuk standalone yang menyalin posisi terbuka MT5 secara langsung (`master_source: publisher_ea`). Pasang `MT5MasterPublisherEA.mq5` pada chart mana pun di terminal MT5 yang login ke akun master sumber posisi. EA hanya membaca posisi master dan mengirim snapshot melalui HTTP loopback; `copier-master.exe` membandingkan snapshot dan menulis JSONL yang dibaca follower. EA ini tidak membuat order dan tidak dipasang pada terminal follower. Untuk managed mode dengan `master_source: telegram_signal`, gunakan `TelegramSignalEA.mq5` di terminal master; EA publisher ini tidak diperlukan.

1. Build dari root repo: `go build -ldflags="-s -w" -o algentra-copier-master.exe ./cmd/copier-master` dan `go build -ldflags="-s -w" -o algentra-copier-launcher.exe ./cmd/copier-launcher`.
2. Gunakan `scripts/install_startup_task.ps1` untuk membuat task publisher. Script membuat token acak otomatis (atau memigrasikan token lama), menyimpannya pada file rahasia runtime, dan menampilkannya untuk diisi pada input `PublisherToken` EA. Token tidak ditulis ke environment User/Machine; task hanya memasukkannya ke environment proses publisher saat berjalan.
3. Compile `MT5MasterPublisherEA.mq5`, pasang pada terminal master, lalu izinkan URL publisher loopback `http://127.0.0.1:8765` di pengaturan Expert Advisors terminal MT5.
4. `scripts/start-all.ps1` memulai publisher Go dan supervisor standalone. Jangan jalankan `infra\windows\run-copier-master.ps1` bersamaan dengan supervisor tersebut; script itu untuk publisher tanpa launcher standalone.

Managed worker Go dan supervisor standalone menjalankan binary Go ini sebagai publisher master. EA follower membaca feed loopback Go dan mengeksekusi order di terminal MT5 pada kedua mode.

## Managed copy dari portal client

Mode ini menyatukan pengaturan follower dengan portal client Algentra. Akun tercipta otomatis setelah pembayaran QRIS Midtrans terkonfirmasi; transfer manual memerlukan review admin. Client memasukkan password broker dan batas risiko di halaman akun; login diperiksa otomatis. Password dienkripsi di backend dan worker mengambil job aktif melalui `X-Copier-Worker-Key`. Worker Windows menjalankan supervisor, publisher Go, login probe, dan feed lokal. TelegramSignalEA pada terminal master mengirim snapshot ke API; worker meneruskannya ke publisher Go lokal, sehingga `MT5MasterPublisherEA` terpisah tidak diperlukan. Client tidak perlu memasang MT5 di perangkatnya, dan copy live baru dimulai setelah client menyetujui aktivasi/risk.

### Instalasi one-click pada satu server

Tidak perlu build binary, menyalin EA, membuat file konfigurasi worker, atau menjalankan helper secara manual. `install-one-click.cmd` memasang Go, Node.js, NSSM, cloudflared, MT5 master dan template follower di `C:\Algentra`, membangun binary/EA, membuat key worker, menyiapkan folder runtime, dan memasang serta menjalankan task managed worker. Key EA dan worker dapat dilihat di Dashboard > Settings; key internal publisher dibuat dan dipasang otomatis, tidak perlu disalin ke EA. Installer juga mengisi preset TelegramSignalEA master dan mengaktifkan WebRequest lokal.

Setelah instalasi portal selesai, client membuat akun, mengisi kredensial broker follower dan batas risiko, lalu menyetujui aktivasi copy live. Worker membuat terminal follower terisolasi dari template lokal, memvalidasi login di latar belakang, dan menjalankan EA. Login Windows tetap diperlukan setelah reboot karena MT5 harus berjalan pada sesi desktop interaktif.

Folder terminal template, binary, key lokal, sinyal, log, dan temp worker berada di bawah `C:\Algentra`. Mode master `telegram_signal` tidak memerlukan terminal master kedua atau `MT5MasterPublisherEA`. Worker mengaktifkan WebRequest lokal pada terminal follower secara otomatis.

### Host worker terpisah atau development

Langkah build binary, pembuatan konfigurasi lokal, dan instalasi task manual hanya berlaku bila worker berjalan di host Windows berbeda atau saat development. Ikuti bagian [instalasi worker pada host terpisah](#host-worker-windows-terpisah) di bawah. Untuk instalasi same-server one-click, lewati langkah manual tersebut.

Portal menyediakan pencarian direktori broker/server dan mengirimkan nama server terpilih ke worker. Saat probe, worker mencari ulang nama server untuk memperoleh alamat koneksinya; hanya nama server publik yang dikirim ke direktori, bukan nomor akun atau password. Jika pencarian jaringan gagal, worker memakai `Config/servers.dat` lokal sebagai fallback. Server tetap harus sesuai detail akun broker karena nomor login tidak menentukan server secara unik. File konfigurasi sementara yang berisi password dihapus setelah pemeriksaan; polling dan heartbeat tetap berjalan selama probe (batas 150 detik). Copy tidak dapat dimulai sebelum validasi berhasil. EA follower mendukung Hedging dan Netting; pada Netting worker memakai exposure virtual per simbol dan menahan simbol jika posisi broker berbeda dari state copier.

Worker Go menjalankan publisher master, memeriksa job aktif mengikuti `poll_seconds` (dibatasi 2–60 detik), membuat terminal terisolasi, dan melaporkan heartbeat, saldo, posisi terbuka, serta deal MT5 ke portal. Saat job baru mulai, worker mengirim histori deal 30 hari terakhir per batch maksimal 100 deal, kemudian melanjutkan dari cursor terakhir. Jangan jalankan `scripts/start-all.ps1` bersamaan karena managed worker sudah menjalankan publisher master sendiri. Password broker hanya berada di environment proses worker dan startup INI MT5 sementara yang dihapus setelah terminal mulai; password tidak disimpan dalam preset EA atau argumen proses. API key worker tidak diteruskan ke proses MT5. Log worker tidak mencatat request body atau environment. `api_base_url` wajib HTTPS; HTTP hanya diizinkan untuk `localhost` atau `127.0.0.1`.

Pada server one-click, installer utama membangun binary Go, memasang MT5 dan EA, membuat key worker acak, menyimpan key pada file lokal ber-ACL terbatas dan memasangnya hanya ke environment proses worker saat task berjalan. Key yang sama dapat dilihat dan disalin melalui Dashboard > Settings. Task managed copier juga didaftarkan dan dijalankan otomatis.

### Host worker Windows terpisah

Untuk memasang worker pada host Windows terpisah, login sebagai akun Windows yang akan menjalankan MT5 lalu jalankan PowerShell dari root repo. Gunakan `COPIER_WORKER_API_KEY` yang sama dengan backend atau salin key dari Dashboard > Settings; installer task menerima key melalui environment sementara atau prompt tersembunyi, menyimpannya dalam file lokal ber-ACL terbatas, lalu memvalidasi koneksi:

```powershell
.\apps\copier-worker\scripts\install_managed_worker_task.ps1 `
  -ProjectRoot "C:\Algentra\app" `
  -ApiBaseUrl "https://portal.example.com" `
  -TerminalTemplateDir "C:\Algentra\mt5\follower-template" `
  -SignalDirectory "C:\Algentra\signals"
```

Installer membuat konfigurasi managed dari contoh, memaksa `execution_mode: mt5_ea`, lalu memvalidasi binary Go, EA, konfigurasi worker/master, folder template MT5, terminal master, folder sinyal, key yang cocok dengan backend, dan koneksi backend sebelum membuat task. `TerminalTemplateDir` default ke folder follower bersih yang disiapkan installer; jangan gunakan folder master atau terminal client yang sudah login. Key tidak dicetak atau ditaruh di argumen command line; file rahasia runtime hanya dapat dibaca oleh akun worker, Administrators, dan SYSTEM. `ApiBaseUrl` harus dapat dijangkau dari host worker. Managed worker tidak memerlukan instalasi Python.

Task **Algentra Managed Copy Worker** berjalan dengan hak standar akun Windows yang memasangnya dan trigger saat akun tersebut login (MT5 perlu sesi desktop interaktif). Installer memulai task setelah membuatnya; Task Scheduler mencoba ulang proses yang berhenti dan hanya mengizinkan satu instance. Task membaca key dari file rahasia runtime dan hanya memasangnya pada environment proses worker selama task berjalan. Jika startup gagal sebelum logger worker aktif, stderr disimpan di `apps\copier-worker\config\runtime\logs\managed_worker-task-error.log`; log worker ada di `apps\copier-worker\config\runtime\logs\managed_worker.log`. Dengan `runtime_dir: runtime` pada contoh, runtime berada di `apps\copier-worker\config\runtime`: log master `logs\master-publisher.log`, log follower `logs\managed_<account_id>.log`, state di `states\<account_id>.json`, dan salinan terminal di `terminals\<account_id>`. Installer membuat task, tetapi tidak membuat akun follower, menyetujui pembayaran, atau mengaktifkan job trading. Client tetap harus menyelesaikan proses akun dan secara eksplisit memulai copy dari portal. Setelah task berjalan, pantau status akun dan heartbeat di portal.

Untuk menerapkan perubahan source pada instalasi yang sudah ada, perbarui repo pada `ProjectRoot`, build ulang binary Go, lalu restart task agar worker memuat kode baru:

```powershell
Stop-ScheduledTask -TaskName 'Algentra Managed Copy Worker'
Start-ScheduledTask -TaskName 'Algentra Managed Copy Worker'
Get-ScheduledTaskInfo -TaskName 'Algentra Managed Copy Worker' |
  Select-Object LastRunTime, LastTaskResult
```

Client dapat menjeda copy kapan saja dari portal. Menjeda atau menghapus profil menghentikan copier, tetapi posisi yang sudah terbuka tetap ada pada akun broker dan tidak ditutup otomatis. Menghapus akun dari portal membuat worker membersihkan folder terminal, state, dan konfigurasi follower milik akun tersebut pada sinkronisasi berikutnya. Uji end-to-end hanya dengan akun demo dahulu; worker aktif akan menggunakan `dry_run: false` setelah client menyetujuinya di portal.

## Menambah follower standalone

Buat JSON follower baru mengikuti contoh `akun1.json`, tetapkan `nama`, login, password environment variable, terminal path unik, folder sinyal, dan batas risiko. Tambahkan nama file itu ke array `followers` di `config/launcher.json`. Untuk akun tambahan, sediakan instalasi MT5 portable dan variabel password sendiri. Satu terminal hanya boleh mengendalikan satu akun.

## Checklist follower siap menerima sinyal

- Terminal MT5 follower berjalan, login ke akun yang benar, dan path-nya cocok dengan config.
- Algo Trading aktif dan **Allow algorithmic trading** dicentang.
- Environment variable password tersedia untuk proses yang berjalan.
- Copier mendukung akun Hedging serta Netting/Exchange. Pada follower Netting, volume semua posisi virtual master pada simbol yang sama digabung sebagai exposure bertanda: Buy mengurangi Sell, dan order MT5 hanya menyesuaikan selisih menuju exposure target. Batas `max_lot_total` menghitung volume bruto semua posisi virtual, termasuk yang saling mengimbangi.
- Gunakan akun follower khusus copier dan jangan membuka/menutup transaksi manual di simbol yang disalin. Perubahan posisi di luar copier akan terdeteksi saat rekonsiliasi dan simbol terkait dijeda sampai state diperiksa.
- SL/TP tiap posisi master di akun follower Netting dipantau secara virtual oleh worker karena akun hanya memiliki satu posisi per simbol. Worker harus terus hidup dan terhubung; jika berhenti, SL/TP virtual tidak dieksekusi broker untuk tiap posisi master. Gunakan akun demo untuk memastikan perilaku dan toleransi eksekusi broker sebelum live.
- Simbol hasil mapping tersedia di broker dan spesifikasi lot sesuai batasnya. Follower memanggil `symbol_select` untuk menampilkan simbol; terminal managed tidak harus memiliki chart terbuka.
- `dry_run` dan batas `max_posisi_terbuka`, `max_lot_total`, serta `max_lot_per_order` sudah ditinjau.
- Heartbeat `heartbeat_<nama>.txt` diperbarui dan log follower tidak menunjukkan error koneksi atau order.

## File sinyal, state, dan batasan

Publisher membentuk snapshot awal dari posisi yang sudah ada dan hanya menerbitkan perubahan setelahnya; posisi master yang terbuka sebelum snapshot awal tidak otomatis disalin. Setiap perubahan ditulis sebagai satu baris JSON ke `sinyal_YYYYMMDD.jsonl`. Follower menyimpan offset byte, tanggal file, mapping posisi, dan ID sinyal secara atomik di `state/<nama>.json` (dapat diubah dengan `state_path`). Entry buka yang lebih tua dari `max_umur_sinyal_buka_detik`—default 3 detik—dilewati; ubah, partial close, dan tutup tetap dibaca tanpa batas umur. Pada Netting, kegagalan eksekusi menahan offset agar sinyal dicoba lagi; mismatch dengan posisi aktual menjeda simbol. Pada Hedging, sinyal tetap dicatat telah diproses setelah percobaan order gagal. Restart tepat setelah broker menerima order namun sebelum state sempat ditulis masih memiliki jendela kecil yang tidak dapat dibuat exactly-once oleh file JSONL saja; pantau log dan terminal saat gangguan proses.

Latency polling, jaringan, dan slippage membuat transaksi follower mirip, bukan identik, dengan master. Spread, nama simbol, ukuran lot minimum, mode akun, dan aturan eksekusi berbeda antarbroker. Periksa aturan broker atau prop firm mengenai copy trading serta penggunaan banyak akun dari satu alamat IP.

## Pemecahan masalah

- **10027 / autotrading dinonaktifkan**: nyalakan Algo Trading dan aktifkan **Allow algorithmic trading** pada terminal follower.
- **10030 / filling mode tidak didukung**: periksa `symbol_info().filling_mode` dan spesifikasi simbol broker; follower mencoba filling mode yang dinyatakan didukung.
- **10014 / volume tidak valid**: periksa `volume_min`, `volume_max`, `volume_step`, `max_lot_per_order`, dan mode lot.
- **Akun Netting**: didukung. Worker menggabungkan exposure virtual per simbol, membatasi `max_posisi_terbuka` berdasarkan jumlah simbol aktif, dan menghitung `max_lot_total` dari volume bruto posisi virtual.
- **Portal terus `CHECKING`**: lihat `config\runtime\logs\managed_worker.log`. Baris `probe login akun ... selesai` menunjukkan hasil EA/log broker; `hasil pemeriksaan login ... tidak terkirim` menunjukkan worker tidak dapat melaporkan hasil ke API. Probe berjalan di latar belakang agar polling dan heartbeat tetap hidup.
- **Probe login MT5 timeout**: setelah 150 detik worker membedakan laporan EA, hasil otorisasi log terminal, dan terminal yang tidak pernah mencoba login. Pastikan `MT5LoginProbe.ex5` ada di `mt5\experts\Algentra`, koneksi keluar worker mengizinkan direktori broker dan server MT5, dan server yang dipilih cocok dengan detail akun broker. Pencarian direktori default memakai layanan pihak ketiga `mt5.mtapi.io`; atur `MT5_BROKER_DIRECTORY_URL` pada proses web dan worker untuk memakai resolver HTTPS milik sendiri. Timeout bukan bukti tunggal bahwa password salah.
- **`MCP bind error on 127.0.0.1:22346`**: port itu milik MCP MT5, bukan feed copier di `8766`. Worker probe membersihkan terminal sisa hanya dari folder akun yang sedang diperiksa. Jika pesan tetap muncul, periksa apakah ada terminal lain di luar folder runtime akun yang memakai port tersebut sebelum menghentikannya.
- **Market Watch kosong atau belum ada chart**: tampilan terminal pada runtime baru dapat kosong dan tidak dengan sendirinya berarti copier gagal. Follower memilih simbol dari `map_simbol` saat mulai; periksa log follower dan heartbeat.
- **`positions_get()` bernilai `None` di master**: publisher mencatat `last_error()` dan menahan snapshot; kondisi ini tidak memicu sinyal tutup massal.
- **Sinyal buka kedaluwarsa**: publisher/follower atau koneksi terminal terlambat; entry lama sengaja dilewati. Sinyal pengelolaan posisi tetap diproses.
- **Follower tidak muncul di launcher**: pastikan file JSON ada, terdaftar di `followers`, `aktif` bukan `false`, dan nama follower unik.

## Startup task standalone

Jalankan PowerShell sebagai akun Windows yang akan menjalankan VPS task, dari folder `apps/copier-worker`:

```powershell
Set-Location apps\copier-worker
.\scripts\install_startup_task.ps1 `
  -ProjectRoot (Get-Location).Path
```

Script membuat scheduled task **CopyTrade MT5 Launcher** dengan trigger **AtLogOn** dan hak standar; task berjalan saat akun Windows login karena terminal MT5 perlu sesi desktop interaktif. Script ini tidak langsung memulai task. Pastikan file `config/launcher.json` dan environment variable password tersedia untuk akun tersebut, serta gunakan izin tulis terbatas pada folder terminal/config yang diperlukan. Log standalone ada di `apps/copier-worker/logs/`.
