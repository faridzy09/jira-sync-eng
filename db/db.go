package db

import (
	"database/sql"
	"fmt"
	"jira-sync-eng/models"
	"strings"
	"time"

	_ "github.com/lib/pq" // PostgreSQL driver
)

type Repository struct {
	db *sql.DB
}

func NewRepository(host, port, user, dbname string) (*Repository, error) {
	dsn := fmt.Sprintf(
		"host=%s port=%s user=%s dbname=%s sslmode=disable",
		host,
		port,
		user,
		dbname,
	)
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		return nil, err
	}

	// Konfigurasi connection pool agar tidak terlalu banyak koneksi idle
	db.SetMaxOpenConns(10)
	db.SetMaxIdleConns(5)
	db.SetConnMaxLifetime(30 * time.Minute)

	if err := db.Ping(); err != nil {
		return nil, err
	}
	return &Repository{db: db}, nil
}

func (r *Repository) CreateTableIfNotExists() error {
	query := `
    CREATE TABLE IF NOT EXISTS jira_issues (
        key                      TEXT PRIMARY KEY,
        issue_type               TEXT,
        summary                  TEXT,
        assignee                 TEXT,
        pic_lead_engineer        TEXT,
        status_category_changed  TIMESTAMP,
        done_week                INTEGER,
        fix_versions             TEXT,
        fix_version_released     BOOLEAN,
        fix_version_release_date TEXT,
        release_week             TEXT,
        story_point              FLOAT,
        from_type                TEXT,
        parent                   TEXT,
        coding_hours             FLOAT,
        code_review_hours        FLOAT,
		code_review_day_work_hours FLOAT,
        testing_hours            FLOAT,
        hanging_bug_by_eng_hours        FLOAT,
		hanging_bug_by_eng_day_work_hours FLOAT,
        hanging_bug_by_qa_hours         FLOAT,
        hanging_bug_by_qa_day_work_hours FLOAT,
        code_review_bug_hours    FLOAT,
		code_review_bug_day_work_hours    FLOAT,
        fixing_hours             FLOAT,
        retest_hours             FLOAT,
        count_fix_version        INTEGER,
        additional_task          TEXT,
        accident_bug             TEXT,
        bug_from_category        TEXT,
        pic_lead_qa              TEXT,
        actual_task_start_date   TEXT,
        actual_task_done_date    TEXT,
        actual_task_done_week    TEXT,
        actual_task_done_month   TEXT,
        actual_task_done_year    TEXT,
        task_status              TEXT,
        status_story             TEXT,
        first_ready_to_test_bug_date TEXT,
        first_in_qa_bug_date         TEXT,
        synced_at                TIMESTAMP DEFAULT NOW()
    );`
	_, err := r.db.Exec(query)
	if err != nil {
		return err
	}

	return nil
}

// UpsertBatch: insert or update dalam batch 500
func (r *Repository) UpsertBatch(issues []models.JiraIssue) error {
	const batchSize = 500

	for i := 0; i < len(issues); i += batchSize {
		end := i + batchSize
		if end > len(issues) {
			end = len(issues)
		}
		chunk := issues[i:end]

		if err := r.upsertChunk(chunk); err != nil {
			return fmt.Errorf("batch %d error: %w", i/batchSize, err)
		}
		fmt.Printf("DB upsert: %d/%d rows\n", end, len(issues))
	}
	return nil
}

func (r *Repository) upsertChunk(issues []models.JiraIssue) error {
	if len(issues) == 0 {
		return nil
	}

	cols := []string{
		"key", "issue_type", "summary", "assignee", "pic_lead_engineer",
		"status_category_changed", "done_week", "fix_versions",
		"fix_version_released", "fix_version_release_date", "release_week",
		"story_point", "from_type", "parent", "coding_hours",
		"code_review_hours", "code_review_day_work_hours",
		"testing_hours", "hanging_bug_by_eng_hours", "hanging_bug_by_eng_day_work_hours",
		"hanging_bug_by_qa_hours", "hanging_bug_by_qa_day_work_hours",
		"code_review_bug_hours", "code_review_bug_day_work_hours",
		"fixing_hours", "retest_hours",
		"count_fix_version", "additional_task", "accident_bug",
		"bug_from_category", "pic_lead_qa", "actual_task_start_date",
		"actual_task_done_date", "actual_task_done_week",
		"actual_task_done_month", "actual_task_done_year",
		"task_status", "status_story", "first_ready_to_test_bug_date", "first_in_qa_bug_date", "synced_at",
	}

	placeholders := []string{}
	args := []interface{}{}
	argIdx := 1

	for _, issue := range issues {
		rowPlaceholders := []string{}
		for range cols {
			rowPlaceholders = append(rowPlaceholders, fmt.Sprintf("$%d", argIdx))
			argIdx++
		}
		placeholders = append(placeholders, "("+strings.Join(rowPlaceholders, ",")+")")

		args = append(args,
			issue.Key, issue.IssueType, issue.Summary, issue.Assignee,
			issue.PicLeadEngineer, issue.StatusCategoryChanged, issue.DoneWeek,
			issue.FixVersions, issue.FixVersionReleased, issue.FixVersionReleaseDate,
			issue.ReleaseWeek, issue.StoryPoint, issue.FromType, issue.Parent,
			issue.CodingHours, issue.CodeReviewHours, issue.CodeReviewDayWorkHours,
			issue.TestingHours, issue.HangingBugByEngHours, issue.HangingBugByEngDayWorkHours,
			issue.HangingBugByQAHours, issue.HangingBugByQADayWorkHours,
			issue.CodeReviewBugHours, issue.CodeReviewBugDayWorkHours,
			issue.FixingHours, issue.RetestHours, issue.CountFixVersion, issue.AdditionalTask,
			issue.AccidentBug, issue.BugFromCategory, issue.PicLeadQA,
			issue.ActualTaskStartDate, issue.ActualTaskDoneDate,
			issue.ActualTaskDoneWeek, issue.ActualTaskDoneMonth,
			issue.ActualTaskDoneYear, issue.TaskStatus, issue.StatusStory,
			issue.FirstReadyToTestBugDate, issue.FirstInQABugDate,
			time.Now(),
		)
	}

	// UPDATE kolom yang conflict di key
	updateCols := []string{}
	for _, col := range cols {
		if col != "key" {
			updateCols = append(updateCols, fmt.Sprintf("%s = EXCLUDED.%s", col, col))
		}
	}

	query := fmt.Sprintf(
		`INSERT INTO jira_issues (%s) VALUES %s
         ON CONFLICT (key) DO UPDATE SET %s`,
		strings.Join(cols, ","),
		strings.Join(placeholders, ","),
		strings.Join(updateCols, ","),
	)

	_, err := r.db.Exec(query, args...)
	return err
}

// GetAllForSync: ambil semua data untuk di-sync ke Sheet
func (r *Repository) GetAllForSync() ([]models.JiraIssue, error) {
	rows, err := r.db.Query(`
		SELECT
			key, issue_type, summary, assignee, pic_lead_engineer,
			status_category_changed, done_week, fix_versions,
			fix_version_released, fix_version_release_date, release_week,
			story_point, from_type, parent, coding_hours,
			code_review_hours, code_review_day_work_hours,
			testing_hours, hanging_bug_by_eng_hours, hanging_bug_by_eng_day_work_hours,
			hanging_bug_by_qa_hours, hanging_bug_by_qa_day_work_hours,
			code_review_bug_hours, code_review_bug_day_work_hours,
			fixing_hours, retest_hours,
			count_fix_version, additional_task, accident_bug,
			bug_from_category, pic_lead_qa, actual_task_start_date,
			actual_task_done_date, actual_task_done_week,
			actual_task_done_month, actual_task_done_year,
			task_status, status_story, first_ready_to_test_bug_date, first_in_qa_bug_date, synced_at
		FROM jira_issues ORDER BY actual_task_done_week ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var issues []models.JiraIssue
	for rows.Next() {
		var issue models.JiraIssue
		err := rows.Scan(
			&issue.Key, &issue.IssueType, &issue.Summary, &issue.Assignee,
			&issue.PicLeadEngineer, &issue.StatusCategoryChanged, &issue.DoneWeek,
			&issue.FixVersions, &issue.FixVersionReleased, &issue.FixVersionReleaseDate,
			&issue.ReleaseWeek, &issue.StoryPoint, &issue.FromType, &issue.Parent,
			&issue.CodingHours, &issue.CodeReviewHours, &issue.CodeReviewDayWorkHours,
			&issue.TestingHours, &issue.HangingBugByEngHours, &issue.HangingBugByEngDayWorkHours,
			&issue.HangingBugByQAHours, &issue.HangingBugByQADayWorkHours,
			&issue.CodeReviewBugHours, &issue.CodeReviewBugDayWorkHours,
			&issue.FixingHours, &issue.RetestHours, &issue.CountFixVersion, &issue.AdditionalTask,
			&issue.AccidentBug, &issue.BugFromCategory, &issue.PicLeadQA,
			&issue.ActualTaskStartDate, &issue.ActualTaskDoneDate,
			&issue.ActualTaskDoneWeek, &issue.ActualTaskDoneMonth,
			&issue.ActualTaskDoneYear, &issue.TaskStatus, &issue.StatusStory,
			&issue.FirstReadyToTestBugDate, &issue.FirstInQABugDate,
			&issue.SyncedAt,
		)
		if err != nil {
			return nil, err
		}
		issues = append(issues, issue)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rows iteration error: %w", err)
	}
	return issues, nil
}
