# jira-sync-eng

Go application untuk mensinkronisasi Jira issues ke PostgreSQL database dan Google Sheets, lengkap dengan agregasi Story Summary per sprint dan sinkronisasi event Grooming dari Google Calendar.

---

## Fitur

- **step-1** — Fetch issues dari Jira (via JQL) → parse → upsert ke PostgreSQL
- **step-2** — Baca dari DB → sync ke sheet **"Jira"** (semua kolom detail per issue)
- **step-3** — Baca dari DB → agregasi per Story → sync ke sheet **"Story Summary"**
- **gcal-auth** — Otorisasi akun Google Calendar via OAuth2 (jalankan sekali per akun)
- **google-calendar-sync** — Fetch event Grooming dari Google Calendar → simpan ke DB → sync ke sheet **"Event Grooming"**

---

## Prasyarat

| Tool | Versi |
|---|---|
| Go | ≥ 1.21 |
| PostgreSQL | ≥ 13 |
| Google Sheets API | enabled di Google Cloud Console |
| Google Calendar API | enabled di Google Cloud Console |

---

## Instalasi

```bash
git clone <repo-url>
cd jira-sync-eng
go mod download
```

---

## Konfigurasi

Buat file `.env` di root project (atau set sebagai environment variable):

```properties
# ── Jira ──────────────────────────────────────────────────
JIRA_BASE_URL=https://<your-domain>.atlassian.net
JIRA_EMAIL=your-email@domain.com
JIRA_API_TOKEN=your-jira-api-token
JIRA_JQL='((issuetype IN (Sub-task, "Sub-task Engineer", "Task", "Sub-task QA", Bug) AND status CHANGED TO Done DURING ("2026/01/05", "2026/04/19") AND status CHANGED TO ("IN QA", "IN TESTING", "In Progress","Ready To Test","Code Review","Done") AFTER "2026-01-04") OR (issuetype = Story AND statusCategoryChangedDate >= "2026-01-05" AND statusCategoryChangedDate <= "2026-04-19")) AND project NOT IN ("ERP Odoo","Product Design",Principal,"SDET Lion Parcel",SRE,"Production Support",Candi-Rorojongrang,"Product Core","Product Partner Board","Product Partner","Principal Technical Documentation",Principal,"Core System") ORDER BY created asc'

# ── Database (PostgreSQL) ──────────────────────────────────
DB_HOST=localhost
DB_PORT=5432
DB_USER=postgres
DB_NAME=jira-sync-eng

# ── Google Sheets ──────────────────────────────────────────
SPREADSHEET_ID=your-spreadsheet-id
GOOGLE_CREDENTIALS_PATH=credentials.json   # path ke service account JSON

# ── Base Date (untuk kalkulasi Done Week number) ───────────
FIRST_YEAR=2026
FIRST_MONTH=1
FIRST_DAY=5

# ── Sheet Names (opsional, sudah ada default) ──────────────
SHEET_JIRA=Jira
SHEET_STORY_SUMMARY=Story Summary

# ── Google Calendar ────────────────────────────────────────
GCAL_OWNER_EMAIL=your-email@domain.com   # akun yang login & fetch kalender
GCAL_OAUTH2_PATH=oauth2_client.json      # path ke OAuth2 Desktop credentials
```

### Google Credentials (Jira & Sheets)

1. Buka [Google Cloud Console](https://console.cloud.google.com/)
2. Buat **Service Account** → buat key → download JSON
3. Simpan sebagai `credentials.json` di root project (atau sesuaikan `GOOGLE_CREDENTIALS_PATH`)
4. Share spreadsheet ke email service account tersebut (Editor)

### Google Calendar Credentials (OAuth2)

1. Buka [Google Cloud Console](https://console.cloud.google.com/) → project yang sama
2. Pastikan **Google Calendar API** sudah di-enable
3. **APIs & Services** → **Credentials** → **Create Credentials** → **OAuth client ID**
4. Application type: **Desktop app** → beri nama → **Create**
5. Download JSON → rename menjadi **`oauth2_client.json`** → taruh di root project
6. Setiap kalender user yang akan di-fetch harus **di-share** ke `GCAL_OWNER_EMAIL`:
   - Google Calendar → klik nama kalender → **Settings and sharing**
   - **Share with specific people** → masukkan `GCAL_OWNER_EMAIL` → permission: **See all event details**

---

## Struktur Project

```
jira-sync-eng/
├── main.go                  # Entry point, routing semua command
├── .env                     # Konfigurasi (tidak di-commit)
├── credentials.json         # Google Service Account key (tidak di-commit)
├── oauth2_client.json       # Google OAuth2 Desktop credentials (tidak di-commit)
├── tokens/                  # Token OAuth2 per user (di-generate otomatis, tidak di-commit)
├── go.mod
├── go.sum
├── config/
│   └── config.go            # Load .env, GetDoneWeekBaseDate()
├── db/
│   ├── db.go                # PostgreSQL repo: CreateTable, Upsert, GetAllForSync
│   └── gcal.go              # Tabel & upsert google_calendar_events
├── google-calendar/
│   ├── client.go            # Fetch event Grooming dari Google Calendar per user
│   └── auth.go              # OAuth2 flow: authorize user, simpan & load token
├── jira/
│   └── client.go            # Jira REST API client, ParseIssues, kalkulasi jam & week
├── models/
│   ├── model.go             # Struct JiraIssue
│   └── holiday.go           # Daftar hari libur + IsHoliday()
└── sheet/
    ├── sheet.go             # Google Sheets client, SyncToSheet (sheet "Jira")
    ├── story_summary.go     # SyncStorySummary (sheet "Story Summary")
    └── gcal_sheet.go        # SyncGCalEvents (sheet "Event Grooming")
```

---

## Cara Pakai

### Step 1 — Jira → Database

Fetch semua issues dari Jira berdasarkan JQL lalu simpan ke PostgreSQL.

```bash
go run main.go step-1
```

### Step 2 — Database → Sheet "Jira"

Baca semua issues dari DB lalu tulis ke sheet **Jira** di Google Spreadsheet.

```bash
go run main.go step-2
```

### Step 3 — Database → Sheet "Story Summary"

Baca semua issues dari DB, agregasi per Story, lalu tulis ke sheet **Story Summary**.

```bash
go run main.go step-3
```

### gcal-auth — Otorisasi Google Calendar (sekali per akun)

Buka browser untuk login dan menyimpan token OAuth2. Jalankan **sekali** untuk akun yang akan digunakan sebagai `GCAL_OWNER_EMAIL`.

```bash
go run main.go gcal-auth your-email@domain.com
```

Token tersimpan di folder `tokens/` secara otomatis. Jika token expired, jalankan perintah ini lagi.

### google-calendar-sync — Fetch Event Grooming → DB → Sheet

Fetch semua event dengan judul mengandung kata **"grooming"** dari kalender masing-masing user (daftar terdapat di `main.go`), simpan ke database, lalu sync ke sheet **Event Grooming**.

```bash
go run main.go google-calendar-sync
```

Event yang diambil dimulai dari **1 Januari 2026** hingga hari ini.

---

## Tabel Database `google_calendar_events`

| Kolom | Tipe | Keterangan |
|---|---|---|
| `user_email` | TEXT (PK) | Email pemilik kalender |
| `event_id` | TEXT (PK) | ID event Google Calendar |
| `summary` | TEXT | Nama event |
| `start_time` | TIMESTAMPTZ | Waktu mulai |
| `end_time` | TIMESTAMPTZ | Waktu selesai |
| `duration_minutes` | INTEGER | Durasi dalam menit |
| `is_all_day` | BOOLEAN | Event seharian atau tidak |
| `bulan` | TEXT | Nama bulan (e.g. `January`) |
| `tahun` | TEXT | Tahun (e.g. `2026`) |
| `synced_at` | TIMESTAMPTZ | Waktu terakhir sync |

---

## Kolom Sheet "Event Grooming"

| Kolom | Keterangan |
|---|---|
| User Email | Email pemilik kalender |
| Bulan | Nama bulan event |
| Tahun | Tahun event |
| Mulai | Tanggal & jam mulai |
| Selesai | Tanggal & jam selesai |
| Durasi (mnt) | Durasi event dalam menit |
| Nama Event | Judul event di Google Calendar |

---

## Kolom Sheet "Jira"

| Kolom | Keterangan |
|---|---|
| Key | Jira issue key |
| Summary | Judul issue |
| Issue Type | Task / Sub-task / Bug / Story |
| Story Point | SP yang diassign |
| Coding Hours | Durasi fase coding |
| Code Review Hours | Durasi code review |
| Testing Hours | Durasi testing |
| Fixing Hours | Durasi fixing bug |
| Retest Hours | Durasi retest |
| Hanging Bug By Eng Hours | Durasi bug menggantung di sisi engineer |
| Hanging Bug By QA Hours | Durasi bug menggantung di sisi QA |
| Done Week | Nomor minggu penyelesaian (relatif dari base date) |
| Actual Task Done Date | Tanggal & waktu issue selesai |
| ... | *(dan kolom lainnya)* |

---

## Kolom Sheet "Story Summary"

| Kolom | Keterangan |
|---|---|
| Key | Story key |
| PIC Lead Engineer | Nama lead engineer |
| PIC Lead QA | Nama lead QA |
| Done Week | Nomor minggu Story selesai |
| Release Week | Minggu release |
| Total SP | Total story point |
| SP QA / SP Eng | SP terdistribusi per role |
| Coding / Testing / Fixing / Retest Hours | Akumulasi jam per fase |
| Hanging Bug By Eng/QA Hours | Jam bug menggantung |
| Total Hours | Total jam semua fase |
| SLA (Hours/SP) | Rata-rata jam per SP |
| Bug Count | Jumlah bug yang dibuat |
| Status Story | Status Story di Jira |

---

## Kalkulasi Jam Kerja

Jam kerja efektif dihitung dengan aturan:
- **Hari kerja**: Senin–Jumat
- **Jam kerja**: 09:00–18:00 WIB
- **Libur nasional**: skip (lihat `models/holiday.go`)

---

## Build

```bash
go build -o jira-sync-eng .
./jira-sync-eng step-1
./jira-sync-eng step-2
./jira-sync-eng step-3
./jira-sync-eng gcal-auth your-email@domain.com
./jira-sync-eng google-calendar-sync
```

---

## Catatan

- Pastikan database sudah berjalan sebelum menjalankan step apapun — koneksi DB selalu dibuat di awal.
- Tabel DB dibuat otomatis (`CREATE TABLE IF NOT EXISTS`) saat aplikasi dijalankan.
- `credentials.json`, `oauth2_client.json`, `.env`, dan folder `tokens/` **jangan di-commit** ke repository.
