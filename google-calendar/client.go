package gcal

import (
	"fmt"
	"strings"
	"time"

	"golang.org/x/oauth2"
	"google.golang.org/api/calendar/v3"
)

// CalendarEvent menyimpan satu event Google Calendar.
type CalendarEvent struct {
	UserEmail       string
	EventID         string
	Summary         string
	StartTime       time.Time
	EndTime         time.Time
	DurationMinutes int
	IsAllDay        bool
	Bulan           string
	Tahun           string
}

// Client fetch event dari Google Calendar menggunakan OAuth2 token satu akun.
// Event difilter berdasarkan keyword "grooming" DAN attendees mengandung
// salah satu dari daftar FilterEmails.
type Client struct {
	oauthCfg     *oauth2.Config
	CalendarEmail string   // email akun yang login (pemilik kalender)
	FilterEmails  []string // daftar email yang harus ada sebagai attendee
}

// NewClient membuat Client dengan OAuth2 config dan daftar email yang difilter.
func NewClient(oauthCfgPath string, calendarEmail string, filterEmails []string) (*Client, error) {
	cfg, err := LoadOAuthConfig(oauthCfgPath)
	if err != nil {
		return nil, err
	}
	return &Client{
		oauthCfg:     cfg,
		CalendarEmail: calendarEmail,
		FilterEmails:  filterEmails,
	}, nil
}

// FetchAllGroomingEvents mengambil event grooming langsung dari kalender
// masing-masing user (kalender harus sudah di-share ke CalendarEmail).
func (c *Client) FetchAllGroomingEvents(timeMin, timeMax time.Time) ([]CalendarEvent, error) {
	svc, err := GetCalendarService(c.oauthCfg, c.CalendarEmail)
	if err != nil {
		return nil, err
	}

	var results []CalendarEvent

	for _, userEmail := range c.FilterEmails {
		events, err := fetchFromCalendar(svc, userEmail, timeMin, timeMax)
		if err != nil {
			fmt.Printf("[WARN] Gagal fetch kalender %s: %v\n", userEmail, err)
			continue
		}
		for _, ev := range events {
			ce := parseEvent(userEmail, ev)
			results = append(results, ce)
		}
		fmt.Printf("[OK] %s: %d event grooming\n", userEmail, len(events))
	}

	return results, nil
}

// fetchFromCalendar fetch event grooming dari kalender spesifik (by email/calendarID).
func fetchFromCalendar(svc *calendar.Service, calendarID string, timeMin, timeMax time.Time) ([]*calendar.Event, error) {
	call := svc.Events.List(calendarID).
		Q("grooming").
		TimeMin(timeMin.Format(time.RFC3339)).
		TimeMax(timeMax.Format(time.RFC3339)).
		SingleEvents(true).
		OrderBy("startTime").
		MaxResults(2500)

	var items []*calendar.Event
	for {
		resp, err := call.Do()
		if err != nil {
			return nil, fmt.Errorf("gagal fetch events: %w", err)
		}
		for _, ev := range resp.Items {
			if strings.Contains(strings.ToLower(ev.Summary), "grooming") {
				items = append(items, ev)
			}
		}
		if resp.NextPageToken == "" {
			break
		}
		call = call.PageToken(resp.NextPageToken)
	}
	return items, nil
}

func parseEvent(calendarEmail string, ev *calendar.Event) CalendarEvent {
	loc := time.Local
	ce := CalendarEvent{
		UserEmail: calendarEmail,
		EventID:   ev.Id,
		Summary:   ev.Summary,
	}

	if ev.Start.DateTime != "" {
		start, _ := time.Parse(time.RFC3339, ev.Start.DateTime)
		end, _ := time.Parse(time.RFC3339, ev.End.DateTime)
		ce.StartTime = start.In(loc)
		ce.EndTime = end.In(loc)
		ce.DurationMinutes = int(ce.EndTime.Sub(ce.StartTime).Minutes())
		ce.IsAllDay = false
	} else if ev.Start.Date != "" {
		start, _ := time.ParseInLocation("2006-01-02", ev.Start.Date, loc)
		end, _ := time.ParseInLocation("2006-01-02", ev.End.Date, loc)
		ce.StartTime = start
		ce.EndTime = end
		ce.DurationMinutes = int(ce.EndTime.Sub(ce.StartTime).Minutes())
		ce.IsAllDay = true
	}

	ce.Bulan = ce.StartTime.Format("January")
	ce.Tahun = ce.StartTime.Format("2006")

	return ce
}


