// Package bot is the Telegram bot: a thin client over the API. It contains NO
// business logic — answer checking, stats, scheduling all live in the API
// (§3). The transport (internal/bot/telegram.go) is deliberately decoupled from
// the conversation logic (bot.go) so it can be swapped for a framework later.
// Presentation-only helpers (HTML formatting, flattening figures onto white)
// live in format.go / richmedia.go — that is UI, not business logic.
package bot

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// ErrNotLinked means the Telegram id is not yet bound to any account (the user
// must link via the web button + a /start <code> deep link).
var ErrNotLinked = errors.New("telegram not linked")

// APIClient is a minimal HTTP client for the egeism API.
type APIClient struct {
	base string
	http *http.Client
}

// NewAPIClient builds a client pointed at the API base URL (e.g. http://api:8080).
func NewAPIClient(base string) *APIClient {
	return &APIClient{base: base, http: &http.Client{Timeout: 20 * time.Second}}
}

// apiError carries the HTTP status and body so callers can branch on it (e.g.
// 404 → ErrNotLinked, 400/409 → show the server's Russian message).
type apiError struct {
	Status int
	Body   string
}

func (e *apiError) Error() string { return fmt.Sprintf("api %d: %s", e.Status, e.Body) }

// message returns the API's {"error": ...} text if present, else the raw body.
func (e *apiError) message() string {
	var env struct {
		Error string `json:"error"`
	}
	if json.Unmarshal([]byte(e.Body), &env) == nil && env.Error != "" {
		return env.Error
	}
	return e.Body
}

// User mirrors the API user shape (only fields the bot needs).
type User struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Role string `json:"role"`
}

// MediaRef mirrors domain.Media: a task figure/formula/file. Alt carries a
// formula's text form (⟦img:N⟧ substitution); Inline marks mid-sentence formulas.
type MediaRef struct {
	Key    string `json:"key"`
	Kind   string `json:"kind"`
	Alt    string `json:"alt,omitempty"`
	Inline bool   `json:"inline,omitempty"`
}

// TaskView mirrors the API student-facing task shape (no correct answer).
type TaskView struct {
	ID         string     `json:"id"`
	Number     int        `json:"number"`
	Statement  string     `json:"statement"`
	Media      []MediaRef `json:"media"`
	AnswerKind string     `json:"answer_kind"`
}

type practiceResp struct {
	TestID    string `json:"test_id"`
	AttemptID string `json:"attempt_id"`
}

// SubmitResult mirrors the answer-submission response.
type SubmitResult struct {
	IsCorrect bool     `json:"is_correct"`
	Solution  []string `json:"solution"`
}

// AssignmentCard mirrors domain.AssignmentCard ("что назначено" — the teacher
// read AND the student's own /tests list).
type AssignmentCard struct {
	ID          string    `json:"id"`
	TestID      string    `json:"test_id"`
	Title       string    `json:"title"`
	Kind        string    `json:"kind"`
	SubjectID   string    `json:"subject_id"`
	ScheduledAt time.Time `json:"scheduled_at"`
	Status      string    `json:"status"`
	TaskCount   int64     `json:"task_count"`
}

// AttemptSummary mirrors domain.AttemptSummary (teacher: "как решено").
type AttemptSummary struct {
	ID         string     `json:"id"`
	Title      string     `json:"title"`
	SubjectID  string     `json:"subject_id"`
	StartedAt  time.Time  `json:"started_at"`
	FinishedAt *time.Time `json:"finished_at"`
	Total      int64      `json:"total"`
	Correct    int64      `json:"correct"`
	TimeMS     int64      `json:"time_ms"`
}

// ReviewItem mirrors the API attemptReviewItem (per-task verdict + answers).
type ReviewItem struct {
	Number    int      `json:"number"`
	RawAnswer string   `json:"raw_answer"`
	IsCorrect bool     `json:"is_correct"`
	Correct   []string `json:"correct"`
}

// Forecast mirrors the scoring forecast (accuracy + predicted score).
type Forecast struct {
	Accuracy   float64 `json:"accuracy"`
	PrimaryEst int     `json:"primary_estimate"`
	PrimaryMax int     `json:"primary_max"`
	TestScore  int     `json:"test_score"`
}

// WeakSpot mirrors a weak-number stat (number + accuracy).
type WeakSpot struct {
	Number   int     `json:"number"`
	Total    int64   `json:"total"`
	Correct  int64   `json:"correct"`
	Accuracy float64 `json:"accuracy"`
}

func (c *APIClient) do(ctx context.Context, method, path, token string, body, out any) error {
	var rdr io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return err
		}
		rdr = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.base+path, rdr)
	if err != nil {
		return err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		msg, _ := io.ReadAll(resp.Body)
		return &apiError{Status: resp.StatusCode, Body: string(bytes.TrimSpace(msg))}
	}
	if out != nil {
		return json.NewDecoder(resp.Body).Decode(out)
	}
	return nil
}

// ResolveTelegram exchanges a telegram_id for a session token, if it is already
// linked to an account. An unlinked id yields ErrNotLinked.
func (c *APIClient) ResolveTelegram(ctx context.Context, tgID int64) (User, string, error) {
	var out struct {
		Token string `json:"token"`
		User  User   `json:"user"`
	}
	err := c.do(ctx, http.MethodPost, "/api/auth/telegram", "",
		map[string]any{"telegram_id": tgID}, &out)
	if err != nil {
		var ae *apiError
		if errors.As(err, &ae) && ae.Status == http.StatusNotFound {
			return User{}, "", ErrNotLinked
		}
		return User{}, "", err
	}
	return out.User, out.Token, nil
}

// LinkTelegram redeems a link code from the web, binding this telegram_id to the
// code's account. Returns the account + session token. The error message (if any)
// is the server's Russian text, safe to show the user.
func (c *APIClient) LinkTelegram(ctx context.Context, code string, tgID int64) (User, string, error) {
	var out struct {
		Token string `json:"token"`
		User  User   `json:"user"`
	}
	err := c.do(ctx, http.MethodPost, "/api/auth/telegram/link", "",
		map[string]any{"code": code, "telegram_id": tgID}, &out)
	if err != nil {
		return User{}, "", err
	}
	return out.User, out.Token, nil
}

// StartPractice opens a free-solve session and returns the attempt id.
func (c *APIClient) StartPractice(ctx context.Context, token, subject string) (string, error) {
	var out practiceResp
	err := c.do(ctx, http.MethodPost, "/api/practice", token,
		map[string]any{"subject": subject}, &out)
	return out.AttemptID, err
}

// PracticeTasks returns active, not-yet-mastered tasks for a subject (with media),
// in random order — the source for the bot's rich task flow.
func (c *APIClient) PracticeTasks(ctx context.Context, token, subject string, limit int) ([]TaskView, error) {
	q := url.Values{}
	q.Set("subject", subject)
	q.Set("limit", fmt.Sprintf("%d", limit))
	var tasks []TaskView
	err := c.do(ctx, http.MethodGet, "/api/practice/tasks?"+q.Encode(), token, nil, &tasks)
	return tasks, err
}

// TestTasks returns a composed/assigned test's tasks in order (student-safe, no
// answers) — what the bot serves when solving an assigned variant.
func (c *APIClient) TestTasks(ctx context.Context, token, testID string) ([]TaskView, error) {
	var tasks []TaskView
	err := c.do(ctx, http.MethodGet, "/api/tests/"+testID+"/tasks", token, nil, &tasks)
	return tasks, err
}

// StartAttempt opens an attempt on a test; assignmentID (may be "") ties it to
// an assignment so finishing marks the assignment done.
func (c *APIClient) StartAttempt(ctx context.Context, token, testID, assignmentID string) (string, error) {
	body := map[string]any{"test_id": testID}
	if assignmentID != "" {
		body["assignment_id"] = assignmentID
	}
	var out struct {
		ID string `json:"id"`
	}
	err := c.do(ctx, http.MethodPost, "/api/attempts", token, body, &out)
	return out.ID, err
}

// FinishAttempt closes an attempt (an assigned test's assignment flips to done).
func (c *APIClient) FinishAttempt(ctx context.Context, token, attemptID string) error {
	return c.do(ctx, http.MethodPost, "/api/attempts/"+attemptID+"/finish", token, nil, nil)
}

// SubmitAnswer submits a raw answer and returns the verdict (+ solution if wrong).
func (c *APIClient) SubmitAnswer(ctx context.Context, token, attemptID, taskID, raw string, timeMS int64) (SubmitResult, error) {
	var out SubmitResult
	err := c.do(ctx, http.MethodPost, "/api/attempts/"+attemptID+"/answers", token,
		map[string]any{"task_id": taskID, "raw_answer": raw, "time_spent_ms": timeMS}, &out)
	return out, err
}

// FetchMedia downloads a task media object by key from the public media endpoint
// (or directly if the key is already an http(s) URL — the ingest fallback).
func (c *APIClient) FetchMedia(ctx context.Context, key string) ([]byte, error) {
	u := c.base + "/api/media/" + key
	if strings.HasPrefix(key, "http") {
		u = key
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return nil, fmt.Errorf("fetch media %s: %d", key, resp.StatusCode)
	}
	return io.ReadAll(resp.Body)
}

// ---- Teacher reads ----

// ListStudents returns the students the teacher oversees.
func (c *APIClient) ListStudents(ctx context.Context, token string) ([]User, error) {
	var out []User
	err := c.do(ctx, http.MethodGet, "/api/students", token, nil, &out)
	return out, err
}

// StudentAssignments lists a student's assignments ("что назначено").
func (c *APIClient) StudentAssignments(ctx context.Context, token, studentID string) ([]AssignmentCard, error) {
	var out []AssignmentCard
	err := c.do(ctx, http.MethodGet, "/api/students/"+studentID+"/assignments", token, nil, &out)
	return out, err
}

// StudentAttempts lists a student's recent attempts ("как решено").
func (c *APIClient) StudentAttempts(ctx context.Context, token, studentID string, limit int) ([]AttemptSummary, error) {
	var out []AttemptSummary
	err := c.do(ctx, http.MethodGet, fmt.Sprintf("/api/students/%s/attempts?limit=%d", studentID, limit), token, nil, &out)
	return out, err
}

// AttemptReview returns per-task verdicts for one attempt.
func (c *APIClient) AttemptReview(ctx context.Context, token, attemptID string) ([]ReviewItem, error) {
	var out []ReviewItem
	err := c.do(ctx, http.MethodGet, "/api/attempts/"+attemptID+"/review", token, nil, &out)
	return out, err
}

// Forecast returns a student's predicted score for a subject.
func (c *APIClient) Forecast(ctx context.Context, token, studentID, subject string) (Forecast, error) {
	var out Forecast
	err := c.do(ctx, http.MethodGet, fmt.Sprintf("/api/students/%s/stats/forecast?subject=%s", studentID, subject), token, nil, &out)
	return out, err
}

// WeakSpots returns a student's weakest task numbers for a subject.
func (c *APIClient) WeakSpots(ctx context.Context, token, studentID, subject string) ([]WeakSpot, error) {
	var out []WeakSpot
	err := c.do(ctx, http.MethodGet, fmt.Sprintf("/api/students/%s/stats/weak-spots?subject=%s&min=1&limit=5", studentID, subject), token, nil, &out)
	return out, err
}
