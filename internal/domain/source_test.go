package domain

import (
	"testing"
	"time"
)

func TestSourceFreshness(t *testing.T) {
	now := time.Date(2026, 9, 30, 12, 0, 0, 0, time.UTC)
	fresh := now.AddDate(0, -2, 0)
	base := Source{Provider: "sdamgia", ExternID: "fixture", URL: "https://math-ege.sdamgia.ru/problem?id=1",
		PublishedAt: &fresh, VerifiedAt: &now, DateEvidenceURL: "https://math-ege.sdamgia.ru/methodist", DateEvidence: "ЕГЭ 30.07.2026"}
	for _, tt := range []struct {
		name   string
		change func(*Source)
		want   string
	}{
		{"dated source", func(s *Source) {}, "current"},
		{"no original date", func(s *Source) { s.PublishedAt = nil }, "unverified"},
		{"no verification", func(s *Source) { s.VerifiedAt = nil }, "unverified"},
		{"no proof", func(s *Source) { s.DateEvidence = "" }, "unverified"},
		{"demo import", func(s *Source) { s.Provider = "demo" }, "unverified"},
		{"spoofed host", func(s *Source) { s.URL = "https://math-ege.sdamgia.ru.evil.test/problem?id=1" }, "unverified"},
		{"future date", func(s *Source) { d := now.Add(time.Hour); s.PublishedAt = &d }, "unverified"},
		{"future verification", func(s *Source) { d := now.Add(time.Hour); s.VerifiedAt = &d }, "unverified"},
		{"verification precedes source", func(s *Source) { d := fresh.Add(-time.Hour); s.VerifiedAt = &d }, "unverified"},
		{"inclusive one year boundary", func(s *Source) { d := now.AddDate(-1, 0, 0); s.PublishedAt = &d }, "current"},
		{"old source recently imported", func(s *Source) { d := now.AddDate(-1, 0, 0).Add(-time.Second); s.PublishedAt = &d }, "expired"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			s := base
			tt.change(&s)
			got, reason := s.Freshness(now)
			if got != tt.want || reason == "" {
				t.Fatalf("got %q (%s), want %q", got, reason, tt.want)
			}
		})
	}
	var absent *Source
	if absent.Current(now) {
		t.Fatal("nil source is current")
	}
}
