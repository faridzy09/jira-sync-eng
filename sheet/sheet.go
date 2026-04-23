package sheets

import (
	"context"
	"fmt"
	"jira-sync-eng/models"
	"os"

	"golang.org/x/oauth2/google"
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
	"Hanging Bug Hours", "Hanging Bug Day Work Hours",
	"Code Review Bug Hours", "Code Review Bug Day Work Hours",
	"Fixing Hours", "Retest Hours",
	"Count Fix Version", "Additional Task", "Accident Bug",
	"Bug From Category", "PIC Lead QA", "Actual Task Start Date",
	"Actual Task Done Date", "Actual Task Done Week",
	"Actual Task Done Month", "Actual Task Done Year",
	"Task Status", "Status Story",
}

// clearSheet membersihkan semua konten dan format sheet, serta expand baris jika kurang.
// rowCount: jumlah baris yang dibutuhkan.
func (c *Client) clearSheet(sheetName string, rowCount int) error {
	ss, err := c.service.Spreadsheets.Get(c.spreadsheetID).Do()
	if err != nil {
		return fmt.Errorf("get spreadsheet error: %w", err)
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
		_, err = c.service.Spreadsheets.BatchUpdate(c.spreadsheetID, &sheets.BatchUpdateSpreadsheetRequest{
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
			return fmt.Errorf("add sheet error: %w", err)
		}
		fmt.Printf("Sheet '%s' created\n", sheetName)
		return nil
	}

	var requests []*sheets.Request

	// Expand baris jika kurang
	neededRows := int64(rowCount) + 10
	if neededRows < 1000 {
		neededRows = 1000
	}
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

	// Clear semua konten + format sekaligus
	requests = append(requests, &sheets.Request{
		RepeatCell: &sheets.RepeatCellRequest{
			Range: &sheets.GridRange{
				SheetId: sheetID,
			},
			Cell:   &sheets.CellData{},
			Fields: "userEnteredValue,userEnteredFormat",
		},
	})

	_, err = c.service.Spreadsheets.BatchUpdate(c.spreadsheetID, &sheets.BatchUpdateSpreadsheetRequest{
		Requests: requests,
	}).Do()
	if err != nil {
		return fmt.Errorf("clear sheet error: %w", err)
	}

	fmt.Printf("Sheet '%s' cleared\n", sheetName)
	return nil
}

func (c *Client) SyncToSheet(sheetName string, issues []models.JiraIssue) error {
	// 1. Hapus dan buat ulang sheet
	if err := c.clearSheet(sheetName, len(issues)+1); err != nil {
		return err
	}

	// 2. Tulis header
	headerRange := sheetName + "!A1"
	var err error
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
		nullFloat(i.TestingHours), nullFloat(i.HangingBugHours), nullFloat(i.HangingBugDayWorkHours),
		nullFloat(i.CodeReviewBugHours), nullFloat(i.CodeReviewBugDayWorkHours),
		nullFloat(i.FixingHours),
		nullFloat(i.RetestHours), nullInt(i.CountFixVersion),
		nullStr(i.AdditionalTask), nullStr(i.AccidentBug),
		nullStr(i.BugFromCategory), nullStr(i.PicLeadQA),
		i.ActualTaskStartDate, i.ActualTaskDoneDate,
		i.ActualTaskDoneWeek, i.ActualTaskDoneMonth, i.ActualTaskDoneYear,
		i.TaskStatus, i.StatusStory,
	}
}
