package sheets

import (
	"fmt"
	"jira-sync-eng/models"
	"math"
	"sort"
	"strings"
	"time"

	"google.golang.org/api/sheets/v4"
)

var SUMMARY_HEADERS = []interface{}{
	"Key",               // 0
	"PIC Lead Engineer", // 1
	"PIC Lead QA",       // 2
	"Done Week",         // 3
	"Release Week",      // 4
	"Total SP",          // 5
	"SP QA",             // 6
	"SP Eng",            // 7
	// First-Time Right group
	"Coding Hours",      // 8
	"Code Review Hours", // 9
	"Testing Hours",     // 10
	// Rework group
	"Hanging Bug Hours", // 11
	"Fixing Hours",      // 12
	"Code Review Hours", // 13
	"Waiting Hours",     // 14
	"Retesting Hours",   // 15
	// Rest
	"Bug Count",    // 16
	"Fix Version",  // 17
	"Has FE",       // 18
	"Status Story", // 19
	"Total Hours",  // 20
	"SLA Eng",      // 21
	"SLA QA",       // 22
	"Total SLA",    // 23
}

var keyFilters = []string{"IM", "CPB", "WB"}

type storyBase struct {
	Key         string
	PicEng      string
	PicQA       string
	DoneWeek    interface{}
	ReleaseWeek string
	FixVersions string
	Status      string
}

type storyAgg struct {
	SP    float64
	SPQA  float64
	SPEng float64
	// First-Time Right
	Coding     float64
	CodeReview float64
	Testing    float64
	// Rework
	HangingBug   float64
	Fixing       float64
	CodeRevBug   float64
	WaitingHours float64
	Retest       float64
	BugCount     int
	HasFE        bool
}

func (c *Client) SyncStorySummary(sheetName string, issues []models.JiraIssue, baseDate time.Time) error {
	// ── STEP 1: Collect stories ───────────────────────────────
	var storyOrder []string
	storyBaseMap := map[string]*storyBase{}
	aggMap := map[string]*storyAgg{}

	for _, issue := range issues {
		if issue.IssueType != "Story" {
			continue
		}
		if _, exists := storyBaseMap[issue.Key]; exists {
			continue
		}
		doneWeek := interface{}(nil)
		if issue.DoneWeek != nil {
			doneWeek = *issue.DoneWeek
		}
		storyOrder = append(storyOrder, issue.Key)
		storyBaseMap[issue.Key] = &storyBase{
			Key:         issue.Key,
			PicEng:      issue.PicLeadEngineer,
			PicQA:       issue.PicLeadQA,
			DoneWeek:    doneWeek,
			ReleaseWeek: issue.ReleaseWeek,
			FixVersions: issue.FixVersions,
			Status:      issue.StatusStory,
		}
	}

	// ── STEP 2: Aggregate child issues ───────────────────────
	skippedParents := map[string]int{}
	for _, issue := range issues {
		if issue.IssueType == "Story" {
			continue
		}
		if _, ok := storyBaseMap[issue.Parent]; !ok {
			skippedParents[issue.Parent]++
			continue
		}

		if issue.ActualTaskDoneDate != "" {
			var doneDate time.Time
			var parseErr error
			for _, layout := range []string{"2006-01-02 15:04:05", "2006-01-02T15:04:05Z07:00", "2006-01-02"} {
				doneDate, parseErr = time.Parse(layout, issue.ActualTaskDoneDate)
				if parseErr == nil {
					break
				}
			}
			if parseErr == nil && doneDate.Before(baseDate) {
				continue
			}
		}

		if aggMap[issue.Parent] == nil {
			aggMap[issue.Parent] = &storyAgg{}
		}
		a := aggMap[issue.Parent]
		summaryUpper := strings.ToUpper(issue.Summary)

		sp := 0.0
		if issue.StoryPoint != nil {
			sp = *issue.StoryPoint
		}

		if strings.Contains(summaryUpper, "[FE]") {
			a.HasFE = true
		}

		a.SP += sp
		if issue.IssueType == "Bug" {
			a.BugCount++
		}

		isFiltered := false
		for _, f := range keyFilters {
			if strings.Contains(issue.Key, f) {
				isFiltered = true
				break
			}
		}

		if isFiltered {
			isCPBorWB := strings.Contains(issue.Key, "CPB") || strings.Contains(issue.Key, "WB")
			if isCPBorWB {
				if issue.IssueType == "Task" {
					a.SPEng += sp
				} else if issue.IssueType == "Sub-task" {
					if strings.Contains(summaryUpper, "[FE]") || strings.Contains(summaryUpper, "[BE]") {
						a.SPEng += sp
					} else {
						a.SPQA += sp
					}
				} else {
					a.SPEng += sp
				}
			} else {
				if issue.IssueType == "Task" {
					a.SPEng += sp
				} else if issue.IssueType == "Sub-task" {
					if strings.Contains(summaryUpper, "[QA]") {
						a.SPQA += sp
					} else {
						a.SPEng += sp
					}
				} else {
					a.SPEng += sp
				}
			}
		} else {
			if issue.IssueType == "Sub-task Engineer" {
				a.SPEng += sp
			} else {
				a.SPQA += sp
			}
		}

		if issue.CodingHours != nil {
			a.Coding += *issue.CodingHours
		}
		if issue.CodeReviewDayWorkHours != nil {
			a.CodeReview += *issue.CodeReviewDayWorkHours
		}
		if issue.TestingHours != nil {
			a.Testing += *issue.TestingHours
		}
		if issue.HangingBugByEngDayWorkHours != nil {
			a.HangingBug += *issue.HangingBugByEngDayWorkHours
		}
		if issue.FixingHours != nil {
			a.Fixing += *issue.FixingHours
		}
		if issue.CodeReviewBugDayWorkHours != nil {
			a.CodeRevBug += *issue.CodeReviewBugDayWorkHours
		}
		if issue.HangingBugByQADayWorkHours != nil {
			a.WaitingHours += *issue.HangingBugByQADayWorkHours
		}
		if issue.RetestHours != nil {
			a.Retest += *issue.RetestHours
		}
	}

	// ── STEP 3: Sort by doneWeek ASC ─────────────────────────
	if len(skippedParents) > 0 {
		fmt.Printf("[WARN] %d child issues skipped (parent story not found):\n", func() int {
			n := 0
			for _, c := range skippedParents {
				n += c
			}
			return n
		}())
		for parent, count := range skippedParents {
			fmt.Printf("  parent=%q  skipped_children=%d\n", parent, count)
		}
	}
	sort.SliceStable(storyOrder, func(i, j int) bool {
		return parseDoneWeek(storyBaseMap[storyOrder[i]].DoneWeek) < parseDoneWeek(storyBaseMap[storyOrder[j]].DoneWeek)
	})

	// ── STEP 4: Build output rows ─────────────────────────────
	rows := make([][]interface{}, 0, len(storyOrder))
	for _, key := range storyOrder {
		base := storyBaseMap[key]
		a := aggMap[key]
		if a == nil {
			a = &storyAgg{}
		}

		ftrHours := a.Coding + a.CodeReview + a.Testing
		reworkHours := a.HangingBug + a.Fixing + a.CodeRevBug + a.WaitingHours + a.Retest
		totalHours := ftrHours + reworkHours

		totalEngineerHours := a.Coding + a.CodeReview + a.Fixing + a.CodeRevBug + a.HangingBug
		totalQAHours := a.Testing + a.WaitingHours + a.Retest

		slaEng := interface{}(nil)
		if a.SPEng > 0 {
			slaEng = round2(totalEngineerHours / a.SPEng)
		}
		slaQA := interface{}(nil)
		if a.SPQA > 0 {
			slaQA = round2(totalQAHours / a.SPQA)
		}
		totalSLA := interface{}(nil)
		if a.SP > 0 {
			totalSLA = round2(totalHours / a.SP)
		}

		hasFE := ""
		if a.HasFE {
			hasFE = "Yes"
		}

		rows = append(rows, []interface{}{
			key, base.PicEng, base.PicQA, base.DoneWeek, base.ReleaseWeek,
			round2(a.SP), round2(a.SPQA), round2(a.SPEng),
			round2(a.Coding), round2(a.CodeReview), round2(a.Testing),
			round2(a.HangingBug), round2(a.Fixing), round2(a.CodeRevBug), round2(a.WaitingHours), round2(a.Retest),
			a.BugCount, base.FixVersions, hasFE, base.Status,
			round2(totalHours), slaEng, slaQA, totalSLA,
		})
	}

	// ── STEP 5: Clear and recreate sheet ─────────────────────
	if err := c.clearSheet(sheetName, len(rows)+2); err != nil {
		return err
	}

	// ── STEP 6: Apply merges & formatting ────────────────────
	sheetID, err := c.getSheetID(sheetName)
	if err != nil {
		return err
	}
	if err = c.applyGroupHeaders(sheetID); err != nil {
		return err
	}

	// ── STEP 7: Write row 1 (hanya group labels, sisanya kosong) ──
	row1 := make([]interface{}, len(SUMMARY_HEADERS))
	// Semua kosong by default, hanya isi group labels
	row1[8] = "First-Time Right"
	row1[11] = "Rework"

	_, err = c.service.Spreadsheets.Values.Update(
		c.spreadsheetID, sheetName+"!A1",
		&sheets.ValueRange{Values: [][]interface{}{row1}},
	).ValueInputOption("RAW").Do()
	if err != nil {
		return fmt.Errorf("group header write error: %w", err)
	}

	// ── STEP 8: Write row 2 (semua detail headers) ────────────
	row2 := make([]interface{}, len(SUMMARY_HEADERS))
	copy(row2, SUMMARY_HEADERS) // semua label ada di row 2
	// Override group cols dengan detail headers
	row2[8] = "Coding Hours"
	row2[9] = "Code Review Hours"
	row2[10] = "Testing Hours"
	row2[11] = "Hanging Bug Hours"
	row2[12] = "Fixing Hours"
	row2[13] = "Code Review Hours"
	row2[14] = "Waiting Hours"
	row2[15] = "Retesting Hours"

	_, err = c.service.Spreadsheets.Values.Update(
		c.spreadsheetID, sheetName+"!A2",
		&sheets.ValueRange{Values: [][]interface{}{row2}},
	).ValueInputOption("RAW").Do()
	if err != nil {
		return fmt.Errorf("header row 2 error: %w", err)
	}

	if len(rows) == 0 {
		return nil
	}

	// ── STEP 9: Write data in chunks (starting row 3) ─────────
	const chunkSize = 1000
	for i := 0; i < len(rows); i += chunkSize {
		end := i + chunkSize
		if end > len(rows) {
			end = len(rows)
		}
		chunk := make([][]interface{}, end-i)
		copy(chunk, rows[i:end])

		dataRange := fmt.Sprintf("%s!A%d", sheetName, i+3)
		_, err = c.service.Spreadsheets.Values.Update(
			c.spreadsheetID, dataRange,
			&sheets.ValueRange{Values: chunk},
		).ValueInputOption("RAW").Do()
		if err != nil {
			return fmt.Errorf("write chunk %d error: %w", i/chunkSize, err)
		}
		fmt.Printf("Story Summary written: %d/%d rows\n", end, len(rows))
	}

	// ── STEP 10: Apply number formatting ──────────────────────
	if err = c.applyStorySummaryNumericFormats(sheetID, len(rows)); err != nil {
		return err
	}

	fmt.Printf("SyncStorySummary done: %d stories\n", len(rows))
	return nil
}

func (c *Client) applyGroupHeaders(sheetID int64) error {
	// Horizontal merge row 1 untuk group cols SAJA (tidak ada vertical merge)
	mergeH := func(startCol, endCol int64) *sheets.Request {
		return &sheets.Request{
			MergeCells: &sheets.MergeCellsRequest{
				Range: &sheets.GridRange{
					SheetId:          sheetID,
					StartRowIndex:    0,
					EndRowIndex:      1,
					StartColumnIndex: startCol,
					EndColumnIndex:   endCol,
				},
				MergeType: "MERGE_ALL",
			},
		}
	}

	centerFmt := func(startRow, endRow, startCol, endCol int64) *sheets.Request {
		return &sheets.Request{
			RepeatCell: &sheets.RepeatCellRequest{
				Range: &sheets.GridRange{
					SheetId:          sheetID,
					StartRowIndex:    startRow,
					EndRowIndex:      endRow,
					StartColumnIndex: startCol,
					EndColumnIndex:   endCol,
				},
				Cell: &sheets.CellData{
					UserEnteredFormat: &sheets.CellFormat{
						HorizontalAlignment: "CENTER",
						VerticalAlignment:   "MIDDLE",
					},
				},
				Fields: "userEnteredFormat.horizontalAlignment,userEnteredFormat.verticalAlignment",
			},
		}
	}

	boldFmt := func(startRow, endRow, startCol, endCol int64) *sheets.Request {
		return &sheets.Request{
			RepeatCell: &sheets.RepeatCellRequest{
				Range: &sheets.GridRange{
					SheetId:          sheetID,
					StartRowIndex:    startRow,
					EndRowIndex:      endRow,
					StartColumnIndex: startCol,
					EndColumnIndex:   endCol,
				},
				Cell: &sheets.CellData{
					UserEnteredFormat: &sheets.CellFormat{
						TextFormat: &sheets.TextFormat{Bold: true},
					},
				},
				Fields: "userEnteredFormat.textFormat.bold",
			},
		}
	}

	requests := []*sheets.Request{
		// Horizontal merge group cols (row 1 only)
		mergeH(8, 11),  // First-Time Right: I–K
		mergeH(11, 16), // Rework: L–P
		// Center group labels row 1
		centerFmt(0, 1, 8, 16),
		// Center group detail headers row 2
		centerFmt(1, 2, 8, 16),
		// Bold group labels row 1
		boldFmt(0, 1, 8, 16),
		// Bold semua cols row 2
		boldFmt(1, 2, 0, int64(len(SUMMARY_HEADERS))),
	}

	_, err := c.service.Spreadsheets.BatchUpdate(c.spreadsheetID, &sheets.BatchUpdateSpreadsheetRequest{
		Requests: requests,
	}).Do()
	if err != nil {
		return fmt.Errorf("apply group headers error: %w", err)
	}
	return nil
}

func (c *Client) applyStorySummaryNumericFormats(sheetID int64, totalRows int) error {
	startRow := int64(2) // row 3 (0-indexed = 2)
	endRow := startRow + int64(totalRows)

	intCols := []int64{
		3,  // Done Week
		16, // Bug Count
	}
	floatCols := []int64{
		5,  // Total SP
		6,  // SP QA
		7,  // SP Eng
		8,  // Coding Hours (FTR)
		9,  // Code Review Hours (FTR)
		10, // Testing Hours (FTR)
		11, // Hanging Bug Hours (Rework)
		12, // Fixing Hours (Rework)
		13, // Code Review Hours (Rework)
		14, // Waiting Hours (Rework)
		15, // Retesting Hours (Rework)
		20, // Total Hours
		21, // SLA Eng
		22, // SLA QA
		23, // Total SLA
	}

	makeRepeat := func(col int64, pattern string) *sheets.Request {
		return &sheets.Request{
			RepeatCell: &sheets.RepeatCellRequest{
				Range: &sheets.GridRange{
					SheetId:          sheetID,
					StartRowIndex:    startRow,
					EndRowIndex:      endRow,
					StartColumnIndex: col,
					EndColumnIndex:   col + 1,
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

	var requests []*sheets.Request
	for _, col := range intCols {
		requests = append(requests, makeRepeat(col, "#,##0"))
	}
	for _, col := range floatCols {
		requests = append(requests, makeRepeat(col, "#,##0.00"))
	}

	_, err := c.service.Spreadsheets.BatchUpdate(c.spreadsheetID, &sheets.BatchUpdateSpreadsheetRequest{
		Requests: requests,
	}).Do()
	if err != nil {
		return fmt.Errorf("apply story summary numeric formats error: %w", err)
	}
	fmt.Println("Story Summary numeric formats applied")
	return nil
}

func parseDoneWeek(val interface{}) int {
	if val == nil {
		return math.MaxInt32
	}
	switch v := val.(type) {
	case int:
		if v == 0 {
			return math.MaxInt32
		}
		return v
	case int64:
		if v == 0 {
			return math.MaxInt32
		}
		return int(v)
	case float64:
		if v == 0 {
			return math.MaxInt32
		}
		return int(v)
	}
	return math.MaxInt32
}

func round2(v float64) float64 {
	return math.Round(v*100) / 100
}
