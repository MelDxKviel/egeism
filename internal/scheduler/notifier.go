package scheduler

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"time"
)

// Notifier delivers a text message to a Telegram chat. Kept as an interface so
// handlers are testable without hitting Telegram.
type Notifier interface {
	Send(ctx context.Context, chatID int64, text string) error
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

// Send posts a message to the chat.
func (t *TelegramNotifier) Send(ctx context.Context, chatID int64, text string) error {
	q := url.Values{}
	q.Set("chat_id", strconv.FormatInt(chatID, 10))
	q.Set("text", text)
	u := fmt.Sprintf("https://api.telegram.org/bot%s/sendMessage?%s", t.token, q.Encode())
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, u, nil)
	if err != nil {
		return err
	}
	resp, err := t.http.Do(req)
	if err != nil {
		return err
	}
	resp.Body.Close()
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
