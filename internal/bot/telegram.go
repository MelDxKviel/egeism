package bot

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"
	"time"
)

// Telegram is a minimal Bot API transport (long-poll getUpdates + sendMessage).
// It is intentionally small and dependency-free; swap for telego/telebot if the
// bot grows inline keyboards, media, etc. The conversation logic lives in Bot.
type Telegram struct {
	token  string
	http   *http.Client
	bot    *Bot
	offset int64
}

// NewTelegram wires the transport to the conversation Bot.
func NewTelegram(token string, b *Bot) *Telegram {
	return &Telegram{token: token, http: &http.Client{Timeout: 65 * time.Second}, bot: b}
}

type tgUpdate struct {
	UpdateID int64 `json:"update_id"`
	Message  *struct {
		Chat struct {
			ID int64 `json:"id"`
		} `json:"chat"`
		From struct {
			ID        int64  `json:"id"`
			FirstName string `json:"first_name"`
		} `json:"from"`
		Text string `json:"text"`
	} `json:"message"`
}

type tgResponse struct {
	OK     bool       `json:"ok"`
	Result []tgUpdate `json:"result"`
}

// Run polls for updates until ctx is cancelled, dispatching each to Bot.Handle.
func (t *Telegram) Run(ctx context.Context) error {
	slog.Info("bot polling started")
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		updates, err := t.getUpdates(ctx)
		if err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			slog.Error("getUpdates", "err", err)
			time.Sleep(3 * time.Second)
			continue
		}
		for _, u := range updates {
			t.offset = u.UpdateID + 1
			if u.Message == nil || u.Message.Text == "" {
				continue
			}
			in := InMessage{
				TelegramID: u.Message.From.ID,
				ChatID:     u.Message.Chat.ID,
				Text:       u.Message.Text,
			}
			reply := t.bot.Handle(ctx, in)
			if reply.Text != "" {
				if err := t.sendMessage(ctx, reply.ChatID, reply.Text); err != nil {
					slog.Error("sendMessage", "err", err)
				}
			}
		}
	}
}

func (t *Telegram) getUpdates(ctx context.Context) ([]tgUpdate, error) {
	q := url.Values{}
	q.Set("timeout", "50")
	q.Set("offset", strconv.FormatInt(t.offset, 10))
	u := fmt.Sprintf("https://api.telegram.org/bot%s/getUpdates?%s", t.token, q.Encode())
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	resp, err := t.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	var out tgResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, err
	}
	if !out.OK {
		return nil, fmt.Errorf("telegram getUpdates not ok")
	}
	return out.Result, nil
}

func (t *Telegram) sendMessage(ctx context.Context, chatID int64, text string) error {
	q := url.Values{}
	q.Set("chat_id", strconv.FormatInt(chatID, 10))
	q.Set("text", text)
	u := fmt.Sprintf("https://api.telegram.org/bot%s/sendMessage", t.token)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, u, nil)
	if err != nil {
		return err
	}
	req.URL.RawQuery = q.Encode()
	resp, err := t.http.Do(req)
	if err != nil {
		return err
	}
	resp.Body.Close()
	return nil
}
