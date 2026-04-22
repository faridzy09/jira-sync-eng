package main

import (
	"encoding/json"
	"fmt"
	"log"
	"os"

	"jira-sync-eng/config"
	"jira-sync-eng/db"
	jiraclient "jira-sync-eng/jira"
	sheets "jira-sync-eng/sheet"
)

func main() {
	if len(os.Args) < 2 {
		log.Fatal("Usage: go run main.go [step-1|step-2|step-3]")
	}
	step := os.Args[1]

	cfg := config.Load()

	fmt.Println(cfg.DBHost, cfg.DBPort, cfg.DBUser, cfg.DBName)

	// ── STEP 1: Init DB ──────────────────────────────────────
	fmt.Println("Connecting to database...")
	repo, err := db.NewRepository(
		cfg.DBHost,
		cfg.DBPort,
		cfg.DBUser,
		cfg.DBName,
	)
	if err != nil {
		log.Fatal("DB error:", err)
	}
	if err := repo.CreateTableIfNotExists(); err != nil {
		log.Fatal("Create table error:", err)
	}

	switch step {
	case "step-1":
		// ── STEP 2: Fetch dari Jira ──────────────────────────────
		fmt.Println("Fetching from Jira...")
		jiraClient := jiraclient.NewClient(jiraclient.Config{
			BaseURL:  cfg.JiraBaseURL,
			Email:    cfg.JiraEmail,
			APIToken: cfg.JiraAPIToken,
			JQL:      cfg.JiraJQL,
		})
		rawIssues, err := jiraClient.FetchAllIssues()
		if err != nil {
			log.Fatal("Jira fetch error:", err)
		}
		fmt.Printf("Fetched %d issues\n", len(rawIssues))

		// ── STEP 3: Parse & Save ke DB ──────────────────────────
		rawMessages := make([]json.RawMessage, len(rawIssues))
		for i, issue := range rawIssues {
			b, err := json.Marshal(issue)
			if err != nil {
				log.Fatal("Marshal error:", err)
			}
			rawMessages[i] = b
		}
		issues := jiraClient.ParseIssues(rawMessages, cfg.GetDoneWeekBaseDate())
		if err := repo.UpsertBatch(issues); err != nil {
			log.Fatal("DB upsert error:", err)
		}

	case "step-2":
		// ── STEP 4: Ambil dari DB ───────────────────────────────
		fmt.Println("Reading from database...")
		allIssues, err := repo.GetAllForSync()
		if err != nil {
			log.Fatal("DB read error:", err)
		}
		fmt.Printf("Total %d issues to sync\n", len(allIssues))

		// ── STEP 5: Sync ke Google Sheet ────────────────────────
		fmt.Println("Syncing to Google Sheets...")
		sheetClient, err := sheets.NewClient(cfg.CredentialsPath, cfg.SpreadsheetID)
		if err != nil {
			log.Fatal("Sheets client error:", err)
		}
		if err := sheetClient.SyncToSheet("Jira", allIssues); err != nil {
			log.Fatal("Sheets sync error:", err)
		}

	case "step-3":
		// ── Story Summary ────────────────────────────────────────
		fmt.Println("Reading from database...")
		allIssues, err := repo.GetAllForSync()
		if err != nil {
			log.Fatal("DB read error:", err)
		}
		fmt.Printf("Total %d issues\n", len(allIssues))

		fmt.Println("Syncing Story Summary to Google Sheets...")
		sheetClient, err := sheets.NewClient(cfg.CredentialsPath, cfg.SpreadsheetID)
		if err != nil {
			log.Fatal("Sheets client error:", err)
		}
		if err := sheetClient.SyncStorySummary("Story Summary", allIssues); err != nil {
			log.Fatal("Story Summary sync error:", err)
		}

	default:
		log.Fatalf("Unknown step: %s. Use step-1, step-2, or step-3", step)
	}

	fmt.Println("Done!")
}
