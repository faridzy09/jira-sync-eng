package sheets

import (
	"context"
	"fmt"
	"jira-sync-eng/models"
	"os"
	"strconv"
	"time"

	"golang.org/x/oauth2/google"
	"google.golang.org/api/googleapi"
	"google.golang.org/api/option"
	"google.golang.org/api/sheets/v4"
)

type Client struct {
	service       *sheets.Service
	spreadsheetID string
}

func NewClient(credentialsPathOrJSON, spreadsheetID string) (*Client, error) {
	ctx := context.Background()

	// Support both: path to file or raw JSON string
	var credsJSON []byte
	if len(credentialsPathOrJSON) > 0 && credentialsPathOrJSON[0] == '{' {
		credsJSON = []byte(credentialsPathOrJSON)
	} else {
		data, err := os.ReadFile(credentialsPathOrJSON)
		if err != nil {
			return nil, fmt.Errorf("read credentials file error: %w", err)
		}
		credsJSON = data
	}

	creds, err := google.CredentialsFromJSON(ctx, credsJSON, sheets.SpreadsheetsScope)
	if err != nil {
		return nil, fmt.Errorf("credentials error: %w", err)
	}

	svc, err := sheets.NewService(ctx, option.WithCredentials(creds))
	if err != nil {
		return nil, fmt.Errorf("sheets service error: %w", err)
	}

	return &Client{service: svc, spreadsheetID: spreadsheetID}, nil
}

var HEADERS = []interface{}{
	"Key", "Issue Type", "Summary", "Assignee", "PIC Lead Engineer",
	"Status Category Changed", "Done Week", "Fix versions",
	"fixVersion.released", "fixVersion.releaseDate", "Release Week",
	"Story Point", "From Type", "Parent", "Coding Hours",
	"Code Review Hours", "Code Review Day Work Hours", "Testing Hours",
	"First Bug Ready to Test Date", "First Retest Date",
	"Hanging Bug By Eng Hours", "Hanging Bug By Eng Day Work Hours",
	"Hanging Bug By QA Hours", "Hanging Bug By QA Day Work Hours",
	"Code Review Bug Hours", "Code Review Bug Day Work Hours",
	"Fixing Hours", "Retest Hours",
	"Count Fix Version", "Additional Task", "Accident Bug",
	"Bug From Category", "PIC Lead QA", "Actual Task Start Date",
	"Actual Task Done Date", "Actual Task Done Week",
	"Actual Task Done Month", "Actual Task Done Year",
	"Task Status", "Status Story",
}

// clearSheet membersihkan semua konten dan format sheet, serta expand baris jika kurang.
// Mengembalikan sheetID agar pemanggil tidak perlu memanggil getSheetID secara terpisah.
// rowCount: jumlah baris yang dibutuhkan.
func (c *Client) clearSheet(sheetName string, rowCount int) (int64, error) {
	ss, err := c.service.Spreadsheets.Get(c.spreadsheetID).Do()
	if err != nil {
		return 0, fmt.Errorf("get spreadsheet error: %w", err)
	}

	var sheetID int64 = -1
	var currentRows int64
	for _, s := range ss.Sheets {
		if s.Properties.Title == sheetName {
			sheetID = s.Properties.SheetId
			currentRows = s.Properties.GridProperties.RowCount
			break
		}
	}

	if sheetID == -1 {
		// Sheet belum ada, buat baru
		neededRows := int64(rowCount) + 10
		if neededRows < 1000 {
			neededRows = 1000
		}
		res, err := c.service.Spreadsheets.BatchUpdate(c.spreadsheetID, &sheets.BatchUpdateSpreadsheetRequest{
			Requests: []*sheets.Request{{
				AddSheet: &sheets.AddSheetRequest{
					Properties: &sheets.SheetProperties{
						Title: sheetName,
						GridProperties: &sheets.GridProperties{
							RowCount:    neededRows,
							ColumnCount: 50,
						},
					},
				},
			}},
		}).Do()
		if err != nil {
			return 0, fmt.Errorf("add sheet error: %w", err)
		}
		// Ambil sheetID dari response agar tidak perlu API call tambahan
		if len(res.Replies) > 0 && res.Replies[0].AddSheet != nil {
			sheetID = res.Replies[0].AddSheet.Properties.SheetId
		}
		fmt.Printf("Sheet '%s' created\n", sheetName)
		return sheetID, nil
	}

	neededRows := int64(rowCount) + 10
	if neededRows < 1000 {
		neededRows = 1000
	}
	// Gunakan max antara currentRows dan neededRows agar semua baris lama ikut ter-cover
	totalRows := currentRows
	if neededRows > totalRows {
		totalRows = neededRows
	}

	var requests []*sheets.Request

	// Expand baris jika kurang
	if currentRows < neededRows {
		requests = append(requests, &sheets.Request{
			UpdateSheetProperties: &sheets.UpdateSheetPropertiesRequest{
				Properties: &sheets.SheetProperties{
					SheetId: sheetID,
					GridProperties: &sheets.GridProperties{
						RowCount:    neededRows,
						ColumnCount: 50,
					},
				},
				Fields: "gridProperties.rowCount,gridProperties.columnCount",
			},
		})
	}

	// Clear format dengan bounds eksplisit (mencakup semua baris yang pernah ada)
	requests = append(requests, &sheets.Request{
		RepeatCell: &sheets.RepeatCellRequest{
			Range: &sheets.GridRange{
				SheetId:          sheetID,
				StartRowIndex:    0,
				EndRowIndex:      totalRows,
				StartColumnIndex: 0,
				EndColumnIndex:   50,
			},
			Cell:   &sheets.CellData{},
			Fields: "userEnteredValue,userEnteredFormat",
		},
	})

	if err = c.retryBatchUpdate(&sheets.BatchUpdateSpreadsheetRequest{Requests: requests}); err != nil {
		return 0, fmt.Errorf("clear sheet error: %w", err)
	}

	// Clear konten dengan Values API (lebih reliable untuk menghapus semua value)
	clearRange := fmt.Sprintf("%s!A1:AX%d", sheetName, totalRows)
	_, err = c.service.Spreadsheets.Values.Clear(c.spreadsheetID, clearRange, &sheets.ClearValuesRequest{}).Do()
	if err != nil {
		return 0, fmt.Errorf("clear values error: %w", err)
	}

	fmt.Printf("Sheet '%s' cleared\n", sheetName)
	return sheetID, nil
}

// getSheetID returns the sheetId for the given sheet name.
// Digunakan di luar SyncToSheet/SyncStorySummary jika perlu.
func (c *Client) getSheetID(sheetName string) (int64, error) {
	ss, err := c.service.Spreadsheets.Get(c.spreadsheetID).Do()
	if err != nil {
		return 0, fmt.Errorf("get spreadsheet error: %w", err)
	}
	for _, s := range ss.Sheets {
		if s.Properties.Title == sheetName {
			return s.Properties.SheetId, nil
		}
	}
	return 0, fmt.Errorf("sheet '%s' not found", sheetName)
}

// getSheetRowCount mengembalikan jumlah baris yang ada di sheet, 0 jika sheet belum ada.
func (c *Client) getSheetRowCount(sheetName string) (int, error) {
	ss, err := c.service.Spreadsheets.Get(c.spreadsheetID).Do()
	if err != nil {
		return 0, err
	}
	for _, s := range ss.Sheets {
		if s.Properties.Title == sheetName {
			return int(s.Properties.GridProperties.RowCount), nil
		}
	}
	return 0, nil
}

// retryBatchUpdate retries a BatchUpdate on transient 5xx errors with exponential backoff.
func (c *Client) retryBatchUpdate(req *sheets.BatchUpdateSpreadsheetRequest) error {
	const maxAttempts = 5
	delay := 2 * time.Second
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		_, err := c.service.Spreadsheets.BatchUpdate(c.spreadsheetID, req).Do()
		if err == nil {
			return nil
		}
		if apiErr, ok := err.(*googleapi.Error); ok && apiErr.Code >= 500 {
			if attempt == maxAttempts {
				return err
			}
			fmt.Printf("BatchUpdate attempt %d failed (%d), retrying in %s...\n", attempt, apiErr.Code, delay)
			time.Sleep(delay)
			delay *= 2
			continue
		}
		return err
	}
	return nil
}

// applyNumericFormats menerapkan format angka ke kolom numerik di sheet Jira.
// Kolom yang berurutan (adjacent) digabung dalam satu request agar lebih efisien.
// intCols   → #,##0      (Done Week, Release Week, Count Fix Version, dll)
// floatCols → #,##0.00   (Story Point, semua kolom Hours)
func (c *Client) applyNumericFormats(sheetID int64, totalRows int) error {
	endRow := int64(totalRows) + 1 // baris header (1) + baris data

	// makeRange membuat satu RepeatCell request untuk range kolom [startCol, endCol).
	makeRange := func(startCol, endCol int64, pattern string) *sheets.Request {
		return &sheets.Request{
			RepeatCell: &sheets.RepeatCellRequest{
				Range: &sheets.GridRange{
					SheetId:          sheetID,
					StartRowIndex:    1, // lewati baris header
					EndRowIndex:      endRow,
					StartColumnIndex: startCol,
					EndColumnIndex:   endCol,
				},
				Cell: &sheets.CellData{
					UserEnteredFormat: &sheets.CellFormat{
						NumberFormat: &sheets.NumberFormat{
							Type:    "NUMBER",
							Pattern: pattern,
						},
					},
				},
				Fields: "userEnteredFormat.numberFormat",
			},
		}
	}

	// Kolom integer — tidak ada yang berurutan, satu request per kolom
	// Indeks kolom (0-based) harus sesuai urutan HEADERS
	intRequests := []*sheets.Request{
		makeRange(6, 7, "#,##0"),   // Done Week
		makeRange(10, 11, "#,##0"), // Release Week
		makeRange(28, 29, "#,##0"), // Count Fix Version
		makeRange(35, 36, "#,##0"), // Actual Task Done Week
		// 36 = Actual Task Done Month (teks, skip)
		makeRange(37, 38, "#,##0"), // Actual Task Done Year
	}

	// Kolom float — kolom berurutan digabung dalam satu request agar hemat API call:
	//   Kolom 14–17 (Coding, Code Review, Code Review Day Work, Testing Hours)
	//   Kolom 20–27 (Hanging Bug Eng, Eng Day Work, QA, QA Day Work,
	//                Code Review Bug, Code Review Bug Day Work, Fixing, Retest Hours)
	// Total: 19 request → 8 request
	floatRequests := []*sheets.Request{
		makeRange(11, 12, "#,##0.00"), // Story Point
		// 12 = From Type (teks), 13 = Parent (teks) → skip
		makeRange(14, 18, "#,##0.00"), // Coding … Testing Hours (4 kolom sekaligus)
		// 18 = First Bug Ready to Test Date (teks), 19 = First Retest Date (teks) → skip
		makeRange(20, 28, "#,##0.00"), // Hanging Bug … Retest Hours (8 kolom sekaligus)
	}

	requests := append(intRequests, floatRequests...)
	if err := c.retryBatchUpdate(&sheets.BatchUpdateSpreadsheetRequest{Requests: requests}); err != nil {
		return fmt.Errorf("apply numeric formats error: %w", err)
	}
	fmt.Println("Numeric formats applied")
	return nil
}

func (c *Client) SyncToSheet(sheetName string, issues []models.JiraIssue) error {
	// 1. Hapus dan buat ulang sheet — sheetID langsung didapat, tidak perlu API call tambahan
	var err error
	sheetID, err := c.clearSheet(sheetName, len(issues)+1)
	if err != nil {
		return err
	}

	// 2. Tulis header
	headerRange := sheetName + "!A1"
	_, err = c.service.Spreadsheets.Values.Update(
		c.spreadsheetID,
		headerRange,
		&sheets.ValueRange{Values: [][]interface{}{HEADERS}},
	).ValueInputOption("RAW").Do()
	if err != nil {
		return fmt.Errorf("header error: %w", err)
	}

	// 3. Tulis data dalam batch 1000
	const chunkSize = 1000
	for i := 0; i < len(issues); i += chunkSize {
		end := i + chunkSize
		if end > len(issues) {
			end = len(issues)
		}
		chunk := issues[i:end]

		rows := make([][]interface{}, len(chunk))
		for j, issue := range chunk {
			rows[j] = issueToRow(issue)
		}

		startRow := i + 2 // row 1 = header
		dataRange := fmt.Sprintf("%s!A%d", sheetName, startRow)

		_, err = c.service.Spreadsheets.Values.Update(
			c.spreadsheetID,
			dataRange,
			&sheets.ValueRange{Values: rows},
		).ValueInputOption("RAW").Do()
		if err != nil {
			return fmt.Errorf("write chunk %d error: %w", i/chunkSize, err)
		}

		fmt.Printf("Sheet written: %d/%d rows\n", end, len(issues))
	}

	// 4. Terapkan number formatting ke kolom numerik
	if err := c.applyNumericFormats(sheetID, len(issues)); err != nil {
		return err
	}

	return nil
}

func issueToRow(i models.JiraIssue) []interface{} {
	nullStr := func(s string) interface{} {
		if s == "" {
			return nil
		}
		return s
	}
	nullFloat := func(f *float64) interface{} {
		if f == nil {
			return nil
		}
		return *f
	}
	nullInt := func(n *int) interface{} {
		if n == nil {
			return nil
		}
		return *n
	}

	return []interface{}{
		i.Key, i.IssueType, i.Summary, i.Assignee, i.PicLeadEngineer,
		nullStr(func() string {
			if i.StatusCategoryChanged == nil {
				return ""
			}
			return i.StatusCategoryChanged.Format("2006-01-02 15:04:05")
		}()),
		nullInt(i.DoneWeek), i.FixVersions,
		i.FixVersionReleased, i.FixVersionReleaseDate, i.ReleaseWeek,
		nullFloat(i.StoryPoint), nullStr(i.FromType), i.Parent,
		nullFloat(i.CodingHours), nullFloat(i.CodeReviewHours), nullFloat(i.CodeReviewDayWorkHours),
		nullFloat(i.TestingHours), nullStr(i.FirstReadyToTestBugDate), nullStr(i.FirstInQABugDate),
		nullFloat(i.HangingBugByEngHours), nullFloat(i.HangingBugByEngDayWorkHours),
		nullFloat(i.HangingBugByQAHours), nullFloat(i.HangingBugByQADayWorkHours),
		nullFloat(i.CodeReviewBugHours), nullFloat(i.CodeReviewBugDayWorkHours),
		nullFloat(i.FixingHours),
		nullFloat(i.RetestHours), nullInt(i.CountFixVersion),
		nullStr(i.AdditionalTask), nullStr(i.AccidentBug),
		nullStr(i.BugFromCategory), nullStr(i.PicLeadQA),
		i.ActualTaskStartDate, i.ActualTaskDoneDate,
		strToInt(i.ActualTaskDoneWeek), nullStr(i.ActualTaskDoneMonth), strToInt(i.ActualTaskDoneYear),
		i.TaskStatus, i.StatusStory,
	}
}

// strToInt converts a string to int for sheet numeric cells.
// Returns nil if the string is empty or not a valid integer.
func strToInt(s string) interface{} {
	if s == "" {
		return nil
	}
	n, err := strconv.Atoi(s)
	if err != nil {
		return nil
	}
	return n
}
