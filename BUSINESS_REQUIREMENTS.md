# Business Requirements Document (BRD)
## Aplikasi: **Jira Sync Engineering**

> Dokumen ini disusun berdasarkan hasil pembacaan source code (Go) pada repository `jira-sync-eng`.
> Tujuan: mendeskripsikan kebutuhan bisnis, tujuan, ruang lingkup, dan aturan bisnis dari aplikasi
> tanpa membahas detail implementasi teknis.

---

## 1. Latar Belakang

Tim Engineering & QA membutuhkan **visibilitas kinerja delivery** yang konsisten dari aktivitas
harian yang tercatat di **Jira** dan **Google Calendar**. Saat ini data tersebar di banyak issue
Jira (Story, Task, Sub-task, Bug) dengan banyak custom field, serta event meeting (grooming) yang
hanya tersimpan di kalender masing-masing engineer.

Untuk mendukung pengambilan keputusan (kapasitas tim, SLA delivery, kualitas, beban rework, dan
beban meeting), dibutuhkan satu aplikasi yang **mengkonsolidasi data tersebut** ke dalam basis data
dan menyajikannya dalam **Google Sheet** yang mudah dibaca oleh stakeholder non-teknis (Engineering
Manager, QA Lead, Product, Leadership).

---

## 2. Tujuan Bisnis (Business Goals)

1. **Otomatisasi pelaporan delivery** Engineering & QA per minggu, per bulan, dan per Story.
2. **Transparansi SLA & Story Point** — mengukur jam kerja aktual vs Story Point yang dijanjikan.
3. **Identifikasi rework & kualitas** — memisahkan jam kerja "first-time right" (FTR) dari rework
   (hanging bug, fixing, code review bug, retest, dll).
4. **Tracking beban meeting grooming** per engineer untuk evaluasi kapasitas tim.
5. **Single source of truth** — data Jira historis tersimpan di database internal sehingga laporan
   dapat di-regenerate tanpa menarik ulang dari Jira.

---

## 3. Stakeholder

| Stakeholder | Kepentingan |
|---|---|
| Engineering Manager | Memantau SLA, kapasitas, beban rework tim |
| QA Lead | Memantau SLA QA, jumlah bug, retest, waiting time |
| Product Manager | Memantau status Story per Fix Version & Release Week |
| Tech Lead / PIC | Validasi distribusi pekerjaan & deteksi outlier |
| Leadership | Laporan agregat performa engineering |

---

## 4. Ruang Lingkup (Scope)

### 4.1 In-Scope
- Sinkronisasi issue Jira berdasarkan **JQL** yang dapat dikonfigurasi.
- Penyimpanan permanen ke **PostgreSQL**.
- Penulisan tiga lembar laporan ke **Google Spreadsheet** yang sama:
  1. Sheet **"Jira"** — detail per issue (mentah, satu baris per issue).
  2. Sheet **"Story Summary"** — agregasi per Story (jam kerja, SP, SLA, bug count).
  3. Sheet **"Event Grooming"** — daftar event grooming per engineer dari Google Calendar.
- Otorisasi Google Calendar via **OAuth2** per user (sekali per akun, token persisten).
- Fetch event grooming dari kalender yang sudah di-share ke pemilik akun OAuth.

### 4.2 Out-of-Scope
- Dashboard interaktif (web UI) — output akhir hanya berupa Google Sheet.
- Notifikasi otomatis (email, Slack, dsb).
- Penjadwalan otomatis (cron) — aplikasi dijalankan manual via CLI (`go run main.go <step>`).
- Edit data Jira (aplikasi bersifat **read-only** terhadap Jira & Calendar).
- Authentication/authorization end-user untuk aplikasi itu sendiri.

---

## 5. Aktor & Mode Eksekusi

Aplikasi dijalankan oleh **Operator** (biasanya Engineering Manager / Tools Engineer) melalui CLI.
Tersedia 5 perintah / mode:

| Command | Tujuan Bisnis |
|---|---|
| `step-1` | Tarik data dari Jira → simpan ke database (refresh data mentah) |
| `step-2` | Publish data mentah dari DB ke sheet **"Jira"** |
| `step-3` | Hitung & publish ringkasan per Story ke sheet **"Story Summary"** |
| `gcal-auth <email>` | Otorisasi sekali untuk satu akun Google (menyimpan token) |
| `google-calendar-sync` | Tarik event grooming → simpan ke DB → publish ke sheet **"Event Grooming"** |

Pemisahan menjadi step terpisah memungkinkan operator **re-publish sheet tanpa fetch ulang Jira**
(menghemat rate limit dan waktu).

---

## 6. Kebutuhan Fungsional (Functional Requirements)

### FR-1. Sinkronisasi Issue Jira (Step 1)
- Sistem **harus** dapat menarik seluruh issue Jira yang cocok dengan JQL yang dikonfigurasi.
- Sistem **harus** menangani pagination Jira secara otomatis.
- Sistem **harus** melakukan retry dengan exponential backoff untuk error `429` (rate limit) dan `5xx`.
- Setiap issue yang ditarik **harus** di-*upsert* ke database (primary key = Jira Key) — data lama
  tidak dihapus, hanya diperbarui jika sudah ada.
- Sistem **harus** menarik **changelog** issue (untuk menghitung jam kerja antar status).

### FR-2. Parsing & Klasifikasi Issue
Untuk setiap issue, sistem **harus** menghitung dan menyimpan field-field berikut:

| Field | Deskripsi Bisnis |
|---|---|
| Issue Type | Story / Task / Sub-task / Sub-task Engineer / Sub-task QA / Bug |
| Assignee, PIC Lead Engineer, PIC Lead QA | Penanggung jawab |
| Story Point | Estimasi effort |
| Fix Version, Released, Release Date, Release Week | Informasi rilis |
| Done Week | Nomor minggu (relatif terhadap *base date* yang dikonfigurasi) saat issue masuk status Done |
| Coding Hours, Code Review Hours, Testing Hours | Jam "first-time right" |
| Hanging Bug Hours (Eng/QA), Fixing Hours, Code Review Bug Hours, Retest Hours | Jam **rework** |
| `*_day_work_hours` | Versi yang menghitung **hanya jam kerja** (mengecualikan akhir pekan & hari libur) |
| Actual Task Start / Done Date / Week / Month / Year | Tanggal mulai & selesai aktual |
| Task Status, Status Story | Status akhir |
| Epic Key, Parent | Hubungan hirarki issue |
| From Type, Additional Task, Accident Bug, Bug From Category | Klasifikasi tambahan |

### FR-3. Aturan Bisnis: "Done Date"
- Untuk **assignee yang termasuk dalam daftar QA** (`qaAssignees`), tanggal selesai aktual diambil
  dari saat status berubah menjadi **`Done`**.
- Untuk assignee lainnya (Engineer), tanggal selesai aktual diambil dari saat status berubah
  menjadi **`Ready to Test`** (artinya pekerjaan engineer sudah selesai dan siap diuji).

### FR-4. Aturan Bisnis: Hari Kerja & Libur
- Sistem **harus** mengenali hari libur nasional Indonesia (tersimpan di `models/holiday.go`).
- Field `*_day_work_hours` **harus** mengeluarkan jam pada hari Sabtu, Minggu, dan hari libur.

### FR-4a. Klasifikasi Metrik Perhitungan Jam

Sistem menghasilkan **dua versi** dari sebagian besar metrik jam: versi **kalender** dan versi
**jam kerja**. Pembedaan ini penting agar stakeholder dapat memilih sudut pandang yang sesuai
(SLA real-time vs SLA berbasis kapasitas kerja efektif).

#### a) Berbasis **Jam Kalender** (24 jam × 7 hari, weekend & hari libur **ikut dihitung**)
Dihitung sebagai durasi mentah `(t_keluar − t_masuk)` antara dua titik waktu di Jira changelog,
tanpa filter apapun.

| Field | Sumber Perhitungan |
|---|---|
| `coding_hours` | Akumulasi waktu issue berada di status **`In Progress`** (changelog) |
| `code_review_hours` | Akumulasi waktu issue berada di status **`Code Review`** (changelog) |
| `testing_hours` | Akumulasi waktu issue berada di status **`In QA`** (changelog) |
| `retest_hours` | Akumulasi waktu issue berada di status **`Retesting`** (changelog) |
| `fixing_hours` | Akumulasi waktu di **`In Progress`** untuk *Eligible Bug* (Bug dengan `Accident Bug` yang qualified, atau rework dari Product/SA/Tech Debt) |
| `code_review_bug_hours` | Akumulasi waktu di **`Code Review`** untuk *Eligible Bug* |
| `hanging_bug_by_eng_hours` | **Selisih waktu** antara `issue.created` → transisi pertama ke **`In Progress`** (bukan akumulasi status) |
| `hanging_bug_by_qa_hours` | **Selisih waktu** antara transisi pertama ke **`Ready to Test`** → transisi pertama ke **`In QA`/`In Testing`** (bukan akumulasi status) |

> **Catatan**: pada metrik *Hanging Bug*, weekend & hari libur **tetap ikut dihitung** karena
> tujuannya menggambarkan berapa lama tiket "menggantung" dari sudut pandang user secara real-time.

#### b) Berbasis **Jam Kerja Efektif** (Senin–Jumat, **09:00–18:00 WIB**, exclude weekend + hari libur nasional)
Dihitung dengan algoritma `calcWorkHoursInInterval`: mengiris setiap interval per hari, hanya
menjumlahkan porsi yang jatuh pada hari kerja **dan** dalam jendela jam kerja `09:00–18:00 WIB`.

| Field | Sumber Perhitungan |
|---|---|
| `code_review_day_work_hours` | Status **`Code Review`** difilter hanya pada jam kerja |
| `hanging_bug_by_eng_day_work_hours` | Interval `created → first In Progress` difilter jam kerja |
| `hanging_bug_by_qa_day_work_hours` | Interval `first Ready to Test → first In QA` difilter jam kerja |
| `code_review_bug_day_work_hours` | Status `Code Review` (untuk Eligible Bug) difilter jam kerja |

Aturan filter:
1. **Weekend** (Sabtu & Minggu) selalu dilewati.
2. **Hari libur nasional** yang terdaftar di `models/holiday.go` selalu dilewati.
3. Hanya jam **09:00–18:00 WIB** yang dijumlahkan dalam satu hari kerja.
4. Bila status berlangsung lintas hari, durasi pada hari pertama hanya dihitung sampai 18:00, lalu
   dilanjutkan dari 09:00 hari kerja berikutnya.

#### c) Berbasis **Pergeseran Status Tiket Saja** (timestamp, bukan durasi)
Field-field ini **bukan akumulasi jam**, melainkan **tanggal/waktu transisi** atau **nomor minggu**
yang dihitung dari satu titik transisi tertentu di changelog. Hari libur & weekend tidak
mempengaruhi nilai field ini.

| Field | Aturan |
|---|---|
| `actual_task_start_date` | Timestamp transisi pertama ke `In Progress` (Engineer) atau `In QA` (QA / `Sub-task QA`) |
| `actual_task_done_date` | Untuk QA dengan key `CPB`/`IM`: transisi **terakhir** ke `Done`; untuk Engineer: transisi **pertama** ke `Ready to Test` (fallback: last `Done`) |
| `actual_task_done_week` | Nomor minggu (relatif terhadap Base Date) dari `actual_task_done_date` |
| `actual_task_done_month` / `actual_task_done_year` | Bulan & tahun dari `actual_task_done_date` |
| `done_week` | Nomor minggu dari `statusCategoryChangedDate` Story (saat Story mencapai status `Done`) |
| `release_week` | Nomor minggu dari `fixVersion.releaseDate` (Fix Version terbaru yang sudah `released`) |
| `first_ready_to_test_bug_date` | Timestamp transisi pertama ke `Ready to Test` (hanya untuk Bug) |
| `first_in_qa_bug_date` | Timestamp transisi pertama ke `In Testing` / `Retesting` / `In QA` (hanya untuk Bug) |
| `status_category_changed` | Timestamp `statusCategoryChangedDate` dari Jira |
| `task_status` / `status_story` | Status terkini issue / Story parent-nya |

> **Penting**: perhitungan **Done Week** & **Release Week** menggunakan hitungan **minggu kalender
> penuh** (Senin–Minggu) — weekend masuk dalam minggu yang sama; hari libur **tidak**
> menggeser nomor minggu.

#### d) Pemakaian di Sheet "Story Summary"
Untuk menghindari distorsi karena weekend (mis. issue masuk `Code Review` Jumat sore), sheet
**"Story Summary"** memakai versi *day-work* untuk metrik yang sensitif terhadap *idle time*,
sementara metrik yang merepresentasikan effort aktif pakai versi kalender:

| Kategori | Kolom Sheet | Versi yang Dipakai |
|---|---|---|
| FTR | Coding Hours | Kalender (`coding_hours`) |
| FTR | Code Review Hours | **Day-work** (`code_review_day_work_hours`) |
| FTR | Testing Hours | Kalender (`testing_hours`) |
| Rework | Hanging Bug Hours | **Day-work** (`hanging_bug_by_eng_day_work_hours`) |
| Rework | Fixing Hours | Kalender (`fixing_hours`) |
| Rework | Code Review Hours (bug) | **Day-work** (`code_review_bug_day_work_hours`) |
| Rework | Waiting Hours | **Day-work** (`hanging_bug_by_qa_day_work_hours`) |
| Rework | Retesting Hours | Kalender (`retest_hours`) |

#### e) Kapan setiap field di-populate (filter scope)
Tidak semua field diisi untuk setiap issue — bergantung pada `issue_type`:

| Field | Hanya diisi bila |
|---|---|
| `coding_hours`, `code_review_hours`, `code_review_day_work_hours` | Issue type = `Task` / `Sub-task` / `Sub-task Engineer` / `Sub-task QA` **dan bukan rework** |
| `testing_hours` | Bukan Story dan bukan Bug |
| `hanging_bug_*`, `retest_hours` | Issue type = `Bug` |
| `fixing_hours`, `code_review_bug_*` | *Eligible Bug* (Bug dengan kategori bug-from-engineer/SA/product/tech-debt, atau task yang ditandai sebagai rework dari Product/SA/Tech Debt) |
| `count_fix_version`, `status_story` | Hanya pada Story |
| `bug_from_category` | Hanya pada Bug |

---

### FR-5. Aturan Bisnis: Done Week
- "Done Week" dihitung sebagai jumlah minggu sejak **Base Date** (default `FIRST_YEAR/FIRST_MONTH/FIRST_DAY`,
  contoh `2026-01-05`).
- Base Date dapat dikonfigurasi via environment variable agar dapat dipakai lintas kuartal.

### FR-6. Publish Sheet "Jira" (Step 2)
- Sistem **harus** menghapus konten lama sheet (clear) sebelum menulis ulang.
- Sistem **harus** menulis seluruh field issue (40+ kolom) ke sheet **"Jira"**.
- Jumlah baris sheet **harus** diperluas otomatis bila data melebihi kapasitas.

### FR-7. Publish Sheet "Story Summary" (Step 3)
Sistem **harus** mengagregasi seluruh child issue (Task/Sub-task/Bug) ke Story parent-nya, kemudian
menulis satu baris per Story dengan kolom:

- Key, Epic Key, PIC Lead Engineer, PIC Lead QA, Done Week, Release Week
- **Story Point**: Total SP, SP QA, SP Eng
- **First-Time Right hours**: Coding, Code Review, Testing
- **Rework hours**: Hanging Bug, Fixing, Code Review Bug, Waiting, Retest
- **Bug Count**, Fix Version, Has FE (Yes/blank), Status Story
- **Old Formula** (legacy untuk perbandingan): Total Hours, SLA Eng, SLA QA, SLA All
- **New Formula** (current): Total Hours, SLA Eng, SLA QA, SLA All

Aturan agregasi:
- Child issue yang `ActualTaskDoneDate` **sebelum** Base Date **harus** diabaikan.
- Distribusi SP ke `SPEng` vs `SPQA` mengikuti aturan berdasarkan **issue type** dan **prefix key**
  (`IM`, `CPB`, `WB`) serta marker `[FE]`, `[BE]`, `[QA]` di summary:
  - Key mengandung `CPB`/`WB`: `Task` → Eng; `Sub-task` dengan `[FE]`/`[BE]` → Eng, lainnya → QA.
  - Key mengandung `IM` lainnya: `Task` → Eng; `Sub-task` dengan `[QA]` → QA, lainnya → Eng.
  - Key di luar filter: `Sub-task Engineer` → Eng; lainnya → QA.
- `HasFE = Yes` bila ada minimal satu assignee FE pada child issue.
- Story tanpa SP tidak menghasilkan SLA (kolom kosong, bukan error).
- Rumus SLA:
  - **Old**: `Total Hours = Coding + Testing + Fixing + Retest`; `SLA = Total Hours / SP`.
  - **New**: `Total Hours = FTR + Rework`; SLA Engineer & QA dipisah berdasarkan jenis jam.
- Output **harus** diurutkan ascending berdasarkan **Done Week**.
- Bila parent Story dari sebuah child tidak ditemukan di dataset, child di-skip dan jumlahnya dicetak
  sebagai warning (tidak menghentikan proses).

### FR-8. Sinkronisasi Google Calendar (gcal-auth & google-calendar-sync)
- Operator **harus** menjalankan `gcal-auth <email>` sekali per akun untuk menyetujui akses
  Google Calendar via OAuth2 (token disimpan di folder `tokens/`).
- Setiap user yang kalendernya hendak dibaca **harus** men-share kalender ke `GCAL_OWNER_EMAIL`
  dengan permission **"See all event details"**.
- Daftar email yang akan di-fetch berasal dari env `GCAL_FILTER_EMAILS` (comma-separated).
- Periode fetch: **1 Januari `FIRST_YEAR`** s/d **hari ini**.
- Hanya event yang **summary**-nya mengandung kata `"grooming"` (case-insensitive) yang diambil.
- Sistem **harus** menghitung durasi event dalam menit, membedakan event seharian (`is_all_day`).
- Event **harus** di-upsert ke tabel `google_calendar_events` dengan primary key gabungan
  `(user_email, event_id)`.
- Sistem **harus** menulis hasilnya ke sheet **"Event Grooming"** dengan kolom:
  User Email, Bulan, Tahun, Mulai, Selesai, Durasi (mnt), Nama Event.

### FR-9. Idempotensi & Keandalan
- Menjalankan ulang setiap step **harus aman** (tidak menggandakan data) — semua tulisan ke DB
  menggunakan UPSERT, dan tulisan ke Sheet selalu *clear & rewrite*.
- Step dapat dijalankan secara independen (mis. `step-2` tanpa `step-1` baru) selama DB sudah berisi.

---

## 7. Kebutuhan Non-Fungsional (Non-Functional)

| Kategori | Kebutuhan |
|---|---|
| **Performa** | Mampu menarik ribuan issue dengan pagination 100/page, batch upsert 500 row/insert. |
| **Reliability** | Retry otomatis pada error rate-limit & server-side dari Jira. |
| **Confidentiality** | Credential (`credentials.json`, `oauth2_client.json`, `.env`, `tokens/`) **tidak boleh** di-commit. |
| **Configurability** | Seluruh kredensial, JQL, base date, nama sheet, dan daftar email dapat di-set via `.env`. |
| **Maintainability** | Pemisahan modul: `jira/`, `db/`, `sheet/`, `google-calendar/`, `config/`, `models/`. |
| **Auditability** | Setiap baris DB memiliki `synced_at` (timestamp sinkronisasi terakhir). |
| **Portability** | Berjalan di mana saja yang mendukung Go 1.21+ dan PostgreSQL 13+. |

---

## 8. Asumsi & Dependensi

- Tim sudah memakai field standar & custom field Jira yang konsisten (`customfield_10024` = Story
  Point, `customfield_10156` = Accident Bug, `customfield_11397` = PIC Lead Engineer, dll).
- Akun Jira yang dipakai memiliki **akses baca** ke seluruh project yang relevan (project di luar
  scope dikecualikan via klausa `project NOT IN (...)` pada JQL).
- Google Spreadsheet target sudah **di-share ke service account** dengan permission Editor.
- Hari libur nasional yang dipakai sudah didefinisikan dalam kode (`models/holiday.go`); perlu
  diperbarui setiap tahun.
- Daftar **QA assignees** (`qaAssignees` di `jira/client.go`) sudah mencerminkan anggota tim QA aktif
  dan harus dijaga agar tetap relevan.

---

## 9. Aturan Bisnis (Business Rules) Ringkas

| ID | Rule |
|---|---|
| BR-01 | Done Date untuk QA (key `CPB`/`IM`) = transisi **terakhir** ke `Done`; untuk Engineer = transisi **pertama** ke `Ready to Test` (fallback: last `Done`). |
| BR-02 | Versi *day-work* (`*_day_work_hours`) menghitung **hanya** jam **09:00–18:00 WIB pada Senin–Jumat**, mengecualikan Sabtu, Minggu, dan hari libur nasional. Versi kalender (tanpa suffix `_day_work`) menghitung 24/7 termasuk weekend & libur. |
| BR-02a | `hanging_bug_by_eng_*` = interval `issue.created → first In Progress`; `hanging_bug_by_qa_*` = interval `first Ready to Test → first In QA/In Testing`. Bukan akumulasi durasi status, melainkan selisih dua titik transisi. |
| BR-02b | `coding_hours`, `code_review_*`, `testing_hours`, `fixing_hours`, `retest_hours`, `code_review_bug_*` = akumulasi durasi total saat issue berada di status terkait (dihitung dari changelog Jira). |
| BR-02c | `actual_task_*_date`, `done_week`, `release_week`, `first_ready_to_test_bug_date`, `first_in_qa_bug_date`, `status_category_changed`, `task_status`, `status_story` = murni hasil **pergeseran status tiket** (timestamp / kategori), **bukan** akumulasi durasi. Hari libur & weekend tidak mempengaruhi nilainya. |
| BR-02d | `done_week` & `release_week` dihitung sebagai jumlah minggu kalender penuh (Senin–Minggu) sejak Base Date; weekend masuk dalam minggu yang sama, hari libur tidak menggeser nomor minggu. |
| BR-03 | Done Week dihitung relatif terhadap Base Date (env `FIRST_YEAR/FIRST_MONTH/FIRST_DAY`). |
| BR-04 | Child issue dengan `ActualTaskDoneDate < BaseDate` tidak masuk agregasi Story Summary. |
| BR-05 | Distribusi SP Eng vs QA mengikuti prefix key (`CPB`, `WB`, `IM`) + marker `[FE]/[BE]/[QA]` di summary. |
| BR-06 | Event grooming difilter dari summary mengandung kata `"grooming"` (case-insensitive). |
| BR-07 | Periode fetch grooming: 1 Januari `FIRST_YEAR` s/d hari ini. |
| BR-08 | Kalender user **harus** di-share ke `GCAL_OWNER_EMAIL` sebelum dapat di-fetch. |
| BR-09 | Project yang dikecualikan dari laporan ditentukan via klausa JQL (`project NOT IN (...)`). |
| BR-10 | Sheet selalu di-clear dan ditulis ulang setiap sync (tidak ada incremental sheet write). |

---

## 10. Risiko & Mitigasi

| Risiko | Dampak | Mitigasi |
|---|---|---|
| Rate limit Jira | Sinkronisasi gagal | Sudah ada retry exponential backoff (max 5x). |
| Token OAuth2 expired | Gagal fetch calendar | Jalankan ulang `gcal-auth <email>` (sudah didokumentasikan). |
| Custom field Jira berubah ID | Data parsing salah / kosong | Konstanta field ID dikelola di `jira/client.go`; perlu review saat ada perubahan workflow. |
| Hari libur tidak update | Perhitungan `day_work_hours` salah di tahun baru | Update `models/holiday.go` setiap awal tahun. |
| Daftar `qaAssignees` outdated | Salah klasifikasi Done Date | Review berkala oleh QA Lead. |
| Kalender tidak di-share ke owner | Event tidak terbaca | Dicatat sebagai warning, tidak menghentikan proses. |

---

## 11. Kriteria Penerimaan (Acceptance Criteria)

1. Menjalankan `go run main.go step-1` berhasil memasukkan/memperbarui issues di tabel `jira_issues`
   tanpa duplikasi.
2. Menjalankan `go run main.go step-2` menghasilkan sheet **"Jira"** dengan header lengkap dan
   seluruh issue dari DB.
3. Menjalankan `go run main.go step-3` menghasilkan sheet **"Story Summary"** dengan satu baris per
   Story, terurut ascending berdasarkan Done Week, dan SLA terisi hanya bila SP > 0.
4. Menjalankan `go run main.go gcal-auth <email>` membuat file token di folder `tokens/`.
5. Menjalankan `go run main.go google-calendar-sync` menampilkan tabel hasil di console dan
   mengisi sheet **"Event Grooming"** untuk seluruh email pada `GCAL_FILTER_EMAILS`.
6. Re-run semua step menghasilkan output yang identik (idempotent).

---

## 12. Glossary

| Istilah | Definisi |
|---|---|
| **FTR (First-Time Right)** | Jam kerja pada jalur normal (Coding + Code Review + Testing) tanpa rework. |
| **Rework** | Jam kerja yang muncul karena bug / pengulangan (Hanging Bug, Fixing, Code Review Bug, Waiting, Retest). |
| **SLA** | Total Hours / Story Point — indikator efisiensi delivery. |
| **Done Week** | Nomor minggu relatif terhadap Base Date saat issue selesai. |
| **Base Date** | Tanggal awal periode pelaporan (dikonfigurasi via env). |
| **PIC Lead Engineer / QA** | Penanggung jawab engineering / QA untuk sebuah Story. |
| **Grooming** | Meeting refinement backlog yang dicatat di Google Calendar. |
