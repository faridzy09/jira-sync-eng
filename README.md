# jira-sync-eng

Go application untuk mensinkronisasi Jira issues ke PostgreSQL database dan Google Sheets, lengkap dengan agregasi Story Summary per sprint.

---

## Fitur

- **step-1** — Fetch issues dari Jira (via JQL) → parse → upsert ke PostgreSQL
- **step-2** — Baca dari DB → sync ke sheet **"Jira"** (semua kolom detail per issue)
- **step-3** — Baca dari DB → agregasi per Story → sync ke sheet **"Story Summary"**

---

## Prasyarat

| Tool | Versi |
|---|---|
| Go | ≥ 1.21 |
| PostgreSQL | ≥ 13 |
| Google Sheets API | enabled di Google Cloud Console |

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
```

### Google Credentials

1. Buka [Google Cloud Console](https://console.cloud.google.com/)
2. Buat **Service Account** → buat key → download JSON
3. Simpan sebagai `credentials.json` di root project (atau sesuaikan `GOOGLE_CREDENTIALS_PATH`)
4. Share spreadsheet ke email service account tersebut (Editor)

---

## Struktur Project

```
jira-sync-eng/
├── main.go                  # Entry point, routing step-1/2/3
├── .env                     # Konfigurasi (tidak di-commit)
├── credentials.json         # Google Service Account key (tidak di-commit)
├── go.mod
├── go.sum
├── config/
│   └── config.go            # Load .env, GetDoneWeekBaseDate()
├── db/
│   └── db.go                # PostgreSQL repo: CreateTable, Upsert, GetAllForSync
├── jira/
│   └── client.go            # Jira REST API client, ParseIssues, kalkulasi jam & week
├── models/
│   ├── model.go             # Struct JiraIssue
│   └── holiday.go           # Daftar hari libur + IsHoliday()
└── sheet/
    ├── sheet.go             # Google Sheets client, SyncToSheet (sheet "Jira")
    └── story_summary.go     # SyncStorySummary (sheet "Story Summary")
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
Hanya child issues dengan `ActualTaskDoneDate >= FIRST_YEAR/FIRST_MONTH/FIRST_DAY` yang dihitung.

```bash
go run main.go step-3
```

---

## Kolom Sheet "Jira"

Sheet ini berisi satu baris per issue (Task, Sub-task, Bug, Story) dengan kolom meliputi:

| Kolom | Keterangan |
|---|---|
| Key | Jira issue key |
| Summary | Judul issue |
| Issue Type | Task / Sub-task / Bug / Story |
| Status | Status terakhir |
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

Sheet ini berisi satu baris per Story dengan agregasi dari semua child issues:

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
| Code Review Hours & Day Work Hours | Jam code review (total & jam kerja) |
| Hanging Bug By Eng/QA Hours & DW Hours | Jam bug menggantung |
| Total Hours | Total jam semua fase |
| Total Day Work Hours | Total jam kerja efektif |
| SLA (Hours/SP) | Rata-rata jam per SP |
| Bug Count | Jumlah bug yang dibuat |
| Has FE | Ada task frontend (`[FE]`) |
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
```

---

## Catatan

- Pastikan database sudah berjalan sebelum menjalankan step apapun — koneksi DB selalu dibuat di awal.
- Tabel DB dibuat otomatis (`CREATE TABLE IF NOT EXISTS`) saat aplikasi dijalankan.
- `credentials.json` dan `.env` **jangan di-commit** ke repository.
