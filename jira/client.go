package jira

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"jira-sync-eng/models"
	"log"
	"math"
	"net/http"
	"strings"
	"time"
)

var wib = time.FixedZone("WIB", 7*3600)

type Config struct {
	BaseURL  string
	Email    string
	APIToken string
	JQL      string
}

type Client struct {
	config     Config
	httpClient *http.Client
	authHeader string
}

func NewClient(cfg Config) *Client {
	auth := base64.StdEncoding.EncodeToString(
		[]byte(fmt.Sprintf("%s:%s", cfg.Email, cfg.APIToken)),
	)
	return &Client{
		config:     cfg,
		httpClient: &http.Client{Timeout: 30 * time.Second},
		authHeader: "Basic " + auth,
	}
}

type jiraResponse struct {
	Issues        []jiraIssueRaw `json:"issues"`
	IsLast        bool           `json:"isLast"`
	NextPageToken string         `json:"nextPageToken"`
}

type jiraIssueRaw struct {
	Key       string          `json:"key"`
	Fields    json.RawMessage `json:"fields"`
	Changelog struct {
		Histories []struct {
			Created string `json:"created"`
			Items   []struct {
				Field      string `json:"field"`
				FromString string `json:"fromString"`
				ToString   string `json:"toString"`
			} `json:"items"`
		} `json:"histories"`
	} `json:"changelog"`
}

func (c *Client) FetchAllIssues() ([]jiraIssueRaw, error) {
	var allIssues []jiraIssueRaw
	var nextPageToken string
	page := 0

	for {
		payload := map[string]interface{}{
			"jql":        c.config.JQL,
			"maxResults": 100,
			"fields": []string{
				"summary", "assignee", "status", "issuetype",
				"customfield_10024", "created", "updated",
				"customfield_10195", "customfield_10196",
				"parent", "fixVersions", "customfield_10156",
				"customfield_10492", "customfield_10165",
				"customfield_11331", "statuscategorychangedate",
				"customfield_11364", "customfield_11397",
				"customfield_11398",
			},
			"expand": "changelog",
		}

		if nextPageToken != "" {
			payload["nextPageToken"] = nextPageToken
		}

		body, _ := json.Marshal(payload)
		req, err := http.NewRequest("POST",
			c.config.BaseURL+"/rest/api/3/search/jql",
			bytes.NewBuffer(body),
		)
		if err != nil {
			return nil, err
		}

		req.Header.Set("Authorization", c.authHeader)
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Accept", "application/json")

		resp, err := c.httpClient.Do(req)
		if err != nil {
			return nil, err
		}
		defer resp.Body.Close()

		respBody, _ := io.ReadAll(resp.Body)
		if resp.StatusCode != 200 {
			return nil, fmt.Errorf("jira error %d: %s", resp.StatusCode, string(respBody))
		}

		var result jiraResponse
		if err := json.Unmarshal(respBody, &result); err != nil {
			return nil, err
		}

		allIssues = append(allIssues, result.Issues...)
		page++
		fmt.Printf("Page %d: %d issues (total: %d)\n", page, len(result.Issues), len(allIssues))

		if result.IsLast || result.NextPageToken == "" {
			break
		}

		nextPageToken = result.NextPageToken
		time.Sleep(300 * time.Millisecond)
	}

	return allIssues, nil
}

// ── Raw Jira field structures ────────────────────────────────

type IssueFields struct {
	Summary  string `json:"summary"`
	Assignee *struct {
		DisplayName string `json:"displayName"`
	} `json:"assignee"`
	Status *struct {
		Name string `json:"name"`
	} `json:"status"`
	IssueType *struct {
		Name string `json:"name"`
	} `json:"issuetype"`
	Parent *struct {
		Key    string `json:"key"`
		Fields *struct {
			Status *struct {
				Name string `json:"name"`
			} `json:"status"`
		} `json:"fields"`
	} `json:"parent"`
	FixVersions []struct {
		Name        string `json:"name"`
		Released    bool   `json:"released"`
		ReleaseDate string `json:"releaseDate"`
	} `json:"fixVersions"`
	StoryPoint              *float64 `json:"customfield_10024"`
	StatusCategoryChangedAt string   `json:"statuscategorychangedate"`
	CreatedAt               string   `json:"created"` // issue created date

	// Custom fields
	CustomField10156 *struct {
		Value string `json:"value"`
	} `json:"customfield_10156"` // Accident Bug
	CustomField10492 *struct {
		Value string `json:"value"`
	} `json:"customfield_10492"` // Additional Task
	CustomField11331 interface{} `json:"customfield_11331"` // Bug parent key (can be string or object)
	CustomField11364 *struct {
		Value string `json:"value"`
	} `json:"customfield_11364"` // From Type
	CustomField11397 *struct {
		DisplayName string `json:"displayName"`
	} `json:"customfield_11397"` // PIC Lead Engineer
	CustomField11398 *struct {
		DisplayName string `json:"displayName"`
	} `json:"customfield_11398"` // PIC Lead QA
}

type RawIssue struct {
	Key       string      `json:"key"`
	Fields    IssueFields `json:"fields"`
	Changelog struct {
		Histories []History `json:"histories"`
	} `json:"changelog"`
}

type History struct {
	Created string `json:"created"`
	Items   []struct {
		Field      string `json:"field"`
		FromString string `json:"fromString"`
		ToString   string `json:"toString"`
	} `json:"items"`
}

// ── Parse semua issue (mirip buildRow_ di Apps Script) ───────

func (c *Client) ParseIssues(raw []json.RawMessage, baseDate time.Time) []models.JiraIssue {
	// Decode semua dulu
	issues := make([]RawIssue, 0, len(raw))
	for _, r := range raw {
		var issue RawIssue
		if err := json.Unmarshal(r, &issue); err != nil {
			continue
		}
		issues = append(issues, issue)
	}

	// Build storyMap: key → *RawIssue
	storyMap := buildStoryMap(issues)

	result := make([]models.JiraIssue, 0, len(issues))
	for i := range issues {
		parsed := parseIssue(&issues[i], storyMap, baseDate)
		result = append(result, parsed)
	}
	return result
}

func buildStoryMap(issues []RawIssue) map[string]*RawIssue {
	m := make(map[string]*RawIssue)
	for i := range issues {
		if issues[i].Fields.IssueType != nil &&
			issues[i].Fields.IssueType.Name == "Story" {
			m[issues[i].Key] = &issues[i]
		}
	}
	return m
}

func parseIssue(issue *RawIssue, storyMap map[string]*RawIssue, baseDate time.Time) models.JiraIssue {
	fields := issue.Fields
	issueType := ""
	if fields.IssueType != nil {
		issueType = fields.IssueType.Name
	}

	isStory := issueType == "Story"
	isBug := issueType == "Bug"

	validSources := map[string]bool{
		"From Product": true, "From SA": true, "From Tech Debt": true,
	}
	bugTypes := map[string]bool{
		"Bug in Test Case": true, "Code Collison": true,
		"Bug in Test Plan": true, "Adjustment Test Case": true,
		"Missed Staging Build": true,
	}

	// ── Parent key resolution ────────────────────────────────
	parentKey := resolveParentKey(issue, isStory, isBug)

	// ── Fix Versions ─────────────────────────────────────────
	fvs := getFixVersions(issue, storyMap[parentKey], isStory)
	latestFV := getLatestFixVersion(fvs)

	// ── Done Week ────────────────────────────────────────────
	var doneDateRef string
	if isStory {
		if fields.Status != nil && fields.Status.Name == "Done" {
			doneDateRef = fields.StatusCategoryChangedAt
		}
	} else {
		if story, ok := storyMap[parentKey]; ok {
			if story.Fields.Status != nil && story.Fields.Status.Name == "Done" {
				doneDateRef = story.Fields.StatusCategoryChangedAt
			}
		}
	}

	// ── Release Week ─────────────────────────────────────────
	releaseWeek := "-"
	if latestFV != nil && latestFV.Released && latestFV.ReleaseDate != "" {
		w := calculateWeek(latestFV.ReleaseDate, baseDate)
		if w > 0 {
			releaseWeek = fmt.Sprintf("%d", w)
		}
	}

	// ── Actual task dates ────────────────────────────────────
	doneDateAt := doneTask(issue)
	actualStartDate := firstInProgress(issue)
	taskDoneWeekNum := calculateWeek(doneDateAt, baseDate)
	taskDoneMonth := getMonthName(doneDateAt)
	taskDoneYear := getYear(doneDateAt)

	// ── Eligible Bug check ───────────────────────────────────
	accidentBugVal := ""
	if fields.CustomField10156 != nil {
		accidentBugVal = fields.CustomField10156.Value
	}
	additionalTaskVal := ""
	if fields.CustomField10492 != nil {
		additionalTaskVal = fields.CustomField10492.Value
	}
	isEligibleBug := (isBug && bugTypes[accidentBugVal]) ||
		validSources[additionalTaskVal]

	// ── Hours map (dari changelog) ───────────────────────────
	hoursMap := map[string]float64{}
	dayWorkHoursMap := map[string]float64{}
	if !isStory {
		hoursMap = calcHoursByStatus(issue)
		dayWorkHoursMap = calcDayWorkHoursByStatus(issue)
	}
	getHours := func(status string) *float64 {
		key := strings.ToUpper(strings.TrimSpace(status))
		if v, ok := hoursMap[key]; ok {
			rounded := round2(v)
			return &rounded
		}
		return nil
	}
	getDayWorkHours := func(status string) *float64 {
		key := strings.ToUpper(strings.TrimSpace(status))
		if v, ok := dayWorkHoursMap[key]; ok {
			rounded := round2(v)
			return &rounded
		}
		return nil
	}

	// ── PIC Lead Engineer & QA ───────────────────────────────
	picEng := ""
	picQA := ""
	if isStory {
		if fields.CustomField11397 != nil {
			picEng = fields.CustomField11397.DisplayName
		}
		if fields.CustomField11398 != nil {
			picQA = fields.CustomField11398.DisplayName
		}
	} else if story, ok := storyMap[parentKey]; ok {
		if story.Fields.CustomField11397 != nil {
			picEng = story.Fields.CustomField11397.DisplayName
		}
		if story.Fields.CustomField11398 != nil {
			picQA = story.Fields.CustomField11398.DisplayName
		}
	}

	// ── Fix version fields ───────────────────────────────────
	fvNames := make([]string, len(fvs))
	for i, fv := range fvs {
		fvNames[i] = fv.Name
	}
	fixVersionsStr := strings.Join(fvNames, "; ")

	var fixVersionReleased *bool
	fixVersionReleaseDate := ""
	if latestFV != nil {
		b := latestFV.Released
		fixVersionReleased = &b
		fixVersionReleaseDate = latestFV.ReleaseDate
	}

	// ── Status Category Changed ──────────────────────────────
	var statusCategoryChanged *time.Time
	if fields.StatusCategoryChangedAt != "" {
		if t, err := parseJiraTime(fields.StatusCategoryChangedAt); err == nil {
			statusCategoryChanged = &t
		}
	}

	// ── Done Week ────────────────────────────────────────────
	doneWeekNum := calculateWeek(doneDateRef, baseDate)
	var doneWeekPtr *int
	if doneWeekNum > 0 {
		doneWeekPtr = &doneWeekNum
	}

	// ── Assignee ─────────────────────────────────────────────
	assignee := "Unassigned"
	if fields.Assignee != nil {
		assignee = fields.Assignee.DisplayName
	}

	// ── Task Status & Status Story ───────────────────────────
	taskStatus := ""
	statusStory := ""
	if fields.Status != nil {
		taskStatus = fields.Status.Name
	}
	if isStory {
		statusStory = taskStatus
	} else if fields.Parent != nil && fields.Parent.Fields != nil &&
		fields.Parent.Fields.Status != nil {
		statusStory = fields.Parent.Fields.Status.Name
	}

	// ── Count Fix Version ────────────────────────────────────
	var countFV *int
	if isStory {
		n := len(fvs)
		countFV = &n
	}

	// ── From Type ────────────────────────────────────────────
	fromType := ""
	if fields.CustomField11364 != nil {
		fromType = fields.CustomField11364.Value
	}

	// ── Bug From Category ────────────────────────────────────
	bugFromCategory := ""
	if isBug {
		bugFromCategory = getBugCategory(accidentBugVal)
	}

	// ── Actual Task Done Week/Month/Year ─────────────────────
	var taskDoneWeekStr, taskDoneMonthStr, taskDoneYearStr string
	if taskDoneWeekNum > 0 {
		taskDoneWeekStr = fmt.Sprintf("%d", taskDoneWeekNum)
	}
	taskDoneMonthStr = taskDoneMonth
	if taskDoneYear > 0 {
		taskDoneYearStr = fmt.Sprintf("%d", taskDoneYear)
	}

	return models.JiraIssue{
		Key:                         issue.Key,
		IssueType:                   issueType,
		Summary:                     fields.Summary,
		Assignee:                    assignee,
		PicLeadEngineer:             picEng,
		StatusCategoryChanged:       statusCategoryChanged,
		DoneWeek:                    doneWeekPtr,
		FixVersions:                 fixVersionsStr,
		FixVersionReleased:          fixVersionReleased,
		FixVersionReleaseDate:       fixVersionReleaseDate,
		ReleaseWeek:                 releaseWeek,
		StoryPoint:                  fields.StoryPoint,
		FromType:                    fromType,
		Parent:                      parentKey,
		CodingHours:                 nilIfNotTask(getHours("IN PROGRESS"), !isStory && !isEligibleBug),
		CodeReviewHours:             nilIfNotTask(getHours("CODE REVIEW"), !isStory && !isEligibleBug),
		CodeReviewDayWorkHours:      nilIfNotTask(getDayWorkHours("CODE REVIEW"), !isStory && !isEligibleBug),
		TestingHours:                nilIfNotTask(getHours("IN QA"), !isStory && !isBug),
		HangingBugByEngHours:        nilIfNotTask(calcHangingBugHours(issue), isBug),
		HangingBugByEngDayWorkHours: nilIfNotTask(calcHangingBugDayWorkHours(issue), isBug),
		HangingBugByQAHours:         nilIfNotTask(calcHangingBugByQAHours(issue), isBug),
		HangingBugByQADayWorkHours:  nilIfNotTask(calcHangingBugByQADayWorkHours(issue), isBug),
		CodeReviewBugHours:          nilIfNotTask(getHours("CODE REVIEW"), isEligibleBug),
		CodeReviewBugDayWorkHours:   nilIfNotTask(getDayWorkHours("CODE REVIEW"), isEligibleBug),
		FixingHours:                 nilIfNotTask(getHours("IN PROGRESS"), isEligibleBug),
		RetestHours:                 nilIfNotTask(getHours("RETESTING"), !isStory && isBug),
		CountFixVersion:             countFV,
		AdditionalTask:              additionalTaskVal,
		AccidentBug:                 accidentBugVal,
		BugFromCategory:             bugFromCategory,
		PicLeadQA:                   picQA,
		ActualTaskStartDate:         actualStartDate,
		ActualTaskDoneDate:          doneDateAt,
		ActualTaskDoneWeek:          taskDoneWeekStr,
		ActualTaskDoneMonth:         taskDoneMonthStr,
		ActualTaskDoneYear:          taskDoneYearStr,
		TaskStatus:                  taskStatus,
		StatusStory:                 statusStory,
		FirstReadyToTestBugDate: func() string {
			if isBug {
				return firstReadyToTest(issue)
			}
			return ""
		}(),
		FirstInQABugDate: func() string {
			if isBug {
				return firstInTesting(issue)
			}
			return ""
		}(),
	}
}

// ── Helper functions ─────────────────────────────────────────

func resolveParentKey(issue *RawIssue, isStory, isBug bool) string {
	if isStory {
		return issue.Key
	}
	if isBug {
		if s, ok := issue.Fields.CustomField11331.(string); ok && s != "" {
			return s
		}
		if issue.Fields.Parent != nil {
			return issue.Fields.Parent.Key
		}
		return "-"
	}
	if issue.Fields.Parent != nil {
		return issue.Fields.Parent.Key
	}
	return "-"
}

type fixVersion struct {
	Name        string
	Released    bool
	ReleaseDate string
}

func getFixVersions(issue *RawIssue, story *RawIssue, isStory bool) []fixVersion {
	src := issue
	if !isStory && story != nil {
		src = story
	}
	result := make([]fixVersion, len(src.Fields.FixVersions))
	for i, fv := range src.Fields.FixVersions {
		result[i] = fixVersion{
			Name:        fv.Name,
			Released:    fv.Released,
			ReleaseDate: fv.ReleaseDate,
		}
	}
	return result
}

func getLatestFixVersion(fvs []fixVersion) *fixVersion {
	if len(fvs) == 0 {
		return nil
	}
	var latest *fixVersion
	var latestTime int64 = math.MinInt64
	for i := range fvs {
		t := int64(math.MinInt64)
		if fvs[i].ReleaseDate != "" {
			if parsed, err := time.Parse("2006-01-02", fvs[i].ReleaseDate); err == nil {
				t = parsed.UnixMilli()
			}
		}
		if t >= latestTime {
			latestTime = t
			latest = &fvs[i]
		}
	}
	return latest
}

// mondayOf returns the Monday (00:00:00 local) of the week containing t.
func mondayOf(t time.Time) time.Time {
	wd := int(t.Weekday())
	if wd == 0 {
		wd = 7 // Sunday → 7 so Monday=1 … Sunday=7
	}
	return time.Date(t.Year(), t.Month(), t.Day()-wd+1, 0, 0, 0, 0, t.Location())
}

// calculateWeek returns the week number (1-based, Mon–Sun) of dateStr
// relative to the week that contains baseDate (that week = week 1).
func calculateWeek(dateStr string, baseDate time.Time) int {
	if dateStr == "" || dateStr == "N/A" {
		return 0
	}
	formats := []string{
		"2006-01-02 15:04:05",
		"2006-01-02T15:04:05.000-0700",
		"2006-01-02T15:04:05-0700",
		time.RFC3339Nano,
		time.RFC3339,
		"2006-01-02",
	}
	var d time.Time
	var err error
	for _, f := range formats {
		d, err = time.Parse(f, dateStr)
		if err == nil {
			break
		}
	}
	if err != nil {
		return 0
	}
	baseMon := mondayOf(baseDate)
	dMon := mondayOf(d)
	diff := dMon.Sub(baseMon)
	week := int(diff.Hours()/(7*24)) + 1
	if week < 1 {
		return 0
	}
	return week
}

func parseJiraTime(s string) (time.Time, error) {
	formats := []string{
		"2006-01-02T15:04:05.000-0700", // Jira: with ms, no colon in tz
		"2006-01-02T15:04:05-0700",     // Jira: no ms, no colon in tz
		time.RFC3339Nano,               // with colon in tz
		time.RFC3339,                   // with colon in tz, no ms
	}
	for _, f := range formats {
		if t, err := time.Parse(f, s); err == nil {
			return t, nil
		}
	}
	return time.Time{}, fmt.Errorf("cannot parse %q", s)
}

func doneTask(issue *RawIssue) string {
	histories := issue.Changelog.Histories
	// Cari yang terakhir dengan status = "Done" (reverse)
	for i := len(histories) - 1; i >= 0; i-- {
		h := histories[i]
		for _, item := range h.Items {
			if item.Field == "status" && item.ToString == "Done" {
				if t, err := parseJiraTime(h.Created); err == nil {
					return t.Format("2006-01-02 15:04:05")
				}
			}
		}
	}
	return "N/A"
}

func firstInProgress(issue *RawIssue) string {
	for _, h := range issue.Changelog.Histories {
		for _, item := range h.Items {
			if item.Field == "status" && item.ToString == "In Progress" {
				if t, err := parseJiraTime(h.Created); err == nil {
					return t.Format("2006-01-02 15:04:05")
				}
			}
		}
	}
	return "N/A"
}

// firstReadyToTest returns the timestamp of the first transition TO "Ready to Test".
func firstReadyToTest(issue *RawIssue) string {
	for _, h := range issue.Changelog.Histories {
		for _, item := range h.Items {
			if item.Field == "status" && item.ToString == "Ready to Test" {
				if t, err := parseJiraTime(h.Created); err == nil {
					return t.Format("2006-01-02 15:04:05")
				}
			}
		}
	}
	return "N/A"
}

// firstInTesting returns the timestamp of the first transition TO  "In Testing".
func firstInTesting(issue *RawIssue) string {
	for _, h := range issue.Changelog.Histories {
		for _, item := range h.Items {
			if item.Field == "status" &&
				(item.ToString == "In Testing" || strings.ToLower(item.ToString) == "retesting") {
				if t, err := parseJiraTime(h.Created); err == nil {
					return t.Format("2006-01-02 15:04:05")
				}
			}
		}
	}
	return "N/A"
}

func calcHoursByStatus(issue *RawIssue) map[string]float64 {
	out := map[string]float64{}
	type event struct {
		t    int64
		from string
		to   string
	}
	var events []event

	for _, h := range issue.Changelog.Histories {
		t, err := parseJiraTime(h.Created)
		if err != nil {
			continue
		}
		for _, item := range h.Items {
			if item.Field == "status" {
				events = append(events, event{
					t:    t.UnixMilli(),
					from: item.FromString,
					to:   item.ToString,
				})
			}
		}
	}

	if len(events) == 0 {
		return out
	}

	// Sort by time
	for i := 1; i < len(events); i++ {
		for j := i; j > 0 && events[j].t < events[j-1].t; j-- {
			events[j], events[j-1] = events[j-1], events[j]
		}
	}

	currentStatus := events[0].from
	currentTime := events[0].t

	for _, e := range events {
		if currentStatus != "" {
			key := strings.ToUpper(strings.TrimSpace(currentStatus))
			hrs := float64(e.t-currentTime) / 3600000.0
			if hrs >= 0 {
				out[key] += hrs
			}
		}
		currentStatus = e.to
		currentTime = e.t
	}

	// Tail sampai sekarang
	if currentStatus != "" {
		key := strings.ToUpper(strings.TrimSpace(currentStatus))
		hrs := float64(time.Now().UnixMilli()-currentTime) / 3600000.0
		if hrs >= 0 {
			out[key] += hrs
		}
	}

	return out
}

func getMonthName(dateStr string) string {
	months := []string{
		"January", "February", "March", "April", "May", "June",
		"July", "August", "September", "October", "November", "December",
	}
	formats := []string{
		"2006-01-02 15:04:05",
		"2006-01-02T15:04:05.000-0700",
		"2006-01-02T15:04:05-0700",
		time.RFC3339Nano,
		time.RFC3339,
		"2006-01-02",
	}
	for _, f := range formats {
		if t, err := time.Parse(f, dateStr); err == nil {
			return months[t.Month()-1]
		}
	}
	return ""
}

func getYear(dateStr string) int {
	formats := []string{
		"2006-01-02 15:04:05",
		"2006-01-02T15:04:05.000-0700",
		"2006-01-02T15:04:05-0700",
		time.RFC3339Nano,
		time.RFC3339,
		"2006-01-02",
	}
	for _, f := range formats {
		if t, err := time.Parse(f, dateStr); err == nil {
			return t.Year()
		}
	}
	return 0
}

func getBugCategory(reason string) string {
	m := map[string]string{
		"Bug in Test Case":     "Bug From Engineer",
		"Missed Staging Build": "Bug From Engineer",
		"Code Collison":        "Bug From Engineer",
		"Adjustment Test Case": "Bug From Engineer",
		"Bug in Test Plan":     "Bug From Engineer",
		"Bug Case Not Covered": "Bug From SA",
		"Infrastructure":       "Bug From SA",
		"Tech Debt SA":         "Bug From SA",
		"Out of Requirement":   "Bug From Product",
		"Changes Requirement":  "Bug From Product",
		"Changes Design":       "Bug From Product",
		"Bug Existing Feature": "Bug From Tech Debt",
	}
	if v, ok := m[reason]; ok {
		return v
	}
	return "Unknown"
}

// calcDayWorkHoursByStatus menghitung jam kerja efektif (Senin-Jumat, 09:00-18:00 WIB, exclude weekend)
// per status dari changelog.
func calcDayWorkHoursByStatus(issue *RawIssue) map[string]float64 {
	out := map[string]float64{}
	type event struct {
		t    time.Time
		from string
		to   string
	}
	var events []event

	for _, h := range issue.Changelog.Histories {
		t, err := parseJiraTime(h.Created)
		if err != nil {
			continue
		}
		for _, item := range h.Items {
			if item.Field == "status" {
				events = append(events, event{t: t, from: item.FromString, to: item.ToString})
			}
		}
	}

	if len(events) == 0 {
		return out
	}

	// Sort by time asc
	for i := 1; i < len(events); i++ {
		for j := i; j > 0 && events[j].t.Before(events[j-1].t); j-- {
			events[j], events[j-1] = events[j-1], events[j]
		}
	}

	currentStatus := events[0].from
	currentTime := events[0].t
	for _, e := range events {
		if currentStatus != "" {
			key := strings.ToUpper(strings.TrimSpace(currentStatus))
			out[key] += calcWorkHoursInInterval(currentTime, e.t)
		}
		currentStatus = e.to
		currentTime = e.t
	}
	// Tail sampai sekarang
	if currentStatus != "" {
		key := strings.ToUpper(strings.TrimSpace(currentStatus))
		out[key] += calcWorkHoursInInterval(currentTime, time.Now())
	}
	return out
}

// calcWorkHoursInInterval menghitung total jam kerja efektif antara from dan to.
// Jam kerja: Senin-Jumat, 09:00–18:00 WIB, exclude weekend dan hari libur nasional.
func calcWorkHoursInInterval(from, to time.Time) float64 {
	if !to.After(from) {
		return 0
	}
	from = from.In(wib)
	to = to.In(wib)

	workStart := 9 // 09:00
	workEnd := 18  // 18:00

	total := 0.0
	cursor := from
	for cursor.Before(to) {
		wd := cursor.Weekday()
		// Skip weekend
		if wd == time.Saturday || wd == time.Sunday {
			cursor = nextWorkMorning(cursor, workStart)
			continue
		}
		// Skip hari libur nasional
		if models.IsHoliday(cursor) {
			cursor = nextWorkMorning(cursor, workStart)
			continue
		}

		dayStart := time.Date(cursor.Year(), cursor.Month(), cursor.Day(), workStart, 0, 0, 0, wib)
		dayEnd := time.Date(cursor.Year(), cursor.Month(), cursor.Day(), workEnd, 0, 0, 0, wib)

		segStart := cursor
		if segStart.Before(dayStart) {
			segStart = dayStart
		}
		segEnd := to
		if segEnd.After(dayEnd) {
			segEnd = dayEnd
		}
		if segEnd.After(segStart) {
			total += segEnd.Sub(segStart).Hours()
		}

		next := time.Date(cursor.Year(), cursor.Month(), cursor.Day()+1, workStart, 0, 0, 0, wib)
		if !next.Before(to) {
			break
		}
		cursor = next
	}
	return round2(total)
}

// calcHangingBugByQAHours menghitung calendar hours dari first "Ready to Test" sampai first "In QA"/"In Testing".
func calcHangingBugByQAHours(issue *RawIssue) *float64 {
	readyStr := firstReadyToTest(issue)
	if readyStr == "N/A" {
		return nil
	}
	inQAStr := firstInTesting(issue)
	if inQAStr == "N/A" {
		return nil
	}
	readyTime, err := time.ParseInLocation("2006-01-02 15:04:05", readyStr, wib)
	if err != nil {
		return nil
	}
	inQATime, err := time.ParseInLocation("2006-01-02 15:04:05", inQAStr, wib)
	if err != nil {
		return nil
	}
	if !inQATime.After(readyTime) {
		return nil
	}
	hrs := round2(inQATime.Sub(readyTime).Hours())
	return &hrs
}

// calcHangingBugByQADayWorkHours menghitung jam kerja efektif dari first "Ready to Test" sampai first "In QA"/"In Testing"
// (Senin-Jumat, 09:00-18:00 WIB, exclude hari libur).
func calcHangingBugByQADayWorkHours(issue *RawIssue) *float64 {
	readyStr := firstReadyToTest(issue)
	if readyStr == "N/A" {
		return nil
	}
	inQAStr := firstInTesting(issue)
	if inQAStr == "N/A" {
		return nil
	}
	readyTime, err := time.ParseInLocation("2006-01-02 15:04:05", readyStr, wib)
	if err != nil {
		return nil
	}
	inQATime, err := time.ParseInLocation("2006-01-02 15:04:05", inQAStr, wib)
	if err != nil {
		return nil
	}
	if !inQATime.After(readyTime) {
		return nil
	}
	hrs := calcWorkHoursInInterval(readyTime, inQATime)
	return &hrs
}

// calcHangingBugHours menghitung total jam dari created tiket sampai first in progress (calendar hours).
func calcHangingBugHours(issue *RawIssue) *float64 {
	if issue.Fields.CreatedAt == "" {
		return nil
	}
	createdTime, err := parseJiraTime(issue.Fields.CreatedAt)
	if err != nil {
		log.Printf("Error parsing created time: %v", err)
		return nil
	}
	inProgressStr := firstInProgress(issue)
	if inProgressStr == "N/A" {
		return nil
	}
	inProgressTime, err := time.ParseInLocation("2006-01-02 15:04:05", inProgressStr, wib)
	if err != nil {
		log.Printf("Error parsing in progress time: %v", err)
		return nil
	}

	hrs := round2(inProgressTime.Sub(createdTime).Hours())
	return &hrs
}

// calcHangingBugDayWorkHours menghitung jam kerja efektif dari created tiket sampai first in progress
// (Senin-Jumat, 09:00-18:00 WIB, exclude hari libur).
func calcHangingBugDayWorkHours(issue *RawIssue) *float64 {
	if issue.Fields.CreatedAt == "" {
		return nil
	}
	createdTime, err := parseJiraTime(issue.Fields.CreatedAt)
	if err != nil {
		return nil
	}
	inProgressStr := firstInProgress(issue)
	if inProgressStr == "N/A" {
		return nil
	}
	inProgressTime, err := time.ParseInLocation("2006-01-02 15:04:05", inProgressStr, wib)
	if err != nil {
		return nil
	}
	hrs := calcWorkHoursInInterval(createdTime, inProgressTime)
	return &hrs
}

func nextWorkMorning(t time.Time, hour int) time.Time {
	next := time.Date(t.Year(), t.Month(), t.Day()+1, hour, 0, 0, 0, t.Location())
	for next.Weekday() == time.Saturday || next.Weekday() == time.Sunday || models.IsHoliday(next) {
		next = next.AddDate(0, 0, 1)
	}
	return next
}

func round2(v float64) float64 {
	return math.Round(v*100) / 100
}

func nilIfNotTask(v *float64, condition bool) *float64 {
	if !condition {
		return nil
	}
	return v
}
