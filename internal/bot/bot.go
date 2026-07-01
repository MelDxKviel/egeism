package bot

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"
)

// InMessage is a normalized inbound message, decoupled from the transport.
type InMessage struct {
	TelegramID int64
	ChatID     int64
	Text       string
	FirstName  string
}

// Reply is an outbound message: an HTML text (parse_mode=HTML) followed by any
// media (figures/files) the transport fetches and sends. Either part may be empty.
type Reply struct {
	ChatID int64
	HTML   string
	Media  []MediaRef
}

// session is a user's in-flight state. In-memory is fine for one student + one
// teacher; move to Redis when this must survive restarts or scale out.
type session struct {
	token string // session JWT from /auth/telegram (empty until resolved/linked)
	user  User   // resolved account (role decides the command set)

	// student solving state
	subject   string
	attemptID string
	queue     []TaskView // remaining practice tasks this run
	current   *TaskView  // task awaiting an answer, nil if none
	askedAt   time.Time

	// teacher browsing state
	students []User           // last listed students (index → student)
	active   *User            // selected student
	attempts []AttemptSummary // last listed attempts (index → attempt, for /review)
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

var subjectTitles = map[string]string{
	"rus": "Русский язык", "math": "Математика", "inf": "Информатика", "soc": "Обществознание",
}

var subjectOrder = []string{"rus", "math", "inf", "soc"}

// Handle processes one inbound message and returns the reply.
func (b *Bot) Handle(ctx context.Context, in InMessage) Reply {
	text := strings.TrimSpace(in.Text)
	sess := b.session(in.TelegramID)

	// 1) Linking works regardless of current auth: /start <code> (deep link) or
	//    /link <code> binds this Telegram to an existing web account.
	if code, ok := parseLinkCode(text); ok {
		return b.link(ctx, sess, in, code)
	}

	// 2) Ensure we have a session token for this Telegram id.
	if sess.token == "" {
		user, token, err := b.api.ResolveTelegram(ctx, in.TelegramID)
		if err != nil {
			if errors.Is(err, ErrNotLinked) {
				return b.text(in.ChatID, needLinkMsg)
			}
			return b.text(in.ChatID, "Не получилось подключиться к серверу. Попробуй позже.")
		}
		sess.token, sess.user = token, user
	}

	if sess.user.Role == "teacher" {
		return b.handleTeacher(ctx, sess, in, text)
	}
	return b.handleStudent(ctx, sess, in, text)
}

// --- linking ---

// parseLinkCode extracts a code from "/start <code>" (deep link) or "/link <code>".
func parseLinkCode(text string) (string, bool) {
	for _, pref := range []string{"/start", "/link", "/привязать"} {
		if strings.HasPrefix(text, pref) {
			arg := strings.TrimSpace(strings.TrimPrefix(text, pref))
			if arg != "" {
				return strings.Fields(arg)[0], true
			}
		}
	}
	return "", false
}

func (b *Bot) link(ctx context.Context, sess *session, in InMessage, code string) Reply {
	user, token, err := b.api.LinkTelegram(ctx, code, in.TelegramID)
	if err != nil {
		var ae *apiError
		if errors.As(err, &ae) {
			return b.text(in.ChatID, "Не удалось привязать: "+ae.message())
		}
		return b.text(in.ChatID, "Не удалось привязать аккаунт. Попробуй позже.")
	}
	// Reset any stale state and greet as the linked account.
	*sess = session{token: token, user: user}
	return b.text(in.ChatID, "✅ Аккаунт привязан: "+user.Name+".\n\n"+welcomeFor(user.Role))
}

// --- student ---

func (b *Bot) handleStudent(ctx context.Context, sess *session, in InMessage, text string) Reply {
	switch {
	case text == "/start" || text == "/help":
		return b.text(in.ChatID, welcomeStudent)

	case strings.HasPrefix(text, "/solve"), strings.HasPrefix(text, "/reshat"):
		arg := strings.TrimSpace(strings.TrimPrefix(strings.TrimPrefix(text, "/solve"), "/reshat"))
		code, ok := subjectAliases[strings.ToLower(arg)]
		if !ok {
			return b.text(in.ChatID, "Укажи предмет: /solve рус | мат | инф | общ")
		}
		return b.startSubject(ctx, sess, in.ChatID, code)

	case text == "/next":
		if sess.subject == "" {
			return b.text(in.ChatID, "Сначала выбери предмет: /solve рус")
		}
		return b.serveTask(ctx, sess, in.ChatID)

	default:
		if code, ok := subjectAliases[strings.ToLower(text)]; ok {
			return b.startSubject(ctx, sess, in.ChatID, code)
		}
		if sess.current == nil {
			return b.text(in.ChatID, "Отправь ответ на задание или выбери предмет: /solve рус")
		}
		return b.submit(ctx, sess, in.ChatID, text)
	}
}

func (b *Bot) startSubject(ctx context.Context, sess *session, chatID int64, code string) Reply {
	attemptID, err := b.api.StartPractice(ctx, sess.token, code)
	if err != nil {
		return b.text(chatID, "Не удалось начать тренировку. Попробуй позже.")
	}
	sess.subject = code
	sess.attemptID = attemptID
	sess.queue = nil
	sess.current = nil
	return b.serveTask(ctx, sess, chatID)
}

func (b *Bot) serveTask(ctx context.Context, sess *session, chatID int64) Reply {
	if len(sess.queue) == 0 {
		tasks, err := b.api.PracticeTasks(ctx, sess.token, sess.subject, 15)
		if err != nil {
			return b.text(chatID, "Не удалось получить задания.")
		}
		sess.queue = tasks
	}
	if len(sess.queue) == 0 {
		return b.text(chatID, "🎉 По этому предмету пока нет заданий для тренировки. Выбери другой: /solve рус | мат | инф | общ")
	}
	task := sess.queue[0]
	sess.queue = sess.queue[1:]
	sess.current = &task
	sess.askedAt = time.Now()
	return b.taskReply(chatID, task)
}

func (b *Bot) submit(ctx context.Context, sess *session, chatID int64, raw string) Reply {
	elapsed := time.Since(sess.askedAt).Milliseconds()
	res, err := b.api.SubmitAnswer(ctx, sess.token, sess.attemptID, sess.current.ID, raw, elapsed)
	if err != nil {
		return b.text(chatID, "Не удалось проверить ответ. Попробуй ещё раз.")
	}
	sess.current = nil // task consumed
	if res.IsCorrect {
		return b.text(chatID, "✅ Верно! Дальше — /next")
	}
	msg := "❌ Неверно."
	if len(res.Solution) > 0 {
		msg += "\nПравильный ответ: " + strings.Join(res.Solution, " / ")
	}
	msg += "\nСледующее — /next"
	return b.text(chatID, msg)
}

// taskReply builds the rich message for a task: the statement as Telegram HTML
// (aligned tables, inline formulas as text) plus the figures/files to send after.
func (b *Bot) taskReply(chatID int64, t TaskView) Reply {
	html := fmt.Sprintf("<b>Задание №%d</b>", t.Number)
	if body := statementToHTML(t.Statement, t.Media); strings.TrimSpace(body) != "" {
		html += "\n\n" + body
	}
	html += "\n\n<i>Отправь ответ сообщением.</i>"
	return Reply{ChatID: chatID, HTML: html, Media: attachments(t.Media)}
}

// attachments selects media to send after the statement: block figures/tables,
// attached files, and inline formulas with no alt text (so they aren't lost).
func attachments(media []MediaRef) []MediaRef {
	var out []MediaRef
	for _, m := range media {
		switch {
		case m.Kind == "file":
			out = append(out, m)
		case m.Inline:
			if m.Alt == "" {
				out = append(out, m)
			}
		default:
			out = append(out, m) // block image/table figure
		}
	}
	return out
}

// --- teacher (read-only: stats, что назначено, как решено) ---

func (b *Bot) handleTeacher(ctx context.Context, sess *session, in InMessage, text string) Reply {
	lower := strings.ToLower(text)
	switch {
	case text == "/start" || text == "/help":
		return b.text(in.ChatID, welcomeTeacher)

	case text == "/students" || lower == "ученики":
		return b.listStudents(ctx, sess, in.ChatID)

	case strings.HasPrefix(text, "/student"):
		arg := strings.TrimSpace(strings.TrimPrefix(text, "/student"))
		return b.selectStudent(sess, in.ChatID, arg)

	case strings.HasPrefix(lower, "/stats"):
		arg := strings.TrimSpace(text[len("/stats"):])
		return b.teacherStats(ctx, sess, in.ChatID, arg)

	case text == "/assigned" || lower == "назначено":
		return b.teacherAssigned(ctx, sess, in.ChatID)

	case text == "/attempts" || lower == "решения":
		return b.teacherAttempts(ctx, sess, in.ChatID)

	case strings.HasPrefix(text, "/review"):
		arg := strings.TrimSpace(strings.TrimPrefix(text, "/review"))
		return b.teacherReview(ctx, sess, in.ChatID, arg)

	default:
		// A bare number selects a student from the last /students list.
		if _, err := strconv.Atoi(text); err == nil && len(sess.students) > 0 {
			return b.selectStudent(sess, in.ChatID, text)
		}
		return b.text(in.ChatID, welcomeTeacher)
	}
}

func (b *Bot) listStudents(ctx context.Context, sess *session, chatID int64) Reply {
	students, err := b.api.ListStudents(ctx, sess.token)
	if err != nil {
		return b.text(chatID, "Не удалось получить список учеников.")
	}
	if len(students) == 0 {
		return b.text(chatID, "Учеников пока нет.")
	}
	sess.students = students
	var sb strings.Builder
	sb.WriteString("Ученики:\n")
	for i, s := range students {
		fmt.Fprintf(&sb, "%d. %s\n", i+1, s.Name)
	}
	if len(students) == 1 {
		sess.active = &students[0]
		sb.WriteString("\nВыбран: " + students[0].Name + ". Команды: /stats, /assigned, /attempts")
	} else {
		sb.WriteString("\nВыбери ученика: /student <номер>")
	}
	return b.text(chatID, sb.String())
}

func (b *Bot) selectStudent(sess *session, chatID int64, arg string) Reply {
	n, err := strconv.Atoi(strings.TrimSpace(arg))
	if err != nil || n < 1 || n > len(sess.students) {
		if len(sess.students) == 0 {
			return b.text(chatID, "Сначала запроси список: /students")
		}
		return b.text(chatID, "Укажи номер ученика: /student 1")
	}
	sess.active = &sess.students[n-1]
	return b.text(chatID, "Выбран: "+sess.active.Name+"\nКоманды: /stats [предмет], /assigned, /attempts")
}

func (b *Bot) requireActive(sess *session, chatID int64) (*User, *Reply) {
	if sess.active == nil {
		r := b.text(chatID, "Сначала выбери ученика: /students")
		return nil, &r
	}
	return sess.active, nil
}

func (b *Bot) teacherStats(ctx context.Context, sess *session, chatID int64, arg string) Reply {
	stud, stop := b.requireActive(sess, chatID)
	if stop != nil {
		return *stop
	}
	// With a subject: weak spots + forecast for it. Without: a 4-subject overview.
	if code, ok := subjectAliases[strings.ToLower(arg)]; ok && arg != "" {
		return b.teacherSubjectStats(ctx, sess, chatID, stud, code)
	}
	var sb strings.Builder
	fmt.Fprintf(&sb, "Статистика · %s\n", stud.Name)
	for _, code := range subjectOrder {
		f, err := b.api.Forecast(ctx, sess.token, stud.ID, code)
		if err != nil {
			continue
		}
		if f.Accuracy == 0 {
			fmt.Fprintf(&sb, "\n%s: нет данных", subjectTitles[code])
			continue
		}
		fmt.Fprintf(&sb, "\n%s: точность %.0f%%, прогноз ~%d б. (тест %d)",
			subjectTitles[code], f.Accuracy*100, f.PrimaryEst, f.TestScore)
	}
	sb.WriteString("\n\nПодробнее по предмету: /stats мат")
	return b.text(chatID, sb.String())
}

func (b *Bot) teacherSubjectStats(ctx context.Context, sess *session, chatID int64, stud *User, code string) Reply {
	var sb strings.Builder
	fmt.Fprintf(&sb, "%s · %s\n", stud.Name, subjectTitles[code])
	f, err := b.api.Forecast(ctx, sess.token, stud.ID, code)
	if err == nil {
		fmt.Fprintf(&sb, "Точность %.0f%%, прогноз ~%d из %d (тест %d)\n",
			f.Accuracy*100, f.PrimaryEst, f.PrimaryMax, f.TestScore)
	}
	weak, err := b.api.WeakSpots(ctx, sess.token, stud.ID, code)
	if err == nil && len(weak) > 0 {
		sb.WriteString("\nСлабые номера:")
		for _, ws := range weak {
			fmt.Fprintf(&sb, "\n№%d — %.0f%% (%d/%d)", ws.Number, ws.Accuracy*100, ws.Correct, ws.Total)
		}
	} else {
		sb.WriteString("\nСлабых номеров пока не видно.")
	}
	return b.text(chatID, sb.String())
}

func (b *Bot) teacherAssigned(ctx context.Context, sess *session, chatID int64) Reply {
	stud, stop := b.requireActive(sess, chatID)
	if stop != nil {
		return *stop
	}
	cards, err := b.api.StudentAssignments(ctx, sess.token, stud.ID)
	if err != nil {
		return b.text(chatID, "Не удалось получить назначения.")
	}
	if len(cards) == 0 {
		return b.text(chatID, stud.Name+": назначений нет.")
	}
	var sb strings.Builder
	fmt.Fprintf(&sb, "Назначено · %s\n", stud.Name)
	for _, c := range cards {
		fmt.Fprintf(&sb, "\n• %s — %s, %d зад., %s",
			c.Title, c.ScheduledAt.Format("02.01 15:04"), c.TaskCount, statusRU(c.Status))
	}
	return b.text(chatID, sb.String())
}

func (b *Bot) teacherAttempts(ctx context.Context, sess *session, chatID int64) Reply {
	stud, stop := b.requireActive(sess, chatID)
	if stop != nil {
		return *stop
	}
	items, err := b.api.StudentAttempts(ctx, sess.token, stud.ID, 10)
	if err != nil {
		return b.text(chatID, "Не удалось получить решения.")
	}
	if len(items) == 0 {
		return b.text(chatID, stud.Name+": решений пока нет.")
	}
	sess.attempts = items
	var sb strings.Builder
	fmt.Fprintf(&sb, "Как решено · %s\n", stud.Name)
	for i, a := range items {
		when := a.StartedAt.Format("02.01 15:04")
		fmt.Fprintf(&sb, "\n%d. %s — %d/%d, %s, %s",
			i+1, testTitle(a.Title), a.Correct, a.Total, humanMS(a.TimeMS), when)
	}
	sb.WriteString("\n\nРазбор попытки: /review <номер>")
	return b.text(chatID, sb.String())
}

func (b *Bot) teacherReview(ctx context.Context, sess *session, chatID int64, arg string) Reply {
	if _, stop := b.requireActive(sess, chatID); stop != nil {
		return *stop
	}
	n, err := strconv.Atoi(strings.TrimSpace(arg))
	if err != nil || n < 1 || n > len(sess.attempts) {
		if len(sess.attempts) == 0 {
			return b.text(chatID, "Сначала запроси решения: /attempts")
		}
		return b.text(chatID, "Укажи номер попытки: /review 1")
	}
	att := sess.attempts[n-1]
	items, err := b.api.AttemptReview(ctx, sess.token, att.ID)
	if err != nil {
		return b.text(chatID, "Не удалось получить разбор.")
	}
	if len(items) == 0 {
		return b.text(chatID, "В этой попытке нет ответов.")
	}
	var sb strings.Builder
	fmt.Fprintf(&sb, "Разбор · %s\n", testTitle(att.Title))
	for _, it := range items {
		if it.IsCorrect {
			fmt.Fprintf(&sb, "\n№%d ✅ %s", it.Number, it.RawAnswer)
		} else {
			fmt.Fprintf(&sb, "\n№%d ❌ ответ: %s · верно: %s",
				it.Number, it.RawAnswer, strings.Join(it.Correct, " / "))
		}
	}
	return b.text(chatID, sb.String())
}

// --- helpers ---

// text builds a plain-text reply. It is escaped so stray <, >, & from data
// (names, answers) don't break Telegram's HTML parser.
func (b *Bot) text(chatID int64, s string) Reply {
	return Reply{ChatID: chatID, HTML: escapeHTML(s)}
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

func statusRU(s string) string {
	switch s {
	case "done":
		return "выполнено"
	case "missed":
		return "пропущено"
	default:
		return "запланировано"
	}
}

// humanMS renders a duration in ms as "Xм Yс" (or "Yс").
func humanMS(ms int64) string {
	sec := ms / 1000
	if sec < 60 {
		return fmt.Sprintf("%dс", sec)
	}
	return fmt.Sprintf("%dм %02dс", sec/60, sec%60)
}

// testTitle nicely renders the internal practice sentinel (mirrors the web).
func testTitle(t string) string {
	if t == "__practice__" {
		return "Свободное решение"
	}
	return t
}

func welcomeFor(role string) string {
	if role == "teacher" {
		return welcomeTeacher
	}
	return welcomeStudent
}

const needLinkMsg = `Привет! Чтобы начать, привяжи свой аккаунт:
1. Открой сайт и войди под своим логином.
2. В меню слева нажми «Привязать Telegram».
3. Пришли сюда код: /link <код> (или просто открой ссылку из сайта).`

const welcomeStudent = `Готов тренироваться! 💪
Команды:
  /solve рус — начать решать русский (мат / инф / общ)
  /next — следующее задание
Пришли ответ сообщением, когда решишь.`

const welcomeTeacher = `Режим учителя. Что можно посмотреть:
  /students — список учеников (потом /student <номер>)
  /stats [предмет] — статистика и прогноз
  /assigned — что назначено
  /attempts — как решено (потом /review <номер>)`
