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
	"Epic Key",          // 1
	"PIC Lead Engineer", // 2
	"PIC Lead QA",       // 3
	"Done Week",         // 4
	"Release Week",      // 5
	"Total SP",          // 6
	"SP QA",             // 7
	"SP Eng",            // 8
	// First-Time Right group
	"Coding Hours",      // 9
	"Code Review Hours", // 10
	"Testing Hours",     // 11
	// Rework group
	"Hanging Bug Hours", // 12
	"Fixing Hours",      // 13
	"Code Review Hours", // 14
	"Waiting Hours",     // 15
	"Retesting Hours",   // 16
	// Rest
	"Bug Count",    // 17
	"Fix Version",  // 18
	"Has FE",       // 19
	"Status Story", // 20
	// Current Formula group
	"Total Hours", // 21
	"SLA Eng",     // 22
	"SLA QA",      // 23
	"SLA All",     // 24
	// New Formula group
	"Total Hours", // 25
	"SLA Eng",     // 26
	"SLA QA",      // 27
	"SLA All",     // 28
	// Development dates
	"Start Development", // 29
	"End Development",   // 30
	// Testing dates
	"Start Testing", // 31
	"End Testing",   // 32
	"Story From",    // 33
	// Missed / Accident Bug breakdown
	"Total Missed PRD",       // 34
	"Total Missed TRD",       // 35
	"Total Missed Coding",    // 36
	"Total Missed Test Case", // 37
	"Total Other Issue",      // 38
}

var keyFilters = []string{"IM", "CPB", "WB"}

type storyBase struct {
	Key         string
	EpicKey     string
	PicEng      string
	PicQA       string
	DoneWeek    interface{}
	ReleaseWeek string
	FixVersions string
	Status      string
	StoryFrom   string
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
	// Accident Bug breakdown (per story)
	MissedPRD      int
	MissedTRD      int
	MissedCoding   int
	MissedTestCase int
	OtherIssue     int
	HasFE          bool
	StartDev       string // earliest ActualTaskStartDate (yyyy-mm-dd)
	EndDev         string // latest ActualTaskDoneDate (yyyy-mm-dd)
	StartTesting   string // earliest ActualTaskStartDate of QA tasks (yyyy-mm-dd)
	EndTesting     string // latest ActualTaskDoneDate of QA tasks (yyyy-mm-dd)
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
		if strings.Contains(issue.Key, "TITAN") {
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
			EpicKey:     issue.EpicKey,
			PicEng:      issue.PicLeadEngineer,
			PicQA:       issue.PicLeadQA,
			DoneWeek:    doneWeek,
			ReleaseWeek: issue.ReleaseWeek,
			FixVersions: issue.FixVersions,
			Status:      issue.StatusStory,
			StoryFrom:   issue.FromType,
		}
	}

	// ── STEP 1b: Derive Story From dari from_type child issues ─
	// Regression menang atas Smoke/Sanity, keduanya menang atas TEDB & from_type story.
	childFromType := map[string]string{} // parent key -> "Regression" | "Smoke/Sanity"
	for _, issue := range issues {
		parent := issue.Parent
		if issue.IssueType == "Story" {
			parent = issue.Key
		}
		if _, ok := storyBaseMap[parent]; !ok {
			continue
		}
		switch classifyFromType(issue.FromType) {
		case "Regression":
			childFromType[parent] = "Regression"
		case "Smoke/Sanity":
			if childFromType[parent] != "Regression" {
				childFromType[parent] = "Smoke/Sanity"
			}
		}
	}
	for key, base := range storyBaseMap {
		base.StoryFrom = resolveStoryFrom(key, base.StoryFrom, childFromType[key])
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

		if isFEAssignee(issue.Assignee) {
			a.HasFE = true
		}

		a.SP += sp
		if issue.IssueType == "Bug" {
			a.BugCount++
			switch classifyAccidentBug(issue.AccidentBug) {
			case "MissedPRD":
				a.MissedPRD++
			case "MissedTRD":
				a.MissedTRD++
			case "MissedCoding":
				a.MissedCoding++
			case "MissedTestCase":
				a.MissedTestCase++
			case "OtherIssue":
				a.OtherIssue++
			}
		}

		isFiltered := false
		for _, f := range keyFilters {
			if strings.Contains(issue.Key, f) {
				isFiltered = true
				break
			}
		}

		isQATask := false
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
						isQATask = true
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
						isQATask = true
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
				isQATask = true
			}
		}

		if isQATask {
			startTestDate := ""
			if issue.ActualTaskStartDate != "" {
				startTestDate = normalizeDate(issue.ActualTaskStartDate)
			}
			if startTestDate == "" && issue.ActualTaskDoneDate != "" {
				startTestDate = normalizeDate(issue.ActualTaskDoneDate)
			}
			if startTestDate != "" && (a.StartTesting == "" || startTestDate < a.StartTesting) {
				a.StartTesting = startTestDate
			}

			if issue.ActualTaskDoneDate != "" {
				d := normalizeDate(issue.ActualTaskDoneDate)
				if d != "" && d > a.EndTesting {
					a.EndTesting = d
				}
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

		if !isQATask {
			startDevDate := ""
			if issue.ActualTaskStartDate != "" {
				startDevDate = normalizeDate(issue.ActualTaskStartDate)
			}
			if startDevDate == "" && issue.ActualTaskDoneDate != "" {
				startDevDate = normalizeDate(issue.ActualTaskDoneDate)
			}
			if startDevDate != "" && (a.StartDev == "" || startDevDate < a.StartDev) {
				a.StartDev = startDevDate
			}

			if issue.ActualTaskDoneDate != "" {
				d := normalizeDate(issue.ActualTaskDoneDate)
				if d != "" && d > a.EndDev {
					a.EndDev = d
				}
			}
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
			// Skip stories without any child issues
			continue
		}

		//current formula: total hours = FTR hours + rework hours + bug count * 4 jam (asumsi tiap bug butuh 4 jam untuk fixing & retesting)
		oldTotalHours := a.Coding + a.Testing + a.Retest + a.Fixing
		oldTotalEngHours := a.Coding + a.Fixing
		oldTotalQAHours := a.Testing + a.Retest

		// new formula: total hours = semua jam kerja yang tercatat (bukan hanya FTR) + bug count * 4 jam (asumsi tiap bug butuh 4 jam untuk fixing & retesting)
		ftrHours := a.Coding + a.CodeReview + a.Testing
		reworkHours := a.HangingBug + a.Fixing + a.CodeRevBug + a.WaitingHours + a.Retest
		totalHours := ftrHours + reworkHours

		totalEngineerHours := a.Coding + a.CodeReview + a.Fixing + a.CodeRevBug + a.HangingBug
		totalQAHours := a.Testing + a.WaitingHours + a.Retest

		//old formula SLA:

		oldSlaEng := interface{}(nil)
		if a.SPEng > 0 {
			oldSlaEng = round2(oldTotalEngHours / a.SPEng)
		}
		oldSlaQA := interface{}(nil)
		if a.SPQA > 0 {
			oldSlaQA = round2(oldTotalQAHours / a.SPQA)
		}
		oldTotalSLA := interface{}(nil)
		if a.SP > 0 {
			oldTotalSLA = round2(oldTotalHours / a.SP)
		}

		//new formula SLA:
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
			key, base.EpicKey, base.PicEng, base.PicQA, base.DoneWeek, base.ReleaseWeek,
			round2(a.SP), round2(a.SPQA), round2(a.SPEng),
			round2(a.Coding), round2(a.CodeReview), round2(a.Testing),
			round2(a.HangingBug), round2(a.Fixing), round2(a.CodeRevBug), round2(a.WaitingHours), round2(a.Retest),
			a.BugCount, base.FixVersions, hasFE, base.Status,
			// Current Formula
			round2(oldTotalHours), oldSlaEng, oldSlaQA, oldTotalSLA,
			// New Formula (placeholder: same as current for now)
			round2(totalHours), slaEng, slaQA, totalSLA,
			// Development dates
			a.StartDev, a.EndDev,
			// Testing dates
			a.StartTesting, a.EndTesting,
			base.StoryFrom,
			// Missed / Accident Bug breakdown
			a.MissedPRD, a.MissedTRD, a.MissedCoding, a.MissedTestCase, a.OtherIssue,
		})
	}

	// ── STEP 5: Clear and recreate sheet — sheetID langsung didapat ─────
	sheetID, err := c.clearSheet(sheetName, len(rows)+2)
	if err != nil {
		return err
	}

	// ── STEP 6: Apply merges & formatting ────────────────────
	if err = c.applyGroupHeaders(sheetID); err != nil {
		return err
	}

	// ── STEP 7: Write row 1 (hanya group labels, sisanya kosong) ──
	row1 := make([]interface{}, len(SUMMARY_HEADERS))
	// Semua kosong by default, hanya isi group labels
	row1[9] = "First-Time Right"
	row1[12] = "Rework"
	row1[21] = "Current Formula"
	row1[25] = "New Formula"

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
	row2[9] = "Coding Hours"
	row2[10] = "Code Review Hours"
	row2[11] = "Testing Hours"
	row2[12] = "Hanging Bug Hours"
	row2[13] = "Fixing Hours"
	row2[14] = "Code Review Hours"
	row2[15] = "Waiting Hours"
	row2[16] = "Retesting Hours"
	row2[21] = "Total Hours"
	row2[22] = "SLA Eng"
	row2[23] = "SLA QA"
	row2[24] = "SLA All"
	row2[25] = "Total Hours"
	row2[26] = "SLA Eng"
	row2[27] = "SLA QA"
	row2[28] = "SLA All"

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
		// Unmerge semua cell dulu agar tidak konflik dengan merge lama (layout berubah)
		{
			UnmergeCells: &sheets.UnmergeCellsRequest{
				Range: &sheets.GridRange{
					SheetId:          sheetID,
					StartRowIndex:    0,
					EndRowIndex:      2,
					StartColumnIndex: 0,
					EndColumnIndex:   int64(len(SUMMARY_HEADERS)),
				},
			},
		},
		// Horizontal merge group cols (row 1 only)
		mergeH(9, 12),  // First-Time Right
		mergeH(12, 17), // Rework
		mergeH(21, 25), // Current Formula
		mergeH(25, 29), // New Formula
		// Center group labels row 1
		centerFmt(0, 1, 9, 17),
		centerFmt(0, 1, 21, 29),
		// Center group detail headers row 2
		centerFmt(1, 2, 9, 17),
		centerFmt(1, 2, 21, 29),
		// Bold group labels row 1
		boldFmt(0, 1, 9, 17),
		boldFmt(0, 1, 21, 29),
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
		4,  // Done Week
		17, // Bug Count
		34, // Total Missed PRD
		35, // Total Missed TRD
		36, // Total Missed Coding
		37, // Total Missed Test Case
		38, // Total Other Issue
	}
	floatCols := []int64{
		6,  // Total SP
		7,  // SP QA
		8,  // SP Eng
		9,  // Coding Hours (FTR)
		10, // Code Review Hours (FTR)
		11, // Testing Hours (FTR)
		12, // Hanging Bug Hours (Rework)
		13, // Fixing Hours (Rework)
		14, // Code Review Hours (Rework)
		15, // Waiting Hours (Rework)
		16, // Retesting Hours (Rework)
		21, // Current Formula - Total Hours
		22, // Current Formula - SLA Eng
		23, // Current Formula - SLA QA
		24, // Current Formula - SLA All
		25, // New Formula - Total Hours
		26, // New Formula - SLA Eng
		27, // New Formula - SLA QA
		28, // New Formula - SLA All
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

	dateCols := []int64{
		29, // Start Development
		30, // End Development
		31, // Start Testing
		32, // End Testing
	}

	makeDateRepeat := func(col int64) *sheets.Request {
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
							Type:    "DATE",
							Pattern: "yyyy-mm-dd",
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
	for _, col := range dateCols {
		requests = append(requests, makeDateRepeat(col))
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

// normalizeDate parses common date formats and returns "yyyy-mm-dd", or "" on failure.
func normalizeDate(s string) string {
	for _, layout := range []string{"2006-01-02 15:04:05", "2006-01-02T15:04:05Z07:00", "2006-01-02"} {
		if t, err := time.Parse(layout, s); err == nil {
			return t.Format("2006-01-02")
		}
	}
	return ""
}

var feAssignees = map[string]struct{}{
	"adi saputra":           {},
	"rizki gumilar":         {},
	"andika prasetya":       {},
	"naufal hadi":           {},
	"fuad rifqi zamzami":    {},
	"andikha apriadi":       {},
	"faridho":               {},
	"faizal bima":           {},
	"pratama ego":           {},
	"dorojatun chandrabumi": {},
}

// accidentBugCategory maps a raw Accident Bug value to its breakdown category.
var accidentBugCategory = map[string]string{
	// Total Missed PRD
	"changes requirement": "MissedPRD",
	"out of requirement":  "MissedPRD",
	"changes design":      "MissedPRD",
	// Total Missed TRD
	"bug case not covered": "MissedTRD",
	"additional trd":       "MissedTRD",
	// Total Missed Coding
	"bug in test case":     "MissedCoding",
	"missed staging build": "MissedCoding",
	"code collison":        "MissedCoding",
	// Total Missed Test Case
	"additional test case": "MissedTestCase",
	// Total Other Issue
	"manual verified":      "OtherIssue",
	"infrastructure":       "OtherIssue",
	"bug existing feature": "OtherIssue",
	"bug in test plan":     "OtherIssue",
}

// classifyAccidentBug maps a raw Accident Bug value to one of the breakdown
// categories (MissedPRD, MissedTRD, MissedCoding, MissedTestCase, OtherIssue),
// or "" when it matches none.
func classifyAccidentBug(accidentBug string) string {
	return accidentBugCategory[strings.ToLower(strings.TrimSpace(accidentBug))]
}

// classifyFromType maps a raw from_type value to "Regression" or "Smoke/Sanity",
// or "" when it is neither.
func classifyFromType(fromType string) string {
	u := strings.ToUpper(fromType)
	switch {
	case strings.Contains(u, "REGRESSION"):
		return "Regression"
	case strings.Contains(u, "SMOKE"), strings.Contains(u, "SANITY"):
		return "Smoke/Sanity"
	}
	return ""
}

// resolveStoryFrom determines the Story From value.
// Priority: Regression / Smoke/Sanity dari issue manapun di dalam story >
// "Tech Debt" untuk key TEDB > from_type story itu sendiri > "Product".
func resolveStoryFrom(key, ownFromType, childFromType string) string {
	if childFromType != "" {
		return childFromType
	}
	if own := classifyFromType(ownFromType); own != "" {
		return own
	}
	if strings.Contains(strings.ToUpper(key), "TEDB") {
		return "Tech Debt"
	}
	if ownFromType != "" {
		return ownFromType
	}
	return "Product"
}

func isFEAssignee(name string) bool {
	if name == "" {
		return false
	}
	_, ok := feAssignees[strings.ToLower(strings.TrimSpace(name))]
	return ok
}
