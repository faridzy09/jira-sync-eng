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
	"Code Review Hours", "Testing Hours", "Hanging Bug Hours",
	"Code Review Bug Hours", "Fixing Hours", "Retest Hours",
	"Count Fix Version", "Additional Task", "Accident Bug",
	"Bug From Category", "PIC Lead QA", "Actual Task Start Date",
	"Actual Task Done Date", "Actual Task Done Week",
	"Actual Task Done Month", "Actual Task Done Year",
	"Task Status", "Status Story",
}

func (c *Client) SyncToSheet(sheetName string, issues []models.JiraIssue) error {
	// 1. Clear sheet
	clearRange := sheetName + "!A:AZ"
	_, err := c.service.Spreadsheets.Values.Clear(
		c.spreadsheetID, clearRange, &sheets.ClearValuesRequest{},
	).Do()
	if err != nil {
		return fmt.Errorf("clear error: %w", err)
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
		nullFloat(i.CodingHours), nullFloat(i.CodeReviewHours),
		nullFloat(i.TestingHours), nullFloat(i.HangingBugHours),
		nullFloat(i.CodeReviewBugHours), nullFloat(i.FixingHours),
		nullFloat(i.RetestHours), nullInt(i.CountFixVersion),
		nullStr(i.AdditionalTask), nullStr(i.AccidentBug),
		nullStr(i.BugFromCategory), nullStr(i.PicLeadQA),
		i.ActualTaskStartDate, i.ActualTaskDoneDate,
		i.ActualTaskDoneWeek, i.ActualTaskDoneMonth, i.ActualTaskDoneYear,
		i.TaskStatus, i.StatusStory,
	}
}
