package sheets

import (
	"fmt"
	"jira-sync-eng/models"
	"sort"
	"strings"

	"google.golang.org/api/sheets/v4"
)

// ClassifyBug mengklasifikasikan bug berdasarkan JENIS masalahnya (bukan fitur).
// Pass 1: summary saja.
// Pass 2: description sebagai fallback — aman karena kategori baru tidak pakai
//
//	keyword generik (login/token/session) yang sering muncul di repro steps.
func ClassifyBug(summary, description, _ string) string {
	if cat := classifyText(strings.ToLower(summary)); cat != "Other" {
		return cat
	}
	if description != "" {
		if cat := classifyText(strings.ToLower(description)); cat != "Other" {
			return cat
		}
	}
	return "Other"
}

// classifyText menerapkan aturan klasifikasi ke satu teks (summary atau description).
func classifyText(s string) string {
	// ── 1. UI tidak sesuai Figma/Design ─────────────────────────────────────
	if containsAny(s, "figma", "wording", "typo", "bahasa indonesia",
		"salah bahasa", "masih bahasa", "masih menggunakan bahasa",
		"belum menggunakan bahasa", "font size", "ukuran font",
		"design lama", "format penulisan", "format angka", "format tanggal") {
		return "UI tidak sesuai Figma/Design"
	}
	// "tidak/belum sesuai" + konteks elemen UI
	if containsAny(s, "tidak sesuai", "belum sesuai") &&
		containsAny(s, "design", "figma", "icon", "button", "tombol",
			"ukuran", "posisi", "layout", "alignment", "warna", "teks",
			"wording", "tampilan", "gambar", "foto") {
		return "UI tidak sesuai Figma/Design"
	}
	// "seharusnya" + konteks tampilan/design → design mismatch
	if strings.Contains(s, "seharusnya") &&
		containsAny(s, "figma", "design", "wording", "bahasa", "icon",
			"layout", "ukuran", "posisi", "warna", "font") {
		return "UI tidak sesuai Figma/Design"
	}

	// ── 2. Payload / Field Kurang ────────────────────────────────────────────
	if containsAny(s, "payload", "request body", "tidak terisi", "tidak terset",
		"tidak ter-set", "tidak dikirim", "missing", "properties",
		"body request tidak valid", "request tidak valid") {
		return "Payload / Field Kurang"
	}

	// ── 3. UI tidak bisa diklik ──────────────────────────────────────────────
	if containsAny(s, "tidak bisa klik", "tidak bisa diklik", "tidak bisa di klik",
		"tidak bisa tap", "tidak bisa di-tap", "tidak responsif",
		"tidak bisa scroll", "tidak bisa input") {
		return "UI tidak bisa diklik"
	}
	if containsAny(s, "button", "tombol") && containsAny(s, "disable", "tidak aktif") {
		return "UI tidak bisa diklik"
	}

	// ── 4. Blank Page / Screen ───────────────────────────────────────────────
	if containsAny(s, "blank page", "blank screen", "halaman kosong", "page kosong",
		"layar kosong", "screen kosong") {
		return "Blank Page / Screen"
	}

	// ── 5. Konten Terpotong ──────────────────────────────────────────────────
	if strings.Contains(s, "terpotong") {
		return "Konten Terpotong"
	}

	// ── 6. Elemen UI Tertutup / Terhalang ────────────────────────────────────
	if containsAny(s, "menutupi", "terhalang", "tercover") {
		return "Elemen UI Tertutup / Terhalang"
	}
	// "tertutup" tanpa konteks "tidak tertutup" (modal yang tidak bisa ditutup → Functional Error)
	if strings.Contains(s, "tertutup") && !strings.Contains(s, "tidak tertutup") {
		return "Elemen UI Tertutup / Terhalang"
	}

	// ── 7. Notifikasi / Toast tidak muncul ───────────────────────────────────
	if containsAny(s, "toast", "snackbar") {
		return "Notifikasi / Toast tidak muncul"
	}
	if containsAny(s, "notif", " pn ", "push notification") &&
		containsAny(s, "tidak muncul", "belum muncul", "tidak tampil", "belum tampil") {
		return "Notifikasi / Toast tidak muncul"
	}
	if containsAny(s, "popup", "pop up", "modal sukses", "modal gagal",
		"notifikasi sukses", "notifikasi gagal") &&
		containsAny(s, "tidak muncul", "belum muncul", "tidak tampil", "belum tampil") {
		return "Notifikasi / Toast tidak muncul"
	}

	// ── 8. Pesan Error / Validasi tidak muncul ───────────────────────────────
	if containsAny(s, "validasi", "validation") &&
		containsAny(s, "tidak muncul", "belum muncul", "tidak tampil", "belum tampil") {
		return "Pesan Error / Validasi tidak muncul"
	}
	if containsAny(s, "error message", "pesan error", "error fullpage", "error page") &&
		containsAny(s, "tidak muncul", "belum muncul", "tidak tampil", "belum tampil") {
		return "Pesan Error / Validasi tidak muncul"
	}

	// ── 9. Badge / Icon / Label tidak muncul ─────────────────────────────────
	if containsAny(s, "badge", "icon") &&
		containsAny(s, "tidak muncul", "belum muncul", "tidak tampil", "belum tampil", "tidak terlihat") {
		return "Badge / Icon / Label tidak muncul"
	}
	if strings.Contains(s, "label") &&
		containsAny(s, "tidak muncul", "belum muncul", "tidak tampil", "belum tampil") {
		return "Badge / Icon / Label tidak muncul"
	}

	// ── 10. Tidak Tampil / Blank (umum) ──────────────────────────────────────
	if containsAny(s, "tidak muncul", "tidak tampil", "tidak menampilkan",
		"belum muncul", "belum tampil", "belum ditampilkan", "belum menampilkan",
		"blank", "tidak terlihat", "tidak ter-load", "tidak load",
		"hilang", "tidak ada separator", "belum ada badge",
		"tetap muncul", "tetap tampil") {
		return "Tidak Tampil / Blank"
	}
	// "tampil X" tanpa negasi → sesuatu muncul yang seharusnya tidak / tampil nilai salah
	if strings.Contains(s, "tampil") &&
		containsAny(s, "rp0", "0,00", "kosong", "salah", "tidak seharusnya",
			"padahal", "ketika tidak ada", "ketika kosong", "ketika 0") {
		return "Tidak Tampil / Blank"
	}
	// "muncul" tanpa negasi → menampilkan sesuatu yang seharusnya tidak
	if strings.Contains(s, "muncul") &&
		containsAny(s, "seharusnya tidak", "padahal tidak", "ketika tidak ada",
			"ketika kosong", "ketika 0", "beberapa kali") {
		return "Tidak Tampil / Blank"
	}
	// "seharusnya menampilkan X" → konten yang harusnya ada tapi tidak tampil
	if strings.Contains(s, "seharusnya") &&
		containsAny(s, "menampilkan", "muncul", "tampil", "terlihat") {
		return "Tidak Tampil / Blank"
	}
	// "seharusnya tidak menampilkan" → konten yang tampil tapi tidak seharusnya
	if containsAny(s, "seharusnya tidak muncul", "seharusnya tidak tampil",
		"seharusnya tidak menampilkan", "seharusnya tidak ada",
		"seharusnya disembunyikan", "seharusnya di-hide", "seharusnya hide") {
		return "Tidak Tampil / Blank"
	}

	// ── 5. Tidak dapat diakses / Functional Error ────────────────────────────
	if containsAny(s, "tidak bisa", "gagal", "tidak dapat", "tidak berfungsi",
		"tidak berhasil", "error", "crash", "timeout", "tidak jalan",
		"page broken", "broken", "kena eror", "belum handle", "force back",
		"tidak redirect", "tidak direct", "tidak navigate",
		// Ejaan Indonesia: "eror"
		" eror", "eror ",
		// Force close & app crash
		"force close", "force-close",
		// Navigasi salah
		"tidak diarahkan", "belum diarahkan", "tidak langsung diarahkan",
		"diarahkan ke login", "tidak pindah ke",
		// Loading / infinite scroll
		"infinity scroll", "infinite scroll", "tidak load",
		// Deeplink
		"belum direct ke", "belum diarahkan ke") {
		return "Tidak dapat diakses / Functional Error"
	}
	// "berhasil tapi [masalah]" → aksi sukses tapi hasilnya salah
	if strings.Contains(s, "berhasil") &&
		containsAny(s, "tapi tidak", "namun tidak", "tetapi tidak",
			"tapi masih", "namun masih", "tapi belum", "namun belum") {
		return "Tidak dapat diakses / Functional Error"
	}
	// scroll + masalah
	if strings.Contains(s, "scroll") &&
		containsAny(s, "tidak bisa", "gagal", "tidak jalan", "salah", "tidak") {
		return "Tidak dapat diakses / Functional Error"
	}

	// ── 6. Data Salah / Tidak Akurat ────────────────────────────────────────
	if containsAny(s, "tidak sesuai", "belum sesuai", "data salah",
		"salah hitung", "salah harga", "salah total", "kalkulasi",
		"tidak update", "tidak terupdate", "belum terupdate", "tidak tersimpan",
		"inkonsisten", "tidak konsisten", "duplikat", "duplicate", "null",
		"masih", "salah", "berbeda", "ke reset", "ter-reset", "tereset",
		"belum berganti", "belum ter", "tidak ter",
		// Nilai tidak valid / invalid
		"invalid", "tidak valid", "nilai tidak valid", "tidak sah",
		// Nilai berubah ke yang salah
		"berubah menjadi", "berubah jadi", "berubah ke",
		// Nilai kosong padahal seharusnya ada
		"kosong", "menjadi kosong", "jadi kosong",
		// Format angka / currency salah
		"format", "rp0", "rp 0", "0,00",
		// Data balik/reset ke awal
		"balik ke awal", "kembali ke awal", "kembali seperti semula",
		// Otomatis yang salah
		"tidak otomatis", "tidak auto", "belum otomatis",
		// Nilai yang berubah
		"menjadi invalid", "menjadi 0", "menjadi kosong") {
		return "Data Salah / Tidak Akurat"
	}
	// "seharusnya" tanpa konteks tampilan/design → logika/data yang salah
	if strings.Contains(s, "seharusnya") {
		return "Data Salah / Tidak Akurat"
	}

	// ── Catch-all ────────────────────────────────────────────────────────────
	// Setiap bug pada dasarnya adalah "sesuatu yang salah" dalam data/logika/behavior.
	// Jika tidak cocok kategori spesifik di atas, masuk Data Salah / Tidak Akurat.
	return "Data Salah / Tidak Akurat"
}

// containsAny returns true jika s mengandung salah satu dari substrings.
func containsAny(s string, substrings ...string) bool {
	for _, sub := range substrings {
		if strings.Contains(s, sub) {
			return true
		}
	}
	return false
}

// ClassifySubBug menentukan sub-kategori berdasarkan kategori utama dan teks summary.
// Setiap kategori utama memiliki sub-kategori yang menjelaskan *kenapa* bug terjadi.
func ClassifySubBug(summary, category string) string {
	s := strings.ToLower(summary)

	switch category {

	case "UI tidak sesuai Figma/Design":
		if containsAny(s, "typo", "wording", "bahasa indonesia", "salah bahasa",
			"masih bahasa", "masih menggunakan bahasa", "belum menggunakan bahasa",
			"penamaan") {
			return "Wording / Teks Salah"
		}
		if containsAny(s, "layout", "posisi", "alignment", "spacing",
			"margin", "padding", "overlap") {
			return "Layout / Posisi Salah"
		}
		if containsAny(s, "warna", "color") {
			return "Warna Salah"
		}
		if containsAny(s, "ukuran", "font size", "ukuran font") {
			return "Ukuran / Font Salah"
		}
		if containsAny(s, "icon", "gambar", "foto") {
			return "Ikon / Gambar Salah"
		}
		if containsAny(s, "format tanggal", "format angka", "format penulisan") {
			return "Format Tampilan Salah"
		}
		return "Design Umum Tidak Sesuai"

	case "Payload / Field Kurang":
		if containsAny(s, "tidak terisi", "tidak terset", "tidak ter-set",
			"tidak dikirim", "missing") {
			return "Field Tidak Terisi / Tidak Terkirim"
		}
		if containsAny(s, "request tidak valid", "body request tidak valid") {
			return "Request Body Invalid"
		}
		return "Properties / Payload Kurang"

	case "UI tidak bisa diklik":
		if containsAny(s, "disable", "tidak aktif") {
			return "Button / Elemen Disable"
		}
		if containsAny(s, "tidak bisa klik", "tidak bisa diklik", "tidak bisa di klik",
			"tidak bisa tap", "tidak bisa di-tap") {
			return "Tidak Bisa Klik / Tap"
		}
		return "Tidak Responsif / Tidak Bisa Input"

	case "Blank Page / Screen":
		return "Halaman / Layar Kosong"

	case "Konten Terpotong":
		return "Teks / Konten Terpotong"

	case "Elemen UI Tertutup / Terhalang":
		if strings.Contains(s, "keyboard") {
			return "Tertutup Keyboard"
		}
		if containsAny(s, "safe area", "navigation bar", "bottom nav", "navbar") {
			return "Tertutup Safe Area / Nav Bar"
		}
		if strings.Contains(s, "menutupi") {
			return "Elemen Menutupi Konten"
		}
		return "Tertutup Elemen Lain"

	case "Notifikasi / Toast tidak muncul":
		if containsAny(s, "toast", "snackbar") {
			return "Toast / Snackbar Tidak Muncul"
		}
		if containsAny(s, " pn ", "push notification") {
			return "Push Notification Tidak Muncul"
		}
		return "Popup / Modal Tidak Muncul"

	case "Pesan Error / Validasi tidak muncul":
		if containsAny(s, "validasi", "validation") {
			return "Validasi Form Tidak Muncul"
		}
		return "Error Message Tidak Muncul"

	case "Badge / Icon / Label tidak muncul":
		if strings.Contains(s, "badge") {
			return "Badge Tidak Muncul"
		}
		if strings.Contains(s, "icon") {
			return "Icon Tidak Muncul"
		}
		return "Label Tidak Muncul"

	case "Tidak Tampil / Blank":
		if containsAny(s, "foto", "gambar", "video", "banner", "image", "media") {
			return "Foto / Media Tidak Tampil"
		}
		if containsAny(s, "counter", "jumlah", "count") {
			return "Counter / Angka Tidak Tampil"
		}
		if containsAny(s, "data", "list", "daftar") {
			return "Data / List Tidak Tampil"
		}
		if containsAny(s, "informasi", "info ", "detail") {
			return "Informasi / Detail Tidak Tampil"
		}
		return "Konten Tidak Tampil"

	case "Tidak dapat diakses / Functional Error":
		if containsAny(s, "crash", "force close", "force-close") {
			return "App Crash / Force Close"
		}
		if strings.Contains(s, "timeout") {
			return "Timeout"
		}
		if containsAny(s, "tidak redirect", "tidak navigate", "tidak diarahkan",
			"belum diarahkan", "tidak pindah ke", "diarahkan ke login",
			"belum direct ke", "tidak direct") {
			return "Navigasi / Redirect Salah"
		}
		if containsAny(s, "infinity scroll", "infinite scroll") {
			return "Infinite Scroll Bermasalah"
		}
		if containsAny(s, "gagal", "tidak berhasil") {
			return "Aksi Gagal"
		}
		if containsAny(s, "tidak bisa", "tidak dapat") {
			return "Tidak Bisa Melakukan Aksi"
		}
		return "Error / Tidak Berfungsi"

	case "Data Salah / Tidak Akurat":
		if containsAny(s, "salah hitung", "salah total", "kalkulasi", "salah harga") {
			return "Kalkulasi / Hitung Salah"
		}
		if containsAny(s, "tidak tersimpan", "tidak ter-simpan") {
			return "Data Tidak Tersimpan"
		}
		if containsAny(s, "ke reset", "ter-reset", "tereset",
			"balik ke awal", "kembali ke awal", "kembali seperti semula") {
			return "Data Ter-reset / Balik"
		}
		if containsAny(s, "tidak update", "tidak terupdate", "belum terupdate",
			"inkonsisten", "tidak konsisten") {
			return "Data Tidak Sinkron / Update"
		}
		if containsAny(s, "duplikat", "duplicate") {
			return "Data Duplikat"
		}
		if strings.Contains(s, "null") {
			return "Data Null"
		}
		if containsAny(s, "rp0", "rp 0", "0,00", "menjadi 0", "menjadi kosong", "jadi kosong") {
			return "Nilai Jadi 0 / Kosong"
		}
		if strings.Contains(s, "format") {
			return "Format / Nilai Salah"
		}
		if containsAny(s, "tidak sesuai", "belum sesuai") {
			return "Data Tidak Sesuai"
		}
		if strings.Contains(s, "salah") {
			return "Nilai / Data Salah"
		}
		return "Data Tidak Akurat"
	}

	return "-"
}

// ClassifyBEFE menentukan apakah bug cenderung di FE, BE, Both, atau Unknown.
func ClassifyBEFE(summary string, category string) string {
	// Kategori baru berbasis jenis bug sudah langsung mengarah ke area tertentu
	switch category {
	case "UI tidak sesuai Figma/Design", "UI tidak bisa diklik",
		"Blank Page / Screen", "Konten Terpotong", "Elemen UI Tertutup / Terhalang",
		"Badge / Icon / Label tidak muncul", "Tidak Tampil / Blank":
		return "FE"
	case "Notifikasi / Toast tidak muncul", "Pesan Error / Validasi tidak muncul":
		// Bisa FE (render) atau BE (trigger/logic) — dideteksi dari summary
	case "Payload / Field Kurang":
		return "BE"
	}

	// Untuk kategori ambigu, deteksi dari keyword summary
	s := strings.ToLower(summary)
	isFE := containsAny(s, "tampil", "muncul", "blank", "display", "halaman", "page",
		"tombol", "button", "layout", "icon", "ui ", " ui", "frontend",
		"typo", "figma", "wording", "design", "gambar", "foto", "posisi",
		"alignment", "mobile", "flutter", "webview", "web view")
	isBE := containsAny(s, "api ", "backend", "database", " db ", "query", "server",
		"endpoint", "cron", "worker", "webhook", "integrasi", "integration",
		"sync", "sinkron", "null", "duplicate", "duplikat", "payload",
		"response", "request body", "missing data")

	switch {
	case isFE && isBE:
		return "Both"
	case isFE:
		return "FE"
	case isBE:
		return "BE"
	default:
		return "Unknown"
	}
}

type bugRow struct {
	Issue       models.JiraIssue
	Category    string
	SubCategory string
	BEFE        string
}

type categorySummary struct {
	Category   string
	Count      int
	Percentage float64
}

type subCatSummary struct {
	Category    string
	SubCategory string
	Count       int
	Percentage  float64
}

type epicSummary struct {
	EpicKey    string
	Count      int
	Percentage float64
}

type befeSummary struct {
	Label      string
	Count      int
	Percentage float64
}

func (c *Client) SyncBugAnalysis(sheetName string, bugs []models.JiraIssue) error {
	// Classify setiap bug
	rows := make([]bugRow, 0, len(bugs))
	catCountMap := map[string]int{}
	subCatCountMap := map[string]int{} // key: "Kategori||SubKategori"
	epicCountMap := map[string]int{}
	befeCountMap := map[string]int{}

	for _, bug := range bugs {
		cat := ClassifyBug(bug.Summary, bug.Description, bug.Labels)
		sub := ClassifySubBug(bug.Summary, cat)
		befe := ClassifyBEFE(bug.Summary, cat)

		rows = append(rows, bugRow{Issue: bug, Category: cat, SubCategory: sub, BEFE: befe})
		catCountMap[cat]++
		subCatCountMap[cat+"||"+sub]++
		befeCountMap[befe]++

		epicKey := bug.EpicKey
		if epicKey == "" {
			epicKey = "(No Epic)"
		}
		epicCountMap[epicKey]++
	}

	// ── Build summaries ──────────────────────────────────────────
	catSummaries := buildCatSummaries(catCountMap, len(bugs))
	subCatSummaries := buildSubCatSummaries(subCatCountMap, len(bugs))
	epicSummaries := buildEpicSummaries(epicCountMap, len(bugs))
	befeSummaries := buildBEFESummaries(befeCountMap, len(bugs))

	// ── Hitung row positions (1-based) ───────────────────────────
	catTitleRow := 1
	catHeaderRow := catTitleRow + 1
	catDataEnd := catHeaderRow + len(catSummaries)

	subCatTitleRow := catDataEnd + 2
	subCatHeaderRow := subCatTitleRow + 1
	subCatDataEnd := subCatHeaderRow + len(subCatSummaries)

	epicTitleRow := subCatDataEnd + 2
	epicHeaderRow := epicTitleRow + 1
	epicDataEnd := epicHeaderRow + len(epicSummaries)

	befeTitleRow := epicDataEnd + 2
	befeHeaderRow := befeTitleRow + 1
	befeDataEnd := befeHeaderRow + len(befeSummaries)

	detailTitleRow := befeDataEnd + 2
	detailHeaderRow := detailTitleRow + 1
	detailDataStart := detailHeaderRow + 1
	detailDataEnd := detailDataStart + len(rows) - 1

	totalRows := detailDataEnd + 1
	if len(rows) == 0 {
		totalRows = detailHeaderRow + 1
	}

	sheetID, err := c.clearSheet(sheetName, totalRows)
	if err != nil {
		return err
	}

	writeRange := func(r string, vv [][]any) error {
		_, err := c.service.Spreadsheets.Values.Update(
			c.spreadsheetID, sheetName+"!"+r,
			&sheets.ValueRange{Values: vv},
		).ValueInputOption("RAW").Do()
		return err
	}

	// ── Bagian 1: Summary Kategori ───────────────────────────────
	if err := writeRange(fmt.Sprintf("A%d", catTitleRow), [][]any{
		{"KLASIFIKASI BUG BERDASARKAN JUDUL"},
	}); err != nil {
		return fmt.Errorf("cat title error: %w", err)
	}
	if err := writeRange(fmt.Sprintf("A%d", catHeaderRow), [][]any{
		{"Kategori Bug", "Jumlah", "Persentase (%)"},
	}); err != nil {
		return fmt.Errorf("cat header error: %w", err)
	}
	catValues := make([][]any, 0, len(catSummaries))
	for _, s := range catSummaries {
		catValues = append(catValues, []any{s.Category, s.Count, round2(s.Percentage)})
	}
	if err := writeRange(fmt.Sprintf("A%d:C%d", catHeaderRow+1, catDataEnd), catValues); err != nil {
		return fmt.Errorf("cat rows error: %w", err)
	}

	// ── Bagian 2: Summary Sub Kategori ──────────────────────────
	if err := writeRange(fmt.Sprintf("A%d", subCatTitleRow), [][]any{
		{"DETAIL SUB KATEGORI BUG"},
	}); err != nil {
		return fmt.Errorf("subcat title error: %w", err)
	}
	if err := writeRange(fmt.Sprintf("A%d", subCatHeaderRow), [][]any{
		{"Kategori Bug", "Sub Kategori", "Jumlah", "Persentase (%)"},
	}); err != nil {
		return fmt.Errorf("subcat header error: %w", err)
	}
	subCatValues := make([][]any, 0, len(subCatSummaries))
	for _, s := range subCatSummaries {
		subCatValues = append(subCatValues, []any{s.Category, s.SubCategory, s.Count, round2(s.Percentage)})
	}
	if err := writeRange(fmt.Sprintf("A%d:D%d", subCatHeaderRow+1, subCatDataEnd), subCatValues); err != nil {
		return fmt.Errorf("subcat rows error: %w", err)
	}

	// ── Format: bold title & header rows ─────────────────────────
	boldRows := []int64{
		int64(catTitleRow - 1),
		int64(catHeaderRow - 1),
		int64(subCatTitleRow - 1),
		int64(subCatHeaderRow - 1),
		int64(epicTitleRow - 1),
		int64(epicHeaderRow - 1),
		int64(befeTitleRow - 1),
		int64(befeHeaderRow - 1),
		int64(detailTitleRow - 1),
		int64(detailHeaderRow - 1),
	}
	var boldRequests []*sheets.Request
	for _, row := range boldRows {
		boldRequests = append(boldRequests, &sheets.Request{
			RepeatCell: &sheets.RepeatCellRequest{
				Range: &sheets.GridRange{
					SheetId:          sheetID,
					StartRowIndex:    row,
					EndRowIndex:      row + 1,
					StartColumnIndex: 0,
					EndColumnIndex:   11,
				},
				Cell: &sheets.CellData{
					UserEnteredFormat: &sheets.CellFormat{
						TextFormat: &sheets.TextFormat{Bold: true},
					},
				},
				Fields: "userEnteredFormat.textFormat.bold",
			},
		})
	}
	if err := c.retryBatchUpdate(&sheets.BatchUpdateSpreadsheetRequest{Requests: boldRequests}); err != nil {
		return fmt.Errorf("bold format error: %w", err)
	}

	fmt.Printf("Bug Analysis synced: %d bugs, %d kategori, %d sub-kategori, %d epic, BE=%d FE=%d Both=%d Unknown=%d\n",
		len(bugs), len(catSummaries), len(subCatSummaries), len(epicSummaries),
		befeCountMap["BE"], befeCountMap["FE"], befeCountMap["Both"], befeCountMap["Unknown"],
	)
	return nil
}

func buildSubCatSummaries(countMap map[string]int, total int) []subCatSummary {
	summaries := make([]subCatSummary, 0, len(countMap))
	for key, cnt := range countMap {
		parts := strings.SplitN(key, "||", 2)
		cat, sub := parts[0], parts[1]
		pct := 0.0
		if total > 0 {
			pct = float64(cnt) / float64(total) * 100
		}
		summaries = append(summaries, subCatSummary{Category: cat, SubCategory: sub, Count: cnt, Percentage: pct})
	}
	sort.Slice(summaries, func(i, j int) bool {
		if summaries[i].Category != summaries[j].Category {
			return summaries[i].Category < summaries[j].Category
		}
		return summaries[i].Count > summaries[j].Count
	})
	return summaries
}

func buildCatSummaries(countMap map[string]int, total int) []categorySummary {
	summaries := make([]categorySummary, 0, len(countMap))
	for cat, cnt := range countMap {
		pct := 0.0
		if total > 0 {
			pct = float64(cnt) / float64(total) * 100
		}
		summaries = append(summaries, categorySummary{Category: cat, Count: cnt, Percentage: pct})
	}
	sort.Slice(summaries, func(i, j int) bool {
		return summaries[i].Count > summaries[j].Count
	})
	return summaries
}

func buildEpicSummaries(countMap map[string]int, total int) []epicSummary {
	summaries := make([]epicSummary, 0, len(countMap))
	for epic, cnt := range countMap {
		pct := 0.0
		if total > 0 {
			pct = float64(cnt) / float64(total) * 100
		}
		summaries = append(summaries, epicSummary{EpicKey: epic, Count: cnt, Percentage: pct})
	}
	sort.Slice(summaries, func(i, j int) bool {
		if summaries[i].Count != summaries[j].Count {
			return summaries[i].Count > summaries[j].Count
		}
		return summaries[i].EpicKey < summaries[j].EpicKey
	})
	return summaries
}

func buildBEFESummaries(countMap map[string]int, total int) []befeSummary {
	labels := []string{"BE", "FE", "Both", "Unknown"}
	summaries := make([]befeSummary, 0, len(labels))
	for _, label := range labels {
		cnt := countMap[label]
		if cnt == 0 {
			continue
		}
		pct := 0.0
		if total > 0 {
			pct = float64(cnt) / float64(total) * 100
		}
		summaries = append(summaries, befeSummary{Label: label, Count: cnt, Percentage: pct})
	}
	return summaries
}
