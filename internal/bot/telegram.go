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
	"unicode/utf16"
)

// Telegram is a minimal Bot API transport (long-poll getUpdates + send*),
// intentionally small and dependency-free. Conversation logic lives in Bot; this
// layer owns the RICH-message send: a task with figures goes out as ONE bubble
// (photo/album with the statement as an HTML caption shown above the media —
// like the web card), tables stay aligned <pre> text, inline keyboards ride as
// reply_markup, and attached files follow as documents.
type Telegram struct {
	token  string
	http   *http.Client
	bot    *Bot
	offset int64
}

// NewTelegram wires the transport to the conversation Bot. It also registers
// itself as the Bot's command-menu so per-role command lists can be pushed to
// Telegram when a user's role becomes known.
func NewTelegram(token string, b *Bot) *Telegram {
	t := &Telegram{token: token, http: &http.Client{Timeout: 65 * time.Second}, bot: b}
	b.menu = t
	return t
}

// botCommand is one entry in the Telegram command menu (the ☰/⁄ button).
type botCommand struct {
	Command     string `json:"command"`
	Description string `json:"description"`
}

// SetChatCommands pushes the role's command list to one chat's menu, so a
// student and a teacher each see their own commands. Implements bot.CommandMenu.
func (t *Telegram) SetChatCommands(ctx context.Context, chatID int64, role string) error {
	return t.setMyCommands(ctx, commandsFor(role), chatID)
}

// setMyCommands sets the command menu. scopeChatID <= 0 sets the global default
// (all private chats); a positive id scopes the list to that chat.
func (t *Telegram) setMyCommands(ctx context.Context, cmds []botCommand, scopeChatID int64) error {
	payload := map[string]any{"commands": cmds}
	if scopeChatID > 0 {
		payload["scope"] = map[string]any{"type": "chat", "chat_id": scopeChatID}
	} else {
		payload["scope"] = map[string]any{"type": "all_private_chats"}
	}
	return t.postJSON(ctx, "setMyCommands", payload)
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
	// Publish a default command menu so commands are discoverable before we know
	// a chat's role; per-role menus are set per chat on first interaction.
	if err := t.setMyCommands(ctx, defaultCommands, 0); err != nil {
		slog.Warn("setMyCommands default", "err", err)
	}
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

// deliver renders one Reply, best rendering first:
//  1. Rich Message (sendRichMessage, Bot API 10.1): real tables/headings, and —
//     with a public web origin — figures inlined in the text; leftovers follow.
//  2. Caption bubble: statement as an HTML caption above the photo/album.
//  3. Classic: parse_mode=HTML text, then an album of figures.
//
// Attached files always follow as documents.
func (t *Telegram) deliver(ctx context.Context, r Reply) {
	if rich := strings.TrimSpace(r.RichHTML); rich != "" {
		if err := t.sendRich(ctx, r.ChatID, rich, r.Buttons); err != nil {
			slog.Warn("sendRichMessage failed; falling back to classic", "err", err)
		} else {
			t.deliverMedia(ctx, r.ChatID, r.RichMedia)
			return
		}
	}
	t.deliverClassic(ctx, r)
}

// deliverMedia sends leftover media: figures as a photo/album, files as documents.
func (t *Telegram) deliverMedia(ctx context.Context, chatID int64, media []MediaRef) {
	photos, files := splitMedia(media)
	blobs := t.fetchPhotos(ctx, photos)
	// Captions tie the follow-up bubbles to the task above them (inline embedding
	// needs a public media URL — see richhtml.go; until then these are separate).
	switch {
	case len(blobs) == 1:
		if err := t.sendFile(ctx, chatID, "sendPhoto", "photo", "figure.jpg", blobs[0], "<i>🖼 Рисунок к заданию</i>"); err != nil {
			slog.Error("sendPhoto", "err", err)
		}
	case len(blobs) > 1:
		if err := t.sendAlbum(ctx, chatID, blobs, "<i>🖼 Рисунки к заданию</i>"); err != nil {
			slog.Error("sendMediaGroup", "err", err)
		}
	}
	for _, m := range files {
		data, err := t.bot.api.FetchMedia(ctx, m.Key)
		if err != nil {
			slog.Error("fetch media", "key", m.Key, "err", err)
			continue
		}
		if err := t.sendFile(ctx, chatID, "sendDocument", "document", fileName(m), data, "<i>📎 Файлы к заданию — реши с ними</i>"); err != nil {
			slog.Error("sendDocument", "err", err)
		}
	}
}

// sendRich posts a Rich Message: extended HTML (tables, headings, inline images
// by public URL) parsed into blocks server-side, with an inline keyboard. The
// keyboard degrades (full → callback-only → none) so a rejected button URL
// doesn't cost the rich rendering.
func (t *Telegram) sendRich(ctx context.Context, chatID int64, richHTML string, buttons [][]Button) error {
	attempts := []string{markupJSON(buttons), markupJSON(callbackOnlyRows(buttons)), ""}
	seen := map[string]bool{}
	var lastErr error
	for _, markup := range attempts {
		if seen[markup] {
			continue
		}
		seen[markup] = true
		if lastErr = t.postRich(ctx, chatID, richHTML, markup); lastErr == nil {
			return nil
		}
	}
	return lastErr
}

func (t *Telegram) postRich(ctx context.Context, chatID int64, richHTML, markup string) error {
	payload := map[string]any{
		"chat_id":      chatID,
		"rich_message": map[string]any{"html": richHTML},
	}
	if markup != "" {
		payload["reply_markup"] = json.RawMessage(markup)
	}
	return t.postJSON(ctx, "sendRichMessage", payload)
}

// postJSON posts a JSON body to a Bot API method and expects a 2xx.
func (t *Telegram) postJSON(ctx context.Context, method string, payload any) error {
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	u := fmt.Sprintf("https://api.telegram.org/bot%s/%s", t.token, method)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, u, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	return t.doExpectOK(req)
}

// callbackOnlyRows strips URL buttons, keeping callback ones.
func callbackOnlyRows(rows [][]Button) [][]Button {
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

// deliverClassic is the pre-rich layout: caption bubble when the statement fits,
// else text + album; files as documents.
func (t *Telegram) deliverClassic(ctx context.Context, r Reply) {
	photos, files := splitMedia(r.Media)
	html := strings.TrimSpace(r.HTML)

	blobs := t.fetchPhotos(ctx, photos)
	rich := false
	if html != "" && len(blobs) > 0 && captionFits(html) {
		switch {
		case len(blobs) == 1:
			if err := t.sendPhotoCaption(ctx, r.ChatID, blobs[0], html, r.Buttons); err != nil {
				slog.Error("sendPhoto rich", "err", err)
			} else {
				rich = true
			}
		case len(r.Buttons) == 0: // albums cannot carry reply_markup
			if err := t.sendAlbum(ctx, r.ChatID, blobs, html); err != nil {
				slog.Error("sendMediaGroup rich", "err", err)
			} else {
				rich = true
			}
		}
	}

	if !rich {
		if html != "" {
			if err := t.sendMessageHTML(ctx, r.ChatID, html, r.Buttons); err != nil {
				slog.Error("sendMessage", "err", err)
			}
		}
		switch {
		case len(blobs) == 1:
			if err := t.sendFile(ctx, r.ChatID, "sendPhoto", "photo", "figure.jpg", blobs[0], ""); err != nil {
				slog.Error("sendPhoto", "err", err)
			}
		case len(blobs) > 1: // one album beats a burst of separate photo messages
			if err := t.sendAlbum(ctx, r.ChatID, blobs, ""); err != nil {
				slog.Error("sendMediaGroup", "err", err)
			}
		}
	}

	for _, m := range files {
		data, err := t.bot.api.FetchMedia(ctx, m.Key)
		if err != nil {
			slog.Error("fetch media", "key", m.Key, "err", err)
			continue
		}
		if err := t.sendFile(ctx, r.ChatID, "sendDocument", "document", fileName(m), data, "<i>📎 Файлы к заданию — реши с ними</i>"); err != nil {
			slog.Error("sendDocument", "err", err)
		}
	}
}

// splitMedia separates figures (sent as photos) from attached files (documents).
func splitMedia(media []MediaRef) (photos, files []MediaRef) {
	for _, m := range media {
		if m.Kind == "file" {
			files = append(files, m)
		} else {
			photos = append(photos, m)
		}
	}
	return photos, files
}

// captionLimit is Telegram's media-caption cap (1024 UTF-16 code units of
// RENDERED text). The raw HTML length is the conservative gate — tags only
// inflate it, so raw ≤ limit guarantees the visible text fits.
const captionLimit = 1024

func captionFits(html string) bool {
	return len(utf16.Encode([]rune(html))) <= captionLimit
}

// fetchPhotos downloads figures and flattens them onto white (transparent ФИПИ
// schemes must stay legible on dark chat themes). Failures are logged & skipped.
func (t *Telegram) fetchPhotos(ctx context.Context, photos []MediaRef) [][]byte {
	var out [][]byte
	for _, m := range photos {
		data, err := t.bot.api.FetchMedia(ctx, m.Key)
		if err != nil {
			slog.Error("fetch media", "key", m.Key, "err", err)
			continue
		}
		out = append(out, flattenToWhite(data))
	}
	return out
}

// sendPhotoCaption sends ONE bubble: the figure with the statement as an HTML
// caption rendered above it (show_caption_above_media), plus an inline keyboard.
func (t *Telegram) sendPhotoCaption(ctx context.Context, chatID int64, photo []byte, captionHTML string, buttons [][]Button) error {
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	_ = mw.WriteField("chat_id", strconv.FormatInt(chatID, 10))
	_ = mw.WriteField("caption", captionHTML)
	_ = mw.WriteField("parse_mode", "HTML")
	_ = mw.WriteField("show_caption_above_media", "true")
	if markup := markupJSON(buttons); markup != "" {
		_ = mw.WriteField("reply_markup", markup)
	}
	fw, err := mw.CreateFormFile("photo", "figure.jpg")
	if err != nil {
		return err
	}
	if _, err := fw.Write(photo); err != nil {
		return err
	}
	if err := mw.Close(); err != nil {
		return err
	}
	return t.postMultipart(ctx, "sendPhoto", &buf, mw.FormDataContentType())
}

// sendAlbum sends up to 10 figures as one media group; captionHTML (may be "")
// rides on the first item, shown above the album. Figures beyond 10 follow as
// plain photos (albums are capped by the Bot API).
func (t *Telegram) sendAlbum(ctx context.Context, chatID int64, blobs [][]byte, captionHTML string) error {
	group := blobs
	var rest [][]byte
	if len(group) > 10 {
		group, rest = blobs[:10], blobs[10:]
	}
	type inputMedia struct {
		Type         string `json:"type"`
		Media        string `json:"media"`
		Caption      string `json:"caption,omitempty"`
		ParseMode    string `json:"parse_mode,omitempty"`
		CaptionAbove bool   `json:"show_caption_above_media,omitempty"`
	}
	items := make([]inputMedia, len(group))
	for i := range group {
		items[i] = inputMedia{Type: "photo", Media: fmt.Sprintf("attach://p%d", i)}
	}
	if captionHTML != "" {
		items[0].Caption = captionHTML
		items[0].ParseMode = "HTML"
		items[0].CaptionAbove = true
	}
	mediaJSON, err := json.Marshal(items)
	if err != nil {
		return err
	}
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	_ = mw.WriteField("chat_id", strconv.FormatInt(chatID, 10))
	_ = mw.WriteField("media", string(mediaJSON))
	for i, b := range group {
		fw, err := mw.CreateFormFile(fmt.Sprintf("p%d", i), fmt.Sprintf("figure%d.jpg", i))
		if err != nil {
			return err
		}
		if _, err := fw.Write(b); err != nil {
			return err
		}
	}
	if err := mw.Close(); err != nil {
		return err
	}
	if err := t.postMultipart(ctx, "sendMediaGroup", &buf, mw.FormDataContentType()); err != nil {
		return err
	}
	for _, b := range rest {
		if err := t.sendFile(ctx, chatID, "sendPhoto", "photo", "figure.jpg", b, ""); err != nil {
			slog.Error("sendPhoto overflow", "err", err)
		}
	}
	return nil
}

// postMultipart posts a prepared multipart body to a Bot API method.
func (t *Telegram) postMultipart(ctx context.Context, method string, body *bytes.Buffer, contentType string) error {
	u := fmt.Sprintf("https://api.telegram.org/bot%s/%s", t.token, method)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, u, body)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", contentType)
	return t.doExpectOK(req)
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

// sendFile uploads bytes via a multipart Bot API call (sendPhoto/sendDocument);
// captionHTML (may be "") labels the bubble.
func (t *Telegram) sendFile(ctx context.Context, chatID int64, method, field, filename string, data []byte, captionHTML string) error {
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	_ = mw.WriteField("chat_id", strconv.FormatInt(chatID, 10))
	if captionHTML != "" {
		_ = mw.WriteField("caption", captionHTML)
		_ = mw.WriteField("parse_mode", "HTML")
	}
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
