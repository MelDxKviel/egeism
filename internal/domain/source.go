package domain

import (
	"net/url"
	"strings"
	"time"
)

// TrustedSourceURL accepts task/evidence links only on the supported real EGE
// providers. An arbitrary dataset, URL suffix or import timestamp is no proof.
func TrustedSourceURL(provider, raw string) bool {
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "https" || u.User != nil || u.Port() != "" {
		return false
	}
	host := u.Hostname()
	switch provider {
	case "fipi":
		return host == "fipi.ru" || host == "doc.fipi.ru" || host == "ege.fipi.ru"
	case "openfipi":
		return host == "openfipi.devinf.ru" || TrustedSourceURL("fipi", raw)
	case "sdamgia":
		for _, subject := range []string{"math", "rus", "inf", "soc"} {
			if host == subject+"-ege.sdamgia.ru" || host == subject+".ege.sdamgia.ru" {
				return true
			}
		}
		return host == "ege.sdamgia.ru" || TrustedSourceURL("fipi", raw)
	}
	return false
}

// Freshness is evaluated at read time: active tasks age out without a cron or
// destructive migration. Existing assignments/history remain available.
func (s *Source) Freshness(now time.Time) (string, string) {
	if s == nil || strings.TrimSpace(s.ExternID) == "" || !TrustedSourceURL(s.Provider, s.URL) {
		return "unverified", "Нет подтверждённого источника ЕГЭ"
	}
	if s.PublishedAt == nil || s.PublishedAt.IsZero() || s.VerifiedAt == nil || s.VerifiedAt.IsZero() ||
		strings.TrimSpace(s.DateEvidence) == "" || !TrustedSourceURL(s.Provider, s.DateEvidenceURL) {
		return "unverified", "Дата исходного задания не подтверждена"
	}
	if s.PublishedAt.After(now) || s.VerifiedAt.After(now) || s.VerifiedAt.Before(*s.PublishedAt) {
		return "unverified", "Некорректная дата источника или проверки"
	}
	cutoff := now.AddDate(-1, 0, 0)
	// PostgreSQL's interval '1 year' clamps Feb 29 to Feb 28; Go normalizes
	// it to March 1. Match the database policy on that boundary.
	if now.Month() == time.February && now.Day() == 29 {
		cutoff = cutoff.AddDate(0, 0, -1)
	}
	if s.PublishedAt.Before(cutoff) {
		return "expired", "Задание старше одного года"
	}
	return "current", "Дата источника подтверждена; заданию не больше года"
}

func (s *Source) Current(now time.Time) bool {
	freshness, _ := s.Freshness(now)
	return freshness == "current"
}
