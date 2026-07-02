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

// InCallback is a normalized inline-button press (callback_query). Buttons on
// worker-sent notifications land here too — one bot identity, one long-poll.
type InCallback struct {
	TelegramID int64
	ChatID     int64
	Data       string
	FirstName  string
}

// Button is one inline-keyboard button: Text plus either a URL to open or
// callback Data the bot handles.
type Button struct {
	Text string
	URL  string
	Data string
}

// Reply is an outbound message. RichHTML (when set) is the preferred rendering:
// a Bot API 10.1 Rich Message with real tables/headings/inline images, with
// RichMedia the attachments left over after inlining. HTML + Media are the
// classic fallback (parse_mode=HTML text, then photos/files) used when the rich
// send is rejected. Buttons ride on whichever message goes out.
type Reply struct {
	ChatID    int64
	RichHTML  string
	RichMedia []MediaRef
	HTML      string
	Buttons   [][]Button
	Media     []MediaRef
}

// Solve modes: free practice pulls from the practice pool; an assigned test
// serves exactly the variant's tasks and finishes the attempt (→ assignment done).
const (
	modePractice = "practice"
	modeTest     = "test"
)

// session is a user's in-flight state. In-memory is fine for one student + one
// teacher; move to Redis when this must survive restarts or scale out.
type session struct {
	token string // session JWT from /auth/telegram (empty until resolved/linked)
	user  User   // resolved account (role decides the command set)

	// student solving state
	mode         string // "", modePractice or modeTest
	subject      string // practice: subject code
	testTitle    string // test: variant title for headers/results
	assignmentID string // test: assignment being solved ("" for plain tests)
	attemptID    string
	queue        []TaskView // remaining tasks this run
	current      *TaskView  // task awaiting an answer, nil if none
	askedAt      time.Time
	total        int // test: variant size (for "3/15" progress)
	answered     int // test: submitted count
	correct      int // test: correct count

	// teacher browsing state
	students []User           // last listed students (index → student)
	active   *User            // selected student
	attempts []AttemptSummary // last listed attempts (index → attempt, for /review)
}

// Bot holds the API client and per-user session state.
type Bot struct {
	api       *APIClient
	mediaBase string // public web origin for inlining figures in rich messages ("" = don't inline)
	mu        sync.Mutex
	sessions  map[int64]*session
}

// New builds a Bot over an API client. mediaBase is the PUBLIC web origin
// (WEB_URL) used to inline task figures into rich messages; leave empty (or
// local) to send figures as separate photos instead.
func New(api *APIClient, mediaBase string) *Bot {
	return &Bot{api: api, mediaBase: mediaBase, sessions: make(map[int64]*session)}
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

var subjectEmoji = map[string]string{
	"rus": "📕", "math": "📗", "inf": "📘", "soc": "📙",
}

var subjectOrder = []string{"rus", "math", "inf", "soc"}

// subjectRows is the standard subject-picker keyboard (2×2).
func subjectRows() [][]Button {
	return [][]Button{
		{{Text: "📕 Русский", Data: "solve:rus"}, {Text: "📗 Математика", Data: "solve:math"}},
		{{Text: "📘 Информатика", Data: "solve:inf"}, {Text: "📙 Обществознание", Data: "solve:soc"}},
	}
}

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

// HandleCallback processes an inline-button press. Data grammar:
//
//	solve:<code>   start practice for a subject
//	next           next task (practice or test)
//	tests          student's assigned tests
//	assign:<id>    start solving an assigned test
//	finish         finish the current test early → results
//	t:<cmd>        teacher shortcuts (students/assigned/attempts/stats)
func (b *Bot) HandleCallback(ctx context.Context, in InCallback) Reply {
	sess := b.session(in.TelegramID)
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
	data := strings.TrimSpace(in.Data)

	if sess.user.Role == "teacher" {
		msg := InMessage{TelegramID: in.TelegramID, ChatID: in.ChatID, FirstName: in.FirstName}
		switch {
		case data == "t:students":
			return b.listStudents(ctx, sess, in.ChatID)
		case data == "t:assigned":
			return b.teacherAssigned(ctx, sess, in.ChatID)
		case data == "t:attempts":
			return b.teacherAttempts(ctx, sess, in.ChatID)
		case data == "t:stats":
			return b.teacherStats(ctx, sess, in.ChatID, "")
		default:
			return b.handleTeacher(ctx, sess, msg, "/help")
		}
	}

	switch {
	case strings.HasPrefix(data, "solve:"):
		code, ok := subjectAliases[strings.TrimPrefix(data, "solve:")]
		if !ok {
			return b.text(in.ChatID, "Не понял предмет — выбери кнопкой ниже.")
		}
		return b.startSubject(ctx, sess, in.ChatID, code)
	case data == "next":
		if sess.mode == "" {
			return b.welcomeStudentReply(in.ChatID)
		}
		return b.serveTask(ctx, sess, in.ChatID)
	case data == "tests":
		return b.listAssigned(ctx, sess, in.ChatID)
	case strings.HasPrefix(data, "assign:"):
		return b.startAssigned(ctx, sess, in.ChatID, strings.TrimPrefix(data, "assign:"))
	case data == "finish":
		if sess.mode != modeTest {
			return b.welcomeStudentReply(in.ChatID)
		}
		return b.finishTest(ctx, sess, in.ChatID)
	default:
		return b.welcomeStudentReply(in.ChatID)
	}
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
	greet := "🔗 <b>Аккаунт привязан:</b> " + escapeHTML(user.Name) + "\n\n"
	if user.Role == "teacher" {
		r := b.welcomeTeacherReply(in.ChatID)
		r.HTML = greet + r.HTML
		return r
	}
	r := b.welcomeStudentReply(in.ChatID)
	r.HTML = greet + r.HTML
	return r
}

// --- student ---

func (b *Bot) handleStudent(ctx context.Context, sess *session, in InMessage, text string) Reply {
	lower := strings.ToLower(text)
	switch {
	case text == "/start" || text == "/help":
		return b.welcomeStudentReply(in.ChatID)

	case strings.HasPrefix(text, "/solve"), strings.HasPrefix(text, "/reshat"):
		arg := strings.TrimSpace(strings.TrimPrefix(strings.TrimPrefix(text, "/solve"), "/reshat"))
		code, ok := subjectAliases[strings.ToLower(arg)]
		if !ok {
			return b.html(in.ChatID, "Какой предмет тренируем?", subjectRows())
		}
		return b.startSubject(ctx, sess, in.ChatID, code)

	case text == "/tests" || lower == "тесты" || lower == "мои тесты":
		return b.listAssigned(ctx, sess, in.ChatID)

	case text == "/next":
		if sess.mode == "" {
			return b.html(in.ChatID, "Сначала выбери, что решаем:", append(subjectRows(), []Button{{Text: "🎯 Мои тесты", Data: "tests"}}))
		}
		return b.serveTask(ctx, sess, in.ChatID)

	case text == "/finish" || lower == "завершить":
		if sess.mode != modeTest {
			return b.welcomeStudentReply(in.ChatID)
		}
		return b.finishTest(ctx, sess, in.ChatID)

	default:
		if code, ok := subjectAliases[strings.ToLower(text)]; ok {
			return b.startSubject(ctx, sess, in.ChatID, code)
		}
		if sess.current == nil {
			return b.html(in.ChatID, "Отправь ответ на задание — или начнём заново:",
				append(subjectRows(), []Button{{Text: "🎯 Мои тесты", Data: "tests"}}))
		}
		return b.submit(ctx, sess, in.ChatID, text)
	}
}

func (b *Bot) startSubject(ctx context.Context, sess *session, chatID int64, code string) Reply {
	attemptID, err := b.api.StartPractice(ctx, sess.token, code)
	if err != nil {
		return b.text(chatID, "Не удалось начать тренировку. Попробуй позже.")
	}
	sess.mode = modePractice
	sess.subject = code
	sess.testTitle, sess.assignmentID = "", ""
	sess.attemptID = attemptID
	sess.queue = nil
	sess.current = nil
	sess.total, sess.answered, sess.correct = 0, 0, 0
	return b.serveTask(ctx, sess, chatID)
}

// listAssigned shows the student their assigned tests with a ▶️ button per
// unsolved one — the same list the dashboard's "Назначено тебе" card shows.
func (b *Bot) listAssigned(ctx context.Context, sess *session, chatID int64) Reply {
	cards, err := b.api.StudentAssignments(ctx, sess.token, sess.user.ID)
	if err != nil {
		return b.text(chatID, "Не удалось получить назначения. Попробуй позже.")
	}
	if len(cards) == 0 {
		return b.html(chatID, "🎯 <b>Твои тесты</b>\n\nПока ничего не назначено. Учитель запланирует тест — он появится здесь.\nА пока можно тренироваться:", subjectRows())
	}
	var sb strings.Builder
	sb.WriteString("🎯 <b>Твои тесты</b>\n")
	var rows [][]Button
	for _, c := range cards {
		icon := "⏳"
		switch c.Status {
		case "done":
			icon = "✅"
		case "missed":
			icon = "⌛"
		}
		fmt.Fprintf(&sb, "\n%s <b>%s</b> · %d %s · на %s — %s",
			icon, escapeHTML(testTitle(c.Title)), c.TaskCount, pluralTasks(c.TaskCount),
			c.ScheduledAt.In(time.Local).Format("02.01 15:04"), statusRU(c.Status))
		if c.Status != "done" && len(rows) < 6 {
			rows = append(rows, []Button{{Text: "▶️ " + testTitle(c.Title), Data: "assign:" + c.ID}})
		}
	}
	if len(rows) == 0 {
		sb.WriteString("\n\nВсё решено — красота! 🎉 Потренируем что-нибудь?")
		rows = subjectRows()
	}
	return b.html(chatID, sb.String(), rows)
}

// startAssigned begins solving an assigned test: exactly the variant's tasks,
// with the attempt tied to the assignment (finish → assignment done).
func (b *Bot) startAssigned(ctx context.Context, sess *session, chatID int64, assignmentID string) Reply {
	cards, err := b.api.StudentAssignments(ctx, sess.token, sess.user.ID)
	if err != nil {
		return b.text(chatID, "Не удалось получить назначение. Попробуй позже.")
	}
	var card *AssignmentCard
	for i := range cards {
		if cards[i].ID == assignmentID {
			card = &cards[i]
			break
		}
	}
	if card == nil {
		return b.html(chatID, "Не нашёл это назначение — глянь список:", [][]Button{{{Text: "🎯 Мои тесты", Data: "tests"}}})
	}
	tasks, err := b.api.TestTasks(ctx, sess.token, card.TestID)
	if err != nil || len(tasks) == 0 {
		return b.text(chatID, "В этом тесте пока нет заданий.")
	}
	attemptID, err := b.api.StartAttempt(ctx, sess.token, card.TestID, card.ID)
	if err != nil {
		return b.text(chatID, "Не удалось начать тест. Попробуй позже.")
	}
	sess.mode = modeTest
	sess.subject = ""
	sess.testTitle = testTitle(card.Title)
	sess.assignmentID = card.ID
	sess.attemptID = attemptID
	sess.queue = tasks
	sess.current = nil
	sess.total, sess.answered, sess.correct = len(tasks), 0, 0

	head := fmt.Sprintf("🚀 <b>Поехали: «%s»</b>\n%d %s — не торопись, время на задание учитывается.",
		escapeHTML(sess.testTitle), sess.total, pluralTasks(int64(sess.total)))
	first := b.serveTask(ctx, sess, chatID)
	first.HTML = head + "\n\n" + first.HTML
	return first
}

func (b *Bot) serveTask(ctx context.Context, sess *session, chatID int64) Reply {
	if len(sess.queue) == 0 {
		if sess.mode == modeTest {
			return b.finishTest(ctx, sess, chatID)
		}
		tasks, err := b.api.PracticeTasks(ctx, sess.token, sess.subject, 15)
		if err != nil {
			return b.text(chatID, "Не удалось получить задания.")
		}
		sess.queue = tasks
	}
	if len(sess.queue) == 0 {
		return b.html(chatID, "🎉 По этому предмету всё решено (или банк пуст). Выбери другой:", subjectRows())
	}
	task := sess.queue[0]
	sess.queue = sess.queue[1:]
	sess.current = &task
	sess.askedAt = time.Now()
	return b.taskReply(sess, chatID, task)
}

// finishTest closes the attempt (the assignment flips to done server-side) and
// shows the results card.
func (b *Bot) finishTest(ctx context.Context, sess *session, chatID int64) Reply {
	if sess.attemptID != "" {
		if err := b.api.FinishAttempt(ctx, sess.token, sess.attemptID); err != nil {
			return b.text(chatID, "Не удалось завершить тест. Попробуй ещё раз: /finish")
		}
	}
	title, total, answered, correct := sess.testTitle, sess.total, sess.answered, sess.correct
	sess.mode, sess.testTitle, sess.assignmentID = "", "", ""
	sess.attemptID, sess.queue, sess.current = "", nil, nil
	sess.total, sess.answered, sess.correct = 0, 0, 0

	var sb strings.Builder
	fmt.Fprintf(&sb, "🏁 <b>Тест «%s» завершён!</b>\n", escapeHTML(title))
	fmt.Fprintf(&sb, "\n✅ Верно: <b>%d из %d</b>", correct, total)
	if skipped := total - answered; skipped > 0 {
		fmt.Fprintf(&sb, " · пропущено: %d", skipped)
	}
	switch {
	case total > 0 && correct == total:
		sb.WriteString("\n\nИдеально! 🔥 Так держать!")
	case total > 0 && correct*2 >= total:
		sb.WriteString("\n\nХорошо идёшь! Разбор ошибок — на сайте. 💪")
	default:
		sb.WriteString("\n\nНичего страшного — разбери ошибки и попробуй ещё раз. 💪")
	}
	return b.html(chatID, sb.String(), [][]Button{
		{{Text: "🎯 Мои тесты", Data: "tests"}},
		{{Text: "📚 Тренироваться", Data: "next"}},
	})
}

func (b *Bot) submit(ctx context.Context, sess *session, chatID int64, raw string) Reply {
	elapsed := time.Since(sess.askedAt).Milliseconds()
	res, err := b.api.SubmitAnswer(ctx, sess.token, sess.attemptID, sess.current.ID, raw, elapsed)
	if err != nil {
		return b.text(chatID, "Не удалось проверить ответ. Попробуй ещё раз.")
	}
	sess.current = nil // task consumed
	sess.answered++
	if res.IsCorrect {
		sess.correct++
	}

	var sb strings.Builder
	if res.IsCorrect {
		sb.WriteString("✅ <b>Верно!</b>")
	} else {
		sb.WriteString("❌ <b>Пока неверно</b>")
		if len(res.Solution) > 0 {
			fmt.Fprintf(&sb, "\n💡 Правильный ответ: <code>%s</code>", escapeHTML(strings.Join(res.Solution, " / ")))
		}
	}
	if sess.mode == modeTest {
		fmt.Fprintf(&sb, "\n\n📊 %d/%d · верно %d", sess.answered, sess.total, sess.correct)
		if len(sess.queue) == 0 {
			return b.html(chatID, sb.String(), [][]Button{{{Text: "🏁 Итоги", Data: "next"}}})
		}
		return b.html(chatID, sb.String(), [][]Button{
			{{Text: "➡️ Дальше", Data: "next"}, {Text: "🏁 Завершить", Data: "finish"}},
		})
	}
	return b.html(chatID, sb.String(), [][]Button{{{Text: "➡️ Дальше", Data: "next"}}})
}

// taskReply builds the message for a task, in two renderings:
//   - Rich (preferred): real <table> grids, an <h3> header, paragraphs, and —
//     when the web origin is public — figures inlined as <img>.
//   - Classic (fallback): parse_mode=HTML text with aligned <pre> tables, then
//     figures/files as separate messages.
func (b *Bot) taskReply(sess *session, chatID int64, t TaskView) Reply {
	var head, richHead string
	if sess.mode == modeTest {
		head = fmt.Sprintf("🎯 <b>%s</b> · %d/%d\n<b>Задание №%d</b>",
			escapeHTML(sess.testTitle), sess.answered+1, sess.total, t.Number)
		richHead = fmt.Sprintf("<h3>🎯 %s · %d/%d</h3><p><b>Задание №%d</b></p>",
			escapeHTML(sess.testTitle), sess.answered+1, sess.total, t.Number)
	} else {
		head = fmt.Sprintf("%s <b>%s · №%d</b>",
			subjectEmoji[sess.subject], subjectTitles[sess.subject], t.Number)
		richHead = fmt.Sprintf("<h3>%s %s · №%d</h3>",
			subjectEmoji[sess.subject], subjectTitles[sess.subject], t.Number)
	}

	html := head
	if body := statementToHTML(t.Statement, t.Media); strings.TrimSpace(body) != "" {
		html += "\n\n" + body
	}
	html += "\n\n<i>✍️ Отправь ответ сообщением.</i>"

	richBody, leftovers := statementToRichHTML(t.Statement, t.Media, b.mediaBase)
	rich := richHead + richBody + "<p><i>✍️ Отправь ответ сообщением.</i></p>"

	return Reply{
		ChatID:    chatID,
		RichHTML:  rich,
		RichMedia: leftovers,
		HTML:      html,
		Media:     attachments(t.Media),
	}
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
		return b.welcomeTeacherReply(in.ChatID)

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
		return b.welcomeTeacherReply(in.ChatID)
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
			c.Title, c.ScheduledAt.In(time.Local).Format("02.01 15:04"), c.TaskCount, statusRU(c.Status))
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
		when := a.StartedAt.In(time.Local).Format("02.01 15:04")
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

// html builds a styled reply: the caller passes ready Telegram-HTML (data
// interpolations escaped via escapeHTML) plus an optional inline keyboard.
func (b *Bot) html(chatID int64, html string, rows [][]Button) Reply {
	return Reply{ChatID: chatID, HTML: html, Buttons: rows}
}

// welcomeStudentReply is the student home card: subject picker + assigned tests.
func (b *Bot) welcomeStudentReply(chatID int64) Reply {
	return b.html(chatID, welcomeStudent, append(subjectRows(), []Button{{Text: "🎯 Мои тесты", Data: "tests"}}))
}

// welcomeTeacherReply is the teacher home card with shortcut buttons.
func (b *Bot) welcomeTeacherReply(chatID int64) Reply {
	return b.html(chatID, welcomeTeacher, [][]Button{
		{{Text: "👥 Ученики", Data: "t:students"}, {Text: "📊 Статистика", Data: "t:stats"}},
		{{Text: "🎯 Назначено", Data: "t:assigned"}, {Text: "📝 Решения", Data: "t:attempts"}},
	})
}

// pluralTasks declines «задание» for a count (1 задание, 2 задания, 15 заданий).
func pluralTasks(n int64) string {
	n = n % 100
	if n >= 11 && n <= 14 {
		return "заданий"
	}
	switch n % 10 {
	case 1:
		return "задание"
	case 2, 3, 4:
		return "задания"
	default:
		return "заданий"
	}
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

const needLinkMsg = `Привет! Чтобы начать, привяжи свой аккаунт:
1. Открой сайт и войди под своим логином.
2. В меню слева нажми «Привязать Telegram».
3. Пришли сюда код: /link <код> (или просто открой ссылку из сайта).`

// Welcome cards are Telegram-HTML (sent via b.html with keyboards attached).
const welcomeStudent = `🎓 <b>Готов тренироваться!</b>

▫️ Жми на предмет — начнём разминку
▫️ «Мои тесты» — что назначил учитель
▫️ Ответ на задание присылай сообщением

<i>Команды: /solve рус · /tests · /next · /finish</i>`

const welcomeTeacher = `👩‍🏫 <b>Режим учителя</b>

▫️ Ученики: /students, выбрать — /student N
▫️ Статистика и прогноз: /stats [предмет]
▫️ Что назначено: /assigned
▫️ Как решено: /attempts, разбор — /review N`
