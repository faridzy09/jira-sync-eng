package sheets

import (
	"fmt"
	"jira-sync-eng/models"
	"math"
	"sort"
	"strings"

	"google.golang.org/api/sheets/v4"
)

var SUMMARY_HEADERS = []interface{}{
	"Key",                            // 0
	"PIC Lead Engineer",              // 1
	"PIC Lead QA",                    // 2
	"Done Week",                      // 3
	"Release Week",                   // 4
	"Total SP",                       // 5
	"SP QA",                          // 6
	"SP Eng",                         // 7
	"Coding Hours",                   // 8
	"Code Review Hours",              // 9
	"Code Review Day Work Hours",     // 10
	"Testing Hours",                  // 11
	"Fixing Hours",                   // 12
	"Code Review Bug Hours",          // 13
	"Code Review Bug Day Work Hours", // 14
	"Retest Hours",                   // 15
	"Total Hours",                    // 16
	"Total Day Work Hours",           // 17
	"SLA (Hours/SP)",                 // 18
	"SLA Day Work (Hours/SP)",        // 19
	"Bug Count",                      // 20
	"Fix Versions",                   // 21
	"Has FE",                         // 22
	"Status Story",                   // 23
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
	SP           float64
	SPQA         float64
	SPEng        float64
	Coding       float64
	CodeReview   float64
	CodeReviewDW float64 // day work hours
	Testing      float64
	Fixing       float64
	CodeRevBug   float64
	CodeRevBugDW float64 // day work hours
	Retest       float64
	BugCount     int
	HasFE        bool
}

func (c *Client) SyncStorySummary(sheetName string, issues []models.JiraIssue) error {
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
	for _, issue := range issues {
		if issue.IssueType == "Story" {
			continue
		}
		if _, ok := storyBaseMap[issue.Parent]; !ok {
			continue
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
		if issue.CodingHours != nil {
			a.Coding += *issue.CodingHours
		}
		if issue.TestingHours != nil {
			a.Testing += *issue.TestingHours
		}
		if issue.FixingHours != nil {
			a.Fixing += *issue.FixingHours
		}
		if issue.RetestHours != nil {
			a.Retest += *issue.RetestHours
		}
		if issue.CodeReviewBugHours != nil {
			a.CodeRevBug += *issue.CodeReviewBugHours
		}
		if issue.CodeReviewBugDayWorkHours != nil {
			a.CodeRevBugDW += *issue.CodeReviewBugDayWorkHours
		}

		if issue.CodeReviewHours != nil {
			a.CodeReview += *issue.CodeReviewHours
		}
		if issue.CodeReviewDayWorkHours != nil {
			a.CodeReviewDW += *issue.CodeReviewDayWorkHours
		}

		if issue.IssueType == "Bug" {
			a.BugCount++
		}

		// ── SP Distribution Logic ─────────────────────────────
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
	}

	// ── STEP 3: Sort by doneWeek ASC ─────────────────────────
	sort.SliceStable(storyOrder, func(i, j int) bool {
		return parseDoneWeek(storyBaseMap[storyOrder[i]].DoneWeek) <
			parseDoneWeek(storyBaseMap[storyOrder[j]].DoneWeek)
	})

	// ── STEP 4: Build output rows ─────────────────────────────
	rows := make([][]interface{}, 0, len(storyOrder))
	for _, key := range storyOrder {
		base := storyBaseMap[key]
		a := aggMap[key]
		if a == nil {
			a = &storyAgg{}
		}
		totalHours := a.Coding + a.CodeReview + a.Testing + a.Fixing + a.CodeRevBug + a.Retest
		totalDWHours := a.Coding + a.CodeReviewDW + a.Testing + a.Fixing + a.CodeRevBugDW + a.Retest
		sla := interface{}(nil)
		slaDW := interface{}(nil)
		if a.SP > 0 {
			sla = round2(totalHours / a.SP)
			slaDW = round2(totalDWHours / a.SP)
		}
		hasFE := ""
		if a.HasFE {
			hasFE = "Yes"
		}
		rows = append(rows, []interface{}{
			key, base.PicEng, base.PicQA, base.DoneWeek, base.ReleaseWeek,
			round2(a.SP), round2(a.SPQA), round2(a.SPEng),
			round2(a.Coding), round2(a.CodeReview), round2(a.CodeReviewDW),
			round2(a.Testing), round2(a.Fixing),
			round2(a.CodeRevBug), round2(a.CodeRevBugDW), round2(a.Retest),
			round2(totalHours), round2(totalDWHours), sla, slaDW,
			a.BugCount, base.FixVersions, hasFE, base.Status,
		})
	}

	// ── STEP 5: Hapus dan buat ulang sheet ───────────────────
	if err := c.clearSheet(sheetName, len(rows)+1); err != nil {
		return err
	}

	// ── STEP 6: Write header ──────────────────────────────────
	var err error
	_, err = c.service.Spreadsheets.Values.Update(
		c.spreadsheetID, sheetName+"!A1",
		&sheets.ValueRange{Values: [][]interface{}{SUMMARY_HEADERS}},
	).ValueInputOption("RAW").Do()
	if err != nil {
		return fmt.Errorf("header error: %w", err)
	}

	if len(rows) == 0 {
		return nil
	}

	// ── STEP 7: Write data in chunks ──────────────────────────
	const chunkSize = 1000
	for i := 0; i < len(rows); i += chunkSize {
		end := i + chunkSize
		if end > len(rows) {
			end = len(rows)
		}
		chunk := make([][]interface{}, end-i)
		copy(chunk, rows[i:end])

		dataRange := fmt.Sprintf("%s!A%d", sheetName, i+2)
		_, err = c.service.Spreadsheets.Values.Update(
			c.spreadsheetID, dataRange,
			&sheets.ValueRange{Values: chunk},
		).ValueInputOption("RAW").Do()
		if err != nil {
			return fmt.Errorf("write chunk %d error: %w", i/chunkSize, err)
		}
		fmt.Printf("Story Summary written: %d/%d rows\n", end, len(rows))
	}

	fmt.Printf("SyncStorySummary done: %d stories\n", len(rows))
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
