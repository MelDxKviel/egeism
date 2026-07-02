package scheduler

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// Button is one inline-keyboard button: Text plus either a URL to open or
// CallbackData for the bot to handle (the worker and the bot share one Telegram
// identity — one token — so callbacks from worker-sent messages arrive at the
// bot's long-poll).
type Button struct {
	Text string
	URL  string
	Data string
}

// Notifier delivers a message to a Telegram chat. Kept as an interface so
// handlers are testable without hitting Telegram.
type Notifier interface {
	Send(ctx context.Context, chatID int64, text string) error
	// SendHTML posts an HTML-formatted message with an optional inline keyboard
	// (one []Button per row). Implementations must degrade gracefully — a
	// notification is never lost to styling.
	SendHTML(ctx context.Context, chatID int64, html string, rows [][]Button) error
}

// TelegramNotifier is the production Notifier using the Bot API sendMessage.
type TelegramNotifier struct {
	token string
	http  *http.Client
}

// NewTelegramNotifier builds a Telegram-backed Notifier.
func NewTelegramNotifier(token string) *TelegramNotifier {
	return &TelegramNotifier{token: token, http: &http.Client{Timeout: 15 * time.Second}}
}

// Send posts a plain-text message to the chat.
func (t *TelegramNotifier) Send(ctx context.Context, chatID int64, text string) error {
	form := url.Values{}
	form.Set("chat_id", strconv.FormatInt(chatID, 10))
	form.Set("text", text)
	return t.post(ctx, form)
}

// SendHTML posts an HTML message with an inline keyboard, degrading in steps so
// a notification is never lost to styling: full keyboard → callback-only
// keyboard (an invalid URL button, e.g. a localhost WEB_URL, must not kill the
// «решать тут» button) → no keyboard.
func (t *TelegramNotifier) SendHTML(ctx context.Context, chatID int64, html string, rows [][]Button) error {
	form := url.Values{}
	form.Set("chat_id", strconv.FormatInt(chatID, 10))
	form.Set("text", html)
	form.Set("parse_mode", "HTML")
	form.Set("disable_web_page_preview", "true")
	attempts := []string{markupJSON(rows), markupJSON(callbackOnly(rows)), ""}
	var lastErr error
	seen := map[string]bool{}
	for _, markup := range attempts {
		if seen[markup] {
			continue
		}
		seen[markup] = true
		if markup == "" {
			form.Del("reply_markup")
		} else {
			form.Set("reply_markup", markup)
		}
		if lastErr = t.post(ctx, form); lastErr == nil {
			return nil
		}
	}
	return lastErr
}

// callbackOnly strips URL buttons, keeping only callback ones.
func callbackOnly(rows [][]Button) [][]Button {
	var out [][]Button
	for _, row := range rows {
		var r []Button
		for _, b := range row {
			if b.Data != "" {
				r = append(r, b)
			}
		}
		if len(r) > 0 {
			out = append(out, r)
		}
	}
	return out
}

// markupJSON renders rows as Telegram reply_markup JSON ("" when no buttons).
func markupJSON(rows [][]Button) string {
	type tgBtn struct {
		Text string `json:"text"`
		URL  string `json:"url,omitempty"`
		Data string `json:"callback_data,omitempty"`
	}
	var kb [][]tgBtn
	for _, row := range rows {
		var r []tgBtn
		for _, b := range row {
			if b.Text == "" || (b.URL == "" && b.Data == "") {
				continue
			}
			r = append(r, tgBtn{Text: b.Text, URL: b.URL, Data: b.Data})
		}
		if len(r) > 0 {
			kb = append(kb, r)
		}
	}
	if len(kb) == 0 {
		return ""
	}
	raw, err := json.Marshal(map[string]any{"inline_keyboard": kb})
	if err != nil {
		return ""
	}
	return string(raw)
}

func (t *TelegramNotifier) post(ctx context.Context, form url.Values) error {
	u := fmt.Sprintf("https://api.telegram.org/bot%s/sendMessage", t.token)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, u, strings.NewReader(form.Encode()))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := t.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return fmt.Errorf("telegram sendMessage: status %d", resp.StatusCode)
	}
	return nil
}

// LogNotifier is a no-network Notifier for local dev without a bot token.
type LogNotifier struct {
	Log func(chatID int64, text string)
}

// Send calls the log func.
func (l LogNotifier) Send(_ context.Context, chatID int64, text string) error {
	if l.Log != nil {
		l.Log(chatID, text)
	}
	return nil
}

// SendHTML logs the HTML body (buttons are dropped — nothing to press in a log).
func (l LogNotifier) SendHTML(_ context.Context, chatID int64, html string, _ [][]Button) error {
	if l.Log != nil {
		l.Log(chatID, html)
	}
	return nil
}
