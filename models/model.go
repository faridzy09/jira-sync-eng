package models

import "time"

type JiraIssue struct {
	Key                         string     `json:"key" db:"key"`
	IssueType                   string     `json:"issue_type" db:"issue_type"`
	Summary                     string     `json:"summary" db:"summary"`
	Assignee                    string     `json:"assignee" db:"assignee"`
	PicLeadEngineer             string     `json:"pic_lead_engineer" db:"pic_lead_engineer"`
	StatusCategoryChanged       *time.Time `json:"status_category_changed" db:"status_category_changed"`
	DoneWeek                    *int       `json:"done_week" db:"done_week"`
	FixVersions                 string     `json:"fix_versions" db:"fix_versions"`
	FixVersionReleased          *bool      `json:"fix_version_released" db:"fix_version_released"`
	FixVersionReleaseDate       string     `json:"fix_version_release_date" db:"fix_version_release_date"`
	ReleaseWeek                 string     `json:"release_week" db:"release_week"`
	StoryPoint                  *float64   `json:"story_point" db:"story_point"`
	FromType                    string     `json:"from_type" db:"from_type"`
	Parent                      string     `json:"parent" db:"parent"`
	CodingHours                 *float64   `json:"coding_hours" db:"coding_hours"`
	CodeReviewHours             *float64   `json:"code_review_hours" db:"code_review_hours"`
	CodeReviewDayWorkHours      *float64   `json:"code_review_day_work_hours" db:"code_review_day_work_hours"`
	TestingHours                *float64   `json:"testing_hours" db:"testing_hours"`
	HangingBugByEngHours        *float64   `json:"hanging_bug_by_eng_hours" db:"hanging_bug_by_eng_hours"`
	HangingBugByEngDayWorkHours *float64   `json:"hanging_bug_by_eng_day_work_hours" db:"hanging_bug_by_eng_day_work_hours"`
	HangingBugByQAHours         *float64   `json:"hanging_bug_by_qa_hours" db:"hanging_bug_by_qa_hours"`
	HangingBugByQADayWorkHours  *float64   `json:"hanging_bug_by_qa_day_work_hours" db:"hanging_bug_by_qa_day_work_hours"`
	CodeReviewBugHours          *float64   `json:"code_review_bug_hours" db:"code_review_bug_hours"`
	CodeReviewBugDayWorkHours   *float64   `json:"code_review_bug_day_work_hours" db:"code_review_bug_day_work_hours"`
	FixingHours                 *float64   `json:"fixing_hours" db:"fixing_hours"`
	RetestHours                 *float64   `json:"retest_hours" db:"retest_hours"`
	CountFixVersion             *int       `json:"count_fix_version" db:"count_fix_version"`
	AdditionalTask              string     `json:"additional_task" db:"additional_task"`
	AccidentBug                 string     `json:"accident_bug" db:"accident_bug"`
	BugFromCategory             string     `json:"bug_from_category" db:"bug_from_category"`
	PicLeadQA                   string     `json:"pic_lead_qa" db:"pic_lead_qa"`
	ActualTaskStartDate         string     `json:"actual_task_start_date" db:"actual_task_start_date"`
	ActualTaskDoneDate          string     `json:"actual_task_done_date" db:"actual_task_done_date"`
	ActualTaskDoneWeek          string     `json:"actual_task_done_week" db:"actual_task_done_week"`
	ActualTaskDoneMonth         string     `json:"actual_task_done_month" db:"actual_task_done_month"`
	ActualTaskDoneYear          string     `json:"actual_task_done_year" db:"actual_task_done_year"`
	TaskStatus                  string     `json:"task_status" db:"task_status"`
	StatusStory                 string     `json:"status_story" db:"status_story"`
	FirstReadyToTestBugDate     string     `json:"first_ready_to_test_bug_date" db:"first_ready_to_test_bug_date"`
	FirstInQABugDate            string     `json:"first_in_qa_bug_date" db:"first_in_qa_bug_date"`
	SyncedAt                    time.Time  `json:"synced_at" db:"synced_at"`
}
