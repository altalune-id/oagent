package fakes

import (
	"context"

	"altalune.id/oagent/apperror"
)

// ReportSink records incidents for assertions.
type ReportSink struct {
	Incidents []apperror.Incident
}

// Report implements apperror.ReportSink.
func (s *ReportSink) Report(_ context.Context, inc apperror.Incident) {
	s.Incidents = append(s.Incidents, inc)
}

// IncidentCounter records the codes it is ticked with.
type IncidentCounter struct {
	Codes []string
}

// Inc implements apperror.IncidentCounter.
func (c *IncidentCounter) Inc(_ context.Context, code string) {
	c.Codes = append(c.Codes, code)
}
