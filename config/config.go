package config

import (
	"log"
	"os"
	"strconv"
	"time"

	"github.com/joho/godotenv"
)

type Config struct {
	// Jira
	JiraBaseURL  string
	JiraEmail    string
	JiraAPIToken string
	JiraJQL      string
	JiraJQLBug   string

	// Database

	DatabaseURL string
	DBHost      string
	DBPort      string
	DBUser      string
	DBName      string

	// Google Sheets
	SpreadsheetID   string
	CredentialsJSON string
	CredentialsPath string

	// Google Calendar
	GCalOwnerEmail string
	GCalOAuth2Path string

	// Date base (untuk kalkulasi week)
	FirstYear  string
	FirstMonth string
	FirstDay   string

	// Sheet names
	SheetJira         string
	SheetJiraSLABug   string
	SheetStorySummary string
	SheetHariLibur    string
	SheetTeamSquad    string
}

func Load() *Config {
	// Load .env jika ada (opsional, tidak error jika tidak ada)
	if err := godotenv.Load(); err != nil {
		log.Println("No .env file found, using environment variables")
	}

	cfg := &Config{
		// Jira
		JiraBaseURL:  getEnv("JIRA_BASE_URL", ""),
		JiraEmail:    getEnv("JIRA_EMAIL", ""),
		JiraAPIToken: getEnv("JIRA_API_TOKEN", ""),
		JiraJQL:      getEnv("JIRA_JQL", ""),
		JiraJQLBug:   getEnv("JIRA_JQL_BUG", ""),

		// Database

		DBHost: getEnv("DB_HOST", "localhost"),
		DBPort: getEnv("DB_PORT", "5432"),
		DBUser: getEnv("DB_USER", "postgres"),
		DBName: getEnv("DB_NAME", "jira-sync-eng"),

		// Google Sheets
		SpreadsheetID:   getEnv("SPREADSHEET_ID", ""),
		CredentialsJSON: getEnv("GOOGLE_CREDENTIALS_JSON", ""),
		CredentialsPath: getEnv("GOOGLE_CREDENTIALS_PATH", "credentials.json"),

		// Google Calendar
		GCalOwnerEmail: getEnv("GCAL_OWNER_EMAIL", ""),
		GCalOAuth2Path: getEnv("GCAL_OAUTH2_PATH", "oauth2_client.json"),

		// Date base
		FirstYear:  getEnv("FIRST_YEAR", "2024"),
		FirstMonth: getEnv("FIRST_MONTH", "1"),
		FirstDay:   getEnv("FIRST_DAY", "1"),

		// Sheet names
		SheetJira:         getEnv("SHEET_JIRA", "Jira"),
		SheetJiraSLABug:   getEnv("SHEET_JIRA_SLA_BUG", "Jira SLA Bug"),
		SheetStorySummary: getEnv("SHEET_STORY_SUMMARY", "Story Summary"),
		SheetHariLibur:    getEnv("SHEET_HARI_LIBUR", "Hari Libur"),
		SheetTeamSquad:    getEnv("SHEET_TEAM_SQUAD", "Team Squad"),
	}

	cfg.validate()
	return cfg
}

func (c *Config) validate() {
	required := map[string]string{
		"JIRA_BASE_URL":  c.JiraBaseURL,
		"JIRA_EMAIL":     c.JiraEmail,
		"JIRA_API_TOKEN": c.JiraAPIToken,
		"JIRA_JQL":       c.JiraJQL,
		"SPREADSHEET_ID": c.SpreadsheetID,
	}

	for key, val := range required {
		if val == "" {
			log.Fatalf("Missing required environment variable: %s", key)
		}
	}
}

func getEnv(key, defaultVal string) string {
	if val := os.Getenv(key); val != "" {
		return val
	}
	return defaultVal
}

func (c *Config) GetDoneWeekBaseDate() time.Time {
	year, _ := strconv.Atoi(c.FirstYear)
	month, _ := strconv.Atoi(c.FirstMonth)
	day, _ := strconv.Atoi(c.FirstDay)
	return time.Date(year, time.Month(month), day, 0, 0, 0, 0, time.Local)
}
