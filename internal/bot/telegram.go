package bot

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"mime/multipart"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// Telegram is a minimal Bot API transport (long-poll getUpdates + send*). It is
// intentionally small and dependency-free; swap for telego/telebot if the bot
// grows inline keyboards, etc. Conversation logic lives in Bot; this layer also
// owns the rich-message send (HTML text + fetched, white-flattened photos/files).
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
	// CallbackQuery arrives when an inline button is pressed — including buttons
	// on notifications the WORKER sent (same bot token, one long-poll).
	CallbackQuery *struct {
		ID   string `json:"id"`
		From struct {
			ID        int64  `json:"id"`
			FirstName string `json:"first_name"`
		} `json:"from"`
		Message *struct {
			Chat struct {
				ID int64 `json:"id"`
			} `json:"chat"`
		} `json:"message"`
		Data string `json:"data"`
	} `json:"callback_query"`
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
			if cq := u.CallbackQuery; cq != nil {
				// Ack first so the button stops spinning even if handling fails.
				t.answerCallback(ctx, cq.ID)
				if cq.Message == nil || cq.Data == "" {
					continue
				}
				in := InCallback{
					TelegramID: cq.From.ID,
					ChatID:     cq.Message.Chat.ID,
					Data:       cq.Data,
					FirstName:  cq.From.FirstName,
				}
				t.deliver(ctx, t.bot.HandleCallback(ctx, in))
				continue
			}
			if u.Message == nil || u.Message.Text == "" {
				continue
			}
			in := InMessage{
				TelegramID: u.Message.From.ID,
				ChatID:     u.Message.Chat.ID,
				Text:       u.Message.Text,
				FirstName:  u.Message.From.FirstName,
			}
			t.deliver(ctx, t.bot.Handle(ctx, in))
		}
	}
}

// answerCallback acks a button press (required, or the client shows a spinner
// for ~30s). Errors are logged only — the reply itself still goes out.
func (t *Telegram) answerCallback(ctx context.Context, id string) {
	form := url.Values{}
	form.Set("callback_query_id", id)
	u := fmt.Sprintf("https://api.telegram.org/bot%s/answerCallbackQuery", t.token)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, u, strings.NewReader(form.Encode()))
	if err != nil {
		return
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	if err := t.doExpectOK(req); err != nil {
		slog.Error("answerCallbackQuery", "err", err)
	}
}

// deliver renders one Reply: the HTML text (with its inline keyboard), then each
// media as a photo (figures, flattened onto white) or a document (attached files).
func (t *Telegram) deliver(ctx context.Context, r Reply) {
	if strings.TrimSpace(r.HTML) != "" {
		if err := t.sendMessageHTML(ctx, r.ChatID, r.HTML, r.Buttons); err != nil {
			slog.Error("sendMessage", "err", err)
		}
	}
	for _, m := range r.Media {
		data, err := t.bot.api.FetchMedia(ctx, m.Key)
		if err != nil {
			slog.Error("fetch media", "key", m.Key, "err", err)
			continue
		}
		if m.Kind == "file" {
			if err := t.sendFile(ctx, r.ChatID, "sendDocument", "document", fileName(m), data); err != nil {
				slog.Error("sendDocument", "err", err)
			}
			continue
		}
		if err := t.sendFile(ctx, r.ChatID, "sendPhoto", "photo", "figure.jpg", flattenToWhite(data)); err != nil {
			slog.Error("sendPhoto", "err", err)
		}
	}
}

func fileName(m MediaRef) string {
	if name := strings.TrimSpace(m.Alt); name != "" {
		return name
	}
	return "file"
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

// sendMessageHTML posts an HTML-formatted message (parse_mode=HTML) with an
// optional inline keyboard. The body is form-encoded (not query) so long
// statements aren't capped by URL length. If Telegram rejects the keyboard
// (e.g. an invalid button URL), it retries without it — text beats nothing.
func (t *Telegram) sendMessageHTML(ctx context.Context, chatID int64, html string, buttons [][]Button) error {
	form := url.Values{}
	form.Set("chat_id", strconv.FormatInt(chatID, 10))
	form.Set("text", html)
	form.Set("parse_mode", "HTML")
	form.Set("disable_web_page_preview", "true")
	if markup := markupJSON(buttons); markup != "" {
		form.Set("reply_markup", markup)
		if err := t.postForm(ctx, "sendMessage", form); err == nil {
			return nil
		} else {
			slog.Warn("sendMessage with keyboard failed; retrying without", "err", err)
		}
		form.Del("reply_markup")
	}
	return t.postForm(ctx, "sendMessage", form)
}

// markupJSON renders button rows as Telegram reply_markup JSON ("" if none).
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

// postForm posts a form-encoded Bot API call and expects a 2xx.
func (t *Telegram) postForm(ctx context.Context, method string, form url.Values) error {
	u := fmt.Sprintf("https://api.telegram.org/bot%s/%s", t.token, method)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, u, strings.NewReader(form.Encode()))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	return t.doExpectOK(req)
}

// sendFile uploads bytes via a multipart Bot API call (sendPhoto/sendDocument).
func (t *Telegram) sendFile(ctx context.Context, chatID int64, method, field, filename string, data []byte) error {
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	_ = mw.WriteField("chat_id", strconv.FormatInt(chatID, 10))
	fw, err := mw.CreateFormFile(field, filename)
	if err != nil {
		return err
	}
	if _, err := fw.Write(data); err != nil {
		return err
	}
	if err := mw.Close(); err != nil {
		return err
	}
	u := fmt.Sprintf("https://api.telegram.org/bot%s/%s", t.token, method)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, u, &buf)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", mw.FormDataContentType())
	return t.doExpectOK(req)
}

// doExpectOK runs a request and treats any non-2xx as an error (with the body).
func (t *Telegram) doExpectOK(req *http.Request) error {
	resp, err := t.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		var b bytes.Buffer
		_, _ = b.ReadFrom(resp.Body)
		return fmt.Errorf("telegram %s: %d %s", req.URL.Path, resp.StatusCode, strings.TrimSpace(b.String()))
	}
	return nil
}
