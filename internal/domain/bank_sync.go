package domain

import (
	"time"

	"github.com/google/uuid"
)

// BankSyncJob is a durable, answer-free progress record. Source I/O never runs
// in the request that creates it; API replicas share the same queue and status.
type BankSyncJob struct {
	ID         uuid.UUID       `json:"id"`
	Subject    SubjectCode     `json:"subject"`
	Number     int             `json:"number"`
	Limit      int             `json:"limit"`
	Active     bool            `json:"active"`
	Kind       string          `json:"kind"`
	State      string          `json:"state"`
	CreatedAt  time.Time       `json:"created_at"`
	StartedAt  *time.Time      `json:"started_at,omitempty"`
	FinishedAt *time.Time      `json:"finished_at,omitempty"`
	Result     *BankSyncResult `json:"result,omitempty"`
	Error      string          `json:"error,omitempty"`
	Attempts   int             `json:"-"` // lease generation: a stale worker cannot overwrite its replacement
}

type BankSyncResult struct {
	Fetched  int    `json:"fetched"`
	Inserted int    `json:"inserted"`
	Skipped  int    `json:"skipped"`
	Invalid  int    `json:"invalid"`
	Promoted int    `json:"promoted"`
	Held     int    `json:"held"`
	Source   string `json:"source"`
	Updated  int    `json:"updated,omitempty"`
	Scanned  int    `json:"scanned,omitempty"`
}

func (j BankSyncJob) Pending() bool { return j.State == "queued" || j.State == "running" }
