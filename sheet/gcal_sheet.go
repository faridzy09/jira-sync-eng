package sheets

import (
	"fmt"

	gcal "jira-sync-eng/google-calendar"

	"google.golang.org/api/sheets/v4"
)

var GCAL_HEADERS = []interface{}{
	"User Email", "Bulan", "Tahun", "Mulai", "Selesai", "Durasi (mnt)", "Nama Event",
}

// SyncGCalEvents clear sheet lalu menulis ulang event grooming Google Calendar.
func (c *Client) SyncGCalEvents(sheetName string, events []gcal.CalendarEvent) error {
	// Ambil jumlah baris existing agar clearSheet bisa hapus semua data lama
	existingRows, _ := c.getSheetRowCount(sheetName)
	neededRows := len(events) + 1 // +1 untuk header
	clearRows := existingRows
	if neededRows > clearRows {
		clearRows = neededRows
	}

	sheetID, err := c.clearSheet(sheetName, clearRows)
	if err != nil {
		return fmt.Errorf("clearSheet error: %w", err)
	}

	// ── Tulis data ────────────────────────────────────────────
	values := [][]interface{}{GCAL_HEADERS}
	for _, ev := range events {
		startStr := ev.StartTime.Format("02 Jan 2006 15:04")
		endStr := ev.EndTime.Format("02 Jan 2006 15:04")
		if ev.IsAllDay {
			startStr = ev.StartTime.Format("02 Jan 2006")
			endStr = ev.EndTime.Format("02 Jan 2006")
		}
		values = append(values, []interface{}{
			ev.UserEmail,
			ev.Bulan,
			ev.Tahun,
			startStr,
			endStr,
			ev.DurationMinutes,
			ev.Summary,
		})
	}

	writeRange := fmt.Sprintf("%s!A1", sheetName)
	_, err = c.service.Spreadsheets.Values.Update(
		c.spreadsheetID,
		writeRange,
		&sheets.ValueRange{Values: values},
	).ValueInputOption("USER_ENTERED").Do()
	if err != nil {
		return fmt.Errorf("write values error: %w", err)
	}
	fmt.Printf("Sheet '%s': %d baris ditulis\n", sheetName, len(values)-1)

	// ── Bold header ───────────────────────────────────────────
	if err := c.retryBatchUpdate(&sheets.BatchUpdateSpreadsheetRequest{
		Requests: []*sheets.Request{{
			RepeatCell: &sheets.RepeatCellRequest{
				Range: &sheets.GridRange{
					SheetId:          sheetID,
					StartRowIndex:    0,
					EndRowIndex:      1,
					StartColumnIndex: 0,
					EndColumnIndex:   int64(len(GCAL_HEADERS)),
				},
				Cell: &sheets.CellData{
					UserEnteredFormat: &sheets.CellFormat{
						TextFormat: &sheets.TextFormat{Bold: true},
					},
				},
				Fields: "userEnteredFormat.textFormat.bold",
			},
		}},
	}); err != nil {
		return fmt.Errorf("bold header error: %w", err)
	}

	return nil
}
