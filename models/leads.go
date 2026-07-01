package models

import (
	"strings"
	"time"
)

var LeadEngineers = map[string]struct{}{
	"derikurniawan":           {},
	"falih mulyana":           {},
	"irvan resna hadiyana":    {},
	"sholahuddin alisyahbana": {},
	"susi cahyati":            {},
	"faridho":                 {},
}

func IsLeadEngineer(name string) bool {
	if name == "" {
		return false
	}
	_, ok := LeadEngineers[strings.ToLower(strings.TrimSpace(name))]
	return ok
}

// LeadSLA holds SLA durations for the lead → team handoff cycle.
type LeadSLA struct {
	// LeadHoldDays: calendar days the lead held the task before delegating.
	LeadHoldDays *float64
	// TeamExecutionDays: calendar days the team took after receiving from lead.
	TeamExecutionDays *float64
}

var dateLayouts = []string{
	"2006-01-02 15:04:05",
	"2006-01-02T15:04:05Z07:00",
	"2006-01-02",
}

func parseLeadDate(s string) (time.Time, bool) {
	for _, layout := range dateLayouts {
		if t, err := time.Parse(layout, s); err == nil {
			return t, true
		}
	}
	return time.Time{}, false
}

// CalculateLeadSLA computes how long a task stayed with the lead before being
// delegated (LeadHoldDays) and how long the team took to complete it
// (TeamExecutionDays).
//
// LeadHoldDays      = AssignedToTeamDate - AssignedToLeadDate
// TeamExecutionDays = ActualTaskDoneDate  - AssignedToTeamDate
func CalculateLeadSLA(issue JiraIssue) LeadSLA {
	var sla LeadSLA

	leadDate, hasLead := parseLeadDate(issue.AssignedToLeadDate)
	teamDate, hasTeam := parseLeadDate(issue.AssignedToTeamDate)
	doneDate, hasDone := parseLeadDate(issue.ActualTaskDoneDate)

	if hasLead && hasTeam && !teamDate.Before(leadDate) {
		days := teamDate.Sub(leadDate).Hours() / 24
		sla.LeadHoldDays = &days
	}

	if hasTeam && hasDone && !doneDate.Before(teamDate) {
		days := doneDate.Sub(teamDate).Hours() / 24
		sla.TeamExecutionDays = &days
	}

	return sla
}
