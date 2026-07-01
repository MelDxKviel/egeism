// Package bot is the Telegram bot: a thin client over the API. It contains NO
// business logic — answer checking, stats, scheduling all live in the API
// (§3). The transport (internal/bot/telegram.go) is deliberately decoupled from
// the conversation logic (bot.go) so it can be swapped for a framework later.
package bot

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"
)

// APIClient is a minimal HTTP client for the egeism API.
type APIClient struct {
	base string
	http *http.Client
}

// NewAPIClient builds a client pointed at the API base URL (e.g. http://api:8080).
func NewAPIClient(base string) *APIClient {
	return &APIClient{base: base, http: &http.Client{Timeout: 15 * time.Second}}
}

// User mirrors the API user shape (only fields the bot needs).
type User struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Role string `json:"role"`
}

// TaskView mirrors the API student-facing task shape.
type TaskView struct {
	ID          string `json:"id"`
	Number      int    `json:"number"`
	Statement   string `json:"statement"`
	AnswerKind  string `json:"answer_kind"`
	BotSolvable bool   `json:"bot_solvable"`
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
		return fmt.Errorf("api %s %s: %d %s", method, path, resp.StatusCode, bytes.TrimSpace(msg))
	}
	if out != nil {
		return json.NewDecoder(resp.Body).Decode(out)
	}
	return nil
}

// AuthTelegram resolves a Telegram id to a user and returns a session token
// (provisioning the student on first contact).
func (c *APIClient) AuthTelegram(ctx context.Context, tgID int64, name string) (User, string, error) {
	var out struct {
		Token string `json:"token"`
		User  User   `json:"user"`
	}
	err := c.do(ctx, http.MethodPost, "/api/auth/telegram", "",
		map[string]any{"telegram_id": tgID, "name": name}, &out)
	return out.User, out.Token, err
}

// StartPractice opens a free-solve session and returns the attempt id.
func (c *APIClient) StartPractice(ctx context.Context, userID, subject string) (string, error) {
	var out practiceResp
	err := c.do(ctx, http.MethodPost, "/api/practice", userID,
		map[string]any{"subject": subject}, &out)
	return out.AttemptID, err
}

// NextBotTask returns the first bot-solvable active task for a subject.
func (c *APIClient) NextBotTask(ctx context.Context, userID, subject string) (TaskView, bool, error) {
	q := url.Values{}
	q.Set("subject", subject)
	q.Set("status", "active")
	q.Set("limit", "50")
	var tasks []TaskView
	if err := c.do(ctx, http.MethodGet, "/api/tasks?"+q.Encode(), userID, nil, &tasks); err != nil {
		return TaskView{}, false, err
	}
	for _, t := range tasks {
		if t.BotSolvable {
			return t, true, nil
		}
	}
	return TaskView{}, false, nil
}

// SubmitAnswer submits a raw answer and returns the verdict (+ solution if wrong).
func (c *APIClient) SubmitAnswer(ctx context.Context, userID, attemptID, taskID, raw string, timeMS int64) (SubmitResult, error) {
	var out SubmitResult
	err := c.do(ctx, http.MethodPost, "/api/attempts/"+attemptID+"/answers", userID,
		map[string]any{"task_id": taskID, "raw_answer": raw, "time_spent_ms": timeMS}, &out)
	return out, err
}
