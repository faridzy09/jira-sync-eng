package db

import (
	"fmt"
	"strings"
	"time"

	gcal "jira-sync-eng/google-calendar"
)

// CreateGCalTableIfNotExists membuat tabel google_calendar_events jika belum ada.
func (r *Repository) CreateGCalTableIfNotExists() error {
	query := `
    CREATE TABLE IF NOT EXISTS google_calendar_events (
        user_email       TEXT        NOT NULL,
        event_id         TEXT        NOT NULL,
        summary          TEXT,
        start_time       TIMESTAMPTZ,
        end_time         TIMESTAMPTZ,
        duration_minutes INTEGER,
        is_all_day       BOOLEAN     DEFAULT FALSE,
        bulan            TEXT,
        tahun            TEXT,
        synced_at        TIMESTAMPTZ DEFAULT NOW(),
        PRIMARY KEY (user_email, event_id)
    );`
	if _, err := r.db.Exec(query); err != nil {
		return err
	}

	// Tambah kolom jika tabel sudah ada sebelumnya (migrasi)
	migrations := []string{
		`ALTER TABLE google_calendar_events ADD COLUMN IF NOT EXISTS bulan TEXT`,
		`ALTER TABLE google_calendar_events ADD COLUMN IF NOT EXISTS tahun TEXT`,
	}
	for _, m := range migrations {
		if _, err := r.db.Exec(m); err != nil {
			return err
		}
	}
	return nil
}

// UpsertGCalEvents menyimpan atau memperbarui event Google Calendar ke database.
func (r *Repository) UpsertGCalEvents(events []gcal.CalendarEvent) error {
	if len(events) == 0 {
		return nil
	}

	cols := []string{
		"user_email", "event_id", "summary",
		"start_time", "end_time", "duration_minutes",
		"is_all_day", "bulan", "tahun", "synced_at",
	}

	placeholders := []string{}
	args := []interface{}{}
	idx := 1

	for _, ev := range events {
		row := []string{}
		for range cols {
			row = append(row, fmt.Sprintf("$%d", idx))
			idx++
		}
		placeholders = append(placeholders, "("+strings.Join(row, ",")+")")
		args = append(args,
			ev.UserEmail,
			ev.EventID,
			ev.Summary,
			ev.StartTime,
			ev.EndTime,
			ev.DurationMinutes,
			ev.IsAllDay,
			ev.StartTime.Format("January"),
			ev.StartTime.Format("2006"),
			time.Now(),
		)
	}

	query := fmt.Sprintf(`
        INSERT INTO google_calendar_events (%s)
        VALUES %s
        ON CONFLICT (user_email, event_id) DO UPDATE SET
            summary          = EXCLUDED.summary,
            start_time       = EXCLUDED.start_time,
            end_time         = EXCLUDED.end_time,
            duration_minutes = EXCLUDED.duration_minutes,
            is_all_day       = EXCLUDED.is_all_day,
            bulan            = EXCLUDED.bulan,
            tahun            = EXCLUDED.tahun,
            synced_at        = EXCLUDED.synced_at
    `, strings.Join(cols, ", "), strings.Join(placeholders, ", "))

	_, err := r.db.Exec(query, args...)
	if err != nil {
		return fmt.Errorf("upsert google_calendar_events error: %w", err)
	}
	return nil
}

// GetGCalEvents mengambil semua event grooming dari database.
func (r *Repository) GetGCalEvents() ([]gcal.CalendarEvent, error) {
	rows, err := r.db.Query(`
        SELECT user_email, event_id, summary, start_time, end_time, duration_minutes, is_all_day, bulan, tahun
        FROM google_calendar_events
        ORDER BY start_time ASC
    `)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var events []gcal.CalendarEvent
	for rows.Next() {
		var ev gcal.CalendarEvent
		if err := rows.Scan(
			&ev.UserEmail, &ev.EventID, &ev.Summary,
			&ev.StartTime, &ev.EndTime, &ev.DurationMinutes, &ev.IsAllDay,
			&ev.Bulan, &ev.Tahun,
		); err != nil {
			return nil, err
		}
		events = append(events, ev)
	}
	return events, rows.Err()
}
