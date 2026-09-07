package api

import (
	"bytes"
	"context"
	"encoding/json"
	"image"
	"image/jpeg"
	"image/png"
	"net/http/httptest"
	"testing"

	"egeism/internal/domain"
)

func TestSolutionPhotoValidation(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 10, 10))
	for _, kind := range []string{"jpeg", "png"} {
		var buf bytes.Buffer
		if kind == "png" {
			_ = png.Encode(&buf, img)
		} else {
			_ = jpeg.Encode(&buf, img, nil)
		}
		ct, ok := validSolutionPhoto(buf.Bytes())
		if !ok || ct != "image/"+kind {
			t.Fatalf("valid %s rejected", kind)
		}
	}
	for _, data := range [][]byte{nil, []byte("<svg xmlns=\"http://www.w3.org/2000/svg\"><script>alert(1)</script></svg>"), []byte("not a jpeg"), {0xff, 0xd8, 0xff}} {
		if _, ok := validSolutionPhoto(data); ok {
			t.Fatal("invalid/photo-script accepted")
		}
	}
}

func TestStudentTaskViewIsSafeForWrittenAnswers(t *testing.T) {
	task := domain.Task{Part: 2, GradingMode: "manual", MaxPoints: 3, AnswerSchema: domain.AnswerSchema{Type: domain.AnswerWritten, Correct: []string{"SECRET_KEY"}}}
	b, err := json.Marshal(toTaskView(task))
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(b, []byte("SECRET_KEY")) || bytes.Contains(b, []byte("answer_schema")) {
		t.Fatal("student view leaked the key")
	}
	if toTaskView(task).BotSolvable {
		t.Fatal("written solution cannot be graded in chat")
	}
}

func TestStudentCannotReviewOrCurate(t *testing.T) {
	s := &Server{}
	for _, handler := range []func(*httptest.ResponseRecorder){
		func(w *httptest.ResponseRecorder) {
			r := httptest.NewRequest("PUT", "/", bytes.NewBufferString(`{"grades":[]}`))
			r = r.WithContext(context.WithValue(r.Context(), userKey, domain.User{Role: domain.RoleStudent}))
			s.handleSaveReview(w, r)
		},
		func(w *httptest.ResponseRecorder) {
			r := httptest.NewRequest("POST", "/", nil)
			r = r.WithContext(context.WithValue(r.Context(), userKey, domain.User{Role: domain.RoleStudent}))
			s.handleFetchTasks(w, r)
		},
	} {
		w := httptest.NewRecorder()
		handler(w)
		if w.Code != 403 {
			t.Fatalf("got %d instead of 403", w.Code)
		}
	}
}

func TestSmartSessionIncludesNewStage(t *testing.T) {
	var tasks []domain.Task
	for n := 1; n <= 15; n++ {
		for i := 0; i < 5; i++ {
			tasks = append(tasks, domain.Task{Number: n})
		}
	}
	plan := interleaveNumbers(tasks, 15)
	if len(plan) != 75 {
		t.Fatal("lost tasks")
	}
	if plan[0].Number != 13 || plan[1].Number != 14 || plan[2].Number != 15 {
		t.Fatal("newly unlocked tasks starve behind easier tasks")
	}
	for _, task := range interleaveNumbers(tasks, 3) {
		if task.Number > 3 {
			t.Fatal("hard task leaked into beginner plan")
		}
	}
}
