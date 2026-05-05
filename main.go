package main

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"jira-sync-eng/config"
	"jira-sync-eng/db"
	gcal "jira-sync-eng/google-calendar"
	jiraclient "jira-sync-eng/jira"
	sheets "jira-sync-eng/sheet"
)

func main() {
	if len(os.Args) < 2 {
		log.Fatal("Usage: go run main.go [step-1|step-2|step-3|gcal-auth|google-calendar-sync]")
	}
	step := os.Args[1]

	cfg := config.Load()

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
		if err := sheetClient.SyncStorySummary("Story Summary", allIssues, cfg.GetDoneWeekBaseDate()); err != nil {
			log.Fatal("Story Summary sync error:", err)
		}

	case "gcal-auth":
		// ── Authorize satu user ke Google Calendar (jalankan sekali per user) ─
		if len(os.Args) < 3 {
			log.Fatal("Usage: go run main.go gcal-auth <email>")
		}
		email := os.Args[2]
		oauthCfg, err := gcal.LoadOAuthConfig(cfg.GCalOAuth2Path)
		if err != nil {
			log.Fatal(err)
		}
		if err := gcal.AuthorizeUser(oauthCfg, email); err != nil {
			log.Fatal("Auth error:", err)
		}
		fmt.Println("Done!")
		return

	case "google-calendar-sync":
		// ── Google Calendar: fetch grooming events & simpan ke DB ─
		fmt.Println("Membuat tabel google_calendar_events jika belum ada...")
		if err := repo.CreateGCalTableIfNotExists(); err != nil {
			log.Fatal("Create gcal table error:", err)
		}

		// Email akun yang login (pemilik kalender / organizer grooming)
		calendarOwner := cfg.GCalOwnerEmail

		// Kalender yang akan di-fetch — diambil dari env GCAL_FILTER_EMAILS (comma-separated)
		// Setiap kalender harus sudah di-share ke GCAL_OWNER_EMAIL
		gcalFilterEmails := cfg.GCalFilterEmails

		gcalClient, err := gcal.NewClient(cfg.GCalOAuth2Path, calendarOwner, gcalFilterEmails)
		if err != nil {
			log.Fatal("GCal client error:", err)
		}

		timeMin := time.Date(2026, 1, 1, 0, 0, 0, 0, time.Local)
		timeMax := time.Now().Add(24 * time.Hour)

		fmt.Printf("Fetching grooming events dari %s s/d %s...\n",
			timeMin.Format("02 Jan 2006"), timeMax.Format("02 Jan 2006"))

		events, err := gcalClient.FetchAllGroomingEvents(timeMin, timeMax)
		if err != nil {
			log.Fatal("GCal fetch error:", err)
		}
		fmt.Printf("Total %d event grooming ditemukan\n", len(events))

		if err := repo.UpsertGCalEvents(events); err != nil {
			log.Fatal("GCal DB upsert error:", err)
		}
		fmt.Printf("Berhasil disimpan %d event ke database\n", len(events))

		// ── Sync ke Google Sheets ────────────────────────────────
		fmt.Println("Syncing ke Google Sheets (Event Grooming)...")
		sheetClient, err := sheets.NewClient(cfg.CredentialsPath, cfg.SpreadsheetID)
		if err != nil {
			log.Fatal("Sheets client error:", err)
		}
		if err := sheetClient.SyncGCalEvents("Event Grooming", events); err != nil {
			log.Fatal("GCal sheet sync error:", err)
		}

		// Tampilkan hasil
		fmt.Println()
		fmt.Printf("%-40s %-8s %-6s %-22s %-22s %-11s %-35s\n",
			"USER", "BULAN", "TAHUN", "MULAI", "SELESAI", "DURASI(mnt)", "NAMA EVENT")
		fmt.Println(strings.Repeat("-", 155))
		for _, ev := range events {
			startStr := ev.StartTime.Format("02 Jan 2006 15:04")
			endStr := ev.EndTime.Format("02 Jan 2006 15:04")
			if ev.IsAllDay {
				startStr = ev.StartTime.Format("02 Jan 2006") + " (all-day)"
				endStr = ev.EndTime.Format("02 Jan 2006")
			}
			bulan := ev.StartTime.Format("January")
			tahun := ev.StartTime.Format("2006")
			fmt.Printf("%-40s %-8s %-6s %-22s %-22s %-11d %-35s\n",
				ev.UserEmail, bulan, tahun, startStr, endStr,
				ev.DurationMinutes, ev.Summary)
		}

	default:
		log.Fatalf("Unknown step: %s. Use step-1, step-2, step-3, atau google-calendar-sync", step)
	}

	fmt.Println("Done!")
}
