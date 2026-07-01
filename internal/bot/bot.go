package bot

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"
)

// InMessage is a normalized inbound message, decoupled from the transport.
type InMessage struct {
	TelegramID int64
	ChatID     int64
	Text       string
}

// Reply is an outbound message.
type Reply struct {
	ChatID int64
	Text   string
}

// session is a user's in-flight solving state. In-memory is fine for one
// student; move to Redis when this must survive restarts or scale out.
type session struct {
	token     string // session JWT from /auth/telegram
	subject   string
	attemptID string
	taskID    string // task awaiting an answer, empty if none
	askedAt   time.Time
}

// Bot holds the API client and per-user session state.
type Bot struct {
	api      *APIClient
	mu       sync.Mutex
	sessions map[int64]*session
}

// New builds a Bot over an API client.
func New(api *APIClient) *Bot {
	return &Bot{api: api, sessions: make(map[int64]*session)}
}

var subjectAliases = map[string]string{
	"рус": "rus", "русский": "rus", "rus": "rus",
	"мат": "math", "математика": "math", "math": "math",
	"инф": "inf", "информатика": "inf", "inf": "inf",
	"общ": "soc", "обществознание": "soc", "soc": "soc",
}

// Handle processes one inbound message and returns the reply.
func (b *Bot) Handle(ctx context.Context, in InMessage) Reply {
	text := strings.TrimSpace(in.Text)
	reply := func(s string) Reply { return Reply{ChatID: in.ChatID, Text: s} }

	// Ensure the user is authenticated (token cached in the session).
	sess := b.session(in.TelegramID)
	if sess.token == "" {
		_, token, err := b.api.AuthTelegram(ctx, in.TelegramID, "Ученик")
		if err != nil {
			return reply("Не получилось подключиться к серверу. Попробуй позже.")
		}
		sess.token = token
	}

	switch {
	case text == "/start" || text == "start":
		return reply(welcome)

	case strings.HasPrefix(text, "/solve"), strings.HasPrefix(text, "/reshat"):
		arg := strings.TrimSpace(strings.TrimPrefix(strings.TrimPrefix(text, "/solve"), "/reshat"))
		code, ok := subjectAliases[strings.ToLower(arg)]
		if !ok {
			return reply("Укажи предмет: /solve рус | мат | инф | общ")
		}
		return b.startSubject(ctx, sess, in.ChatID, code)

	case text == "/next":
		if sess.subject == "" {
			return reply("Сначала выбери предмет: /solve рус")
		}
		return b.serveTask(ctx, sess, in.ChatID)

	default:
		if code, ok := subjectAliases[strings.ToLower(text)]; ok {
			return b.startSubject(ctx, sess, in.ChatID, code)
		}
		if sess.taskID == "" {
			return reply("Отправь ответ на задание или выбери предмет: /solve рус")
		}
		return b.submit(ctx, sess, in.ChatID, text)
	}
}

func (b *Bot) startSubject(ctx context.Context, sess *session, chatID int64, code string) Reply {
	attemptID, err := b.api.StartPractice(ctx, sess.token, code)
	if err != nil {
		return Reply{ChatID: chatID, Text: "Не удалось начать тренировку. Попробуй позже."}
	}
	sess.subject = code
	sess.attemptID = attemptID
	return b.serveTask(ctx, sess, chatID)
}

func (b *Bot) serveTask(ctx context.Context, sess *session, chatID int64) Reply {
	task, ok, err := b.api.NextBotTask(ctx, sess.token, sess.subject)
	if err != nil {
		return Reply{ChatID: chatID, Text: "Не удалось получить задание."}
	}
	if !ok {
		return Reply{ChatID: chatID, Text: "Пока нет заданий, которые можно решить в чате. Открой сайт для остальных."}
	}
	sess.taskID = task.ID
	sess.askedAt = time.Now()
	return Reply{ChatID: chatID, Text: fmt.Sprintf("Задание №%d:\n\n%s\n\nОтправь ответ сообщением.", task.Number, task.Statement)}
}

func (b *Bot) submit(ctx context.Context, sess *session, chatID int64, raw string) Reply {
	elapsed := time.Since(sess.askedAt).Milliseconds()
	res, err := b.api.SubmitAnswer(ctx, sess.token, sess.attemptID, sess.taskID, raw, elapsed)
	if err != nil {
		return Reply{ChatID: chatID, Text: "Не удалось проверить ответ. Попробуй ещё раз."}
	}
	sess.taskID = "" // task consumed
	if res.IsCorrect {
		return Reply{ChatID: chatID, Text: "✅ Верно! Дальше — /next"}
	}
	msg := "❌ Неверно."
	if len(res.Solution) > 0 {
		msg += "\nПравильный ответ: " + strings.Join(res.Solution, " / ")
	}
	msg += "\nСледующее — /next"
	return Reply{ChatID: chatID, Text: msg}
}

func (b *Bot) session(tgID int64) *session {
	b.mu.Lock()
	defer b.mu.Unlock()
	s, ok := b.sessions[tgID]
	if !ok {
		s = &session{}
		b.sessions[tgID] = s
	}
	return s
}

const welcome = `Привет! Я помогу готовиться к ЕГЭ.
Команды:
  /solve рус — начать решать русский (мат / инф / общ)
  /next — следующее задание
Просто пришли ответ сообщением, когда решишь.`
