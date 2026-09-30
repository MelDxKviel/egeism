package api

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"net/url"
	"os"
	"testing"
	"time"

	"egeism/internal/domain"
	"egeism/internal/store"
	"egeism/migrations"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"
)

// Uses a new schema in an explicitly supplied test database. No production
// config is read. Run with EGEISM_TEST_DATABASE_URL=postgres://... go test ./internal/api.
func reviewTestServer(t *testing.T) *Server {
	t.Helper()
	dsn := os.Getenv("EGEISM_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("EGEISM_TEST_DATABASE_URL is not set")
	}
	ctx := context.Background()
	conn, err := pgx.Connect(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	schema := "review_test_" + uuid.NewString()[:8]
	quoted := pgx.Identifier{schema}.Sanitize()
	if _, err := conn.Exec(ctx, "CREATE SCHEMA "+quoted); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = conn.Exec(ctx, "DROP SCHEMA "+quoted+" CASCADE"); _ = conn.Close(ctx) })
	u, err := url.Parse(dsn)
	if err != nil {
		t.Fatal(err)
	}
	q := u.Query()
	q.Set("search_path", schema)
	u.RawQuery = q.Encode()
	db, err := sql.Open("pgx", u.String())
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	goose.SetBaseFS(migrations.FS)
	if err := goose.SetDialect("postgres"); err != nil {
		t.Fatal(err)
	}
	if err := goose.UpContext(ctx, db, "."); err != nil {
		t.Fatal(err)
	}
	st, err := store.New(ctx, u.String())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(st.Close)
	return NewServer(st, nil, "integration-test-secret", nil, "", "")
}

func TestWrittenReviewLifecycle(t *testing.T) {
	s := reviewTestServer(t)
	ctx := context.Background()
	user := func(role domain.Role, subject *domain.SubjectCode) domain.User {
		u, err := s.store.CreateUserWithCredentials(ctx, role, string(role), uuid.NewString(), "unused", subject)
		if err != nil {
			t.Fatal(err)
		}
		return u
	}
	math, rus := domain.SubjectMath, domain.SubjectRus
	student, other := user(domain.RoleStudent, nil), user(domain.RoleStudent, nil)
	teacher, outsider, wrongSubject := user(domain.RoleTeacher, &math), user(domain.RoleTeacher, &math), user(domain.RoleTeacher, &rus)
	for _, u := range []domain.User{teacher, wrongSubject} {
		if _, err := s.store.Pool().Exec(ctx, "INSERT INTO enrollments (teacher_id, student_id) VALUES ($1,$2)", u.ID, student.ID); err != nil {
			t.Fatal(err)
		}
	}
	sub, err := s.store.GetSubjectByCode(ctx, math)
	if err != nil {
		t.Fatal(err)
	}
	makeTask := func(n int) domain.Task {
		published, verified := time.Now().Add(-24*time.Hour), time.Now()
		// Provenance fixture only; these synthetic tasks never leave the isolated test schema.
		source := &domain.Source{Provider: "sdamgia", ExternID: uuid.NewString(), URL: "https://math-ege.sdamgia.ru/problem?id=1",
			PublishedAt: &published, VerifiedAt: &verified, DateEvidenceURL: "https://math-ege.sdamgia.ru/problem?id=1", DateEvidence: "Test-only date fixture"}
		task, err := s.store.CreateTask(ctx, domain.Task{SubjectID: sub.ID, Number: n, Statement: "Решите задание", AnswerSchema: domain.AnswerSchema{Type: domain.AnswerNumber, Correct: []string{"42"}}, Status: domain.TaskActive, Source: source})
		if err != nil {
			t.Fatal(err)
		}
		return task
	}
	short, written, foreign := makeTask(1), makeTask(14), makeTask(15)
	test, err := s.store.CreateTest(ctx, sub.ID, domain.TestComposed, "Проверка решения", teacher.ID)
	if err != nil {
		t.Fatal(err)
	}
	for i, task := range []domain.Task{short, written} {
		if _, err := s.store.AddTestItem(ctx, test.ID, task.ID, i+1); err != nil {
			t.Fatal(err)
		}
	}
	asg, err := s.store.CreateAssignment(ctx, test.ID, student.ID, teacher.ID, time.Now(), nil, true)
	if err != nil {
		t.Fatal(err)
	}
	att, err := s.store.StartAttempt(ctx, student.ID, test.ID, &asg.ID)
	if err != nil {
		t.Fatal(err)
	}
	request := func(u domain.User, method, path string, body any, want int) *httptest.ResponseRecorder {
		t.Helper()
		var data []byte
		if body != nil {
			data, _ = json.Marshal(body)
		}
		r := httptest.NewRequest(method, path, bytes.NewReader(data))
		token, err := s.issueToken(u)
		if err != nil {
			t.Fatal(err)
		}
		r.Header.Set("Authorization", "Bearer "+token)
		w := httptest.NewRecorder()
		s.Router().ServeHTTP(w, r)
		if w.Code != want {
			t.Fatalf("%s %s: got %d want %d: %s", method, path, w.Code, want, w.Body.String())
		}
		return w
	}
	base := "/api/attempts/" + att.ID.String()
	request(student, "POST", base+"/answers", map[string]any{"task_id": foreign.ID, "raw_answer": "42"}, 422)
	request(student, "POST", base+"/answers", map[string]any{"task_id": written.ID, "raw_answer": "42"}, 422)
	request(student, "POST", base+"/finish", nil, 422)
	request(other, "POST", base+"/answers", map[string]any{"task_id": short.ID, "raw_answer": "42"}, 403)
	request(student, "POST", base+"/answers", map[string]any{"task_id": short.ID, "raw_answer": "42"}, 201)
	request(student, "POST", base+"/answers", map[string]any{"task_id": short.ID, "raw_answer": "42"}, 422)
	photo, err := s.store.MutateSolutionPhoto(ctx, domain.SolutionPhoto{AttemptID: att.ID, TaskID: written.ID, ObjectKey: "solutions/" + uuid.NewString(), ContentType: "image/png", SizeBytes: 100}, false)
	if err != nil {
		t.Fatal(err)
	}
	for _, u := range []domain.User{other, outsider, wrongSubject} {
		request(u, "GET", "/api/solution-photos/"+photo.ID.String(), nil, 403)
	}
	request(student, "GET", "/api/solution-photos/"+photo.ID.String(), nil, 503) // authorized; test has no media backend
	resp := request(student, "POST", base+"/answers", map[string]any{"task_id": written.ID, "raw_answer": ""}, 201)
	if !bytes.Contains(resp.Body.Bytes(), []byte(`"review_status":"pending"`)) || bytes.Contains(resp.Body.Bytes(), []byte(`"solution"`)) {
		t.Fatal("written solution auto-graded or key leaked")
	}
	request(student, "DELETE", "/api/solution-photos/"+photo.ID.String(), nil, 422)
	request(student, "POST", base+"/finish", nil, 200)
	request(student, "POST", base+"/finish", nil, 200) // retry is idempotent
	request(student, "POST", base+"/answers", map[string]any{"task_id": written.ID, "raw_answer": "42"}, 422)
	answers, err := s.store.ListAnswersForAttempt(ctx, att.ID)
	if err != nil {
		t.Fatal(err)
	}
	var manual domain.Answer
	for _, a := range answers {
		if a.ReviewStatus == "pending" {
			manual = a
		}
	}
	if manual.ID == uuid.Nil || manual.MaxPoints != 3 {
		t.Fatalf("missing pending answer: %+v", answers)
	}
	grades := func(points int, comment string) any {
		return map[string]any{"grades": []map[string]any{{"answer_id": manual.ID, "points": points, "comment": comment}}}
	}
	for _, u := range []domain.User{student, other, outsider, wrongSubject} {
		request(u, "PUT", base+"/review", grades(2, "Проверка"), 403)
	}
	request(teacher, "PUT", base+"/review", grades(4, "Слишком много"), 422)
	before, err := s.store.RecentNumberPerformance(ctx, student.ID, sub.ID)
	if err != nil {
		t.Fatal(err)
	}
	for _, row := range before {
		if row.Number == 14 {
			t.Fatal("pending review counted as failure")
		}
	}
	request(teacher, "PUT", base+"/review", grades(2, "Не хватает обоснования"), 200)
	request(teacher, "PUT", base+"/review", grades(2, "Не хватает обоснования"), 200)
	feed, err := s.store.ListNotifications(ctx, student.ID, 30)
	if err != nil {
		t.Fatal(err)
	}
	if len(feed) != 1 || feed[0].AttemptID == nil || *feed[0].AttemptID != att.ID || feed[0].TestTitle != test.Title {
		t.Fatalf("notification duplicated or missing context: %+v", feed)
	}
	cards, err := s.store.ListAssignmentCards(ctx, student.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(cards) != 1 || cards[0].Points != 3 || cards[0].MaxPoints != 4 || cards[0].PendingReview != 0 {
		t.Fatalf("partial score: %+v", cards)
	}
	request(teacher, "PUT", base+"/review", grades(3, "Теперь всё верно"), 200)
	cards, err = s.store.ListAssignmentCards(ctx, student.ID)
	if err != nil {
		t.Fatal(err)
	}
	if cards[0].Points != 4 || cards[0].Correct != 2 {
		t.Fatalf("edited score not reflected: %+v", cards[0])
	}
	request(student, "GET", base+"/review", nil, 200)
	// Turning the requirement off permits a text-only written answer.
	optional, err := s.store.CreateAssignment(ctx, test.ID, student.ID, teacher.ID, time.Now(), nil, false)
	if err != nil {
		t.Fatal(err)
	}
	a2, err := s.store.StartAttempt(ctx, student.ID, test.ID, &optional.ID)
	if err != nil {
		t.Fatal(err)
	}
	request(student, "POST", fmt.Sprintf("/api/attempts/%s/answers", a2.ID), map[string]any{"task_id": written.ID, "raw_answer": "Моё решение"}, 201)
	request(student, "POST", fmt.Sprintf("/api/attempts/%s/finish", a2.ID), nil, 200)
	// Skipped short answers are recorded as zero; unfinished written answers
	// still get a review row so the teacher can grade the entire test.
	a3, err := s.store.StartAttempt(ctx, student.ID, test.ID, &optional.ID)
	if err != nil {
		t.Fatal(err)
	}
	request(student, "POST", fmt.Sprintf("/api/attempts/%s/finish", a3.ID), nil, 200)
	rows, err := s.store.ListAnswersForAttempt(ctx, a3.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 2 {
		t.Fatal("skipped tasks missing from review")
	}
	// Practice revisits short answers, but each written solution has one grade.
	practice, err := s.store.GetOrCreatePracticeTest(ctx, sub.ID, other.ID)
	if err != nil {
		t.Fatal(err)
	}
	patt, err := s.store.StartAttempt(ctx, other.ID, practice.ID, nil)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		if _, err := s.store.SubmitAnswer(ctx, patt.ID, short.ID, "41", 100); err != nil {
			t.Fatal(err)
		}
	}
	_, err = s.store.MutateSolutionPhoto(ctx, domain.SolutionPhoto{AttemptID: patt.ID, TaskID: foreign.ID, ObjectKey: "solutions/" + uuid.NewString(), ContentType: "image/png", SizeBytes: 100}, false)
	if err != nil {
		t.Fatal(err)
	}
	// Clearing an unsubmitted photo's bank must not delete its task or fail FK checks.
	if _, _, err := s.store.ClearBank(ctx, sub.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.store.FinishSubmission(ctx, patt.ID); err != nil {
		t.Fatal(err)
	}
	rows, err = s.store.ListAnswersForAttempt(ctx, patt.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 3 || rows[2].ReviewStatus != "pending" || rows[2].TaskID != foreign.ID {
		t.Fatalf("unsubmitted practice photo lost: %+v", rows)
	}
	resp = request(other, "GET", "/api/practice/recommended?subject=math", nil, 200)
	var plan recommendedResp
	if err := json.Unmarshal(resp.Body.Bytes(), &plan); err != nil {
		t.Fatal(err)
	}
	if plan.MaxNumber != 3 || len(plan.Tasks) == 0 {
		t.Fatalf("beginner plan: %+v", plan)
	}
	for _, task := range plan.Tasks {
		if task.Number > 3 || task.GradingMode == "manual" {
			t.Fatalf("advanced task given to beginner: %+v", task)
		}
	}
}
