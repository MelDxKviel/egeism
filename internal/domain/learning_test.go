package domain

import "testing"

func readyNumbers(last int) []RecentNumber {
	out := []RecentNumber{}
	for n := 1; n <= last; n++ {
		out = append(out, RecentNumber{Number: n, Total: 5, Correct: 4})
	}
	return out
}

func TestTrainingCeiling(t *testing.T) {
	for _, tc := range []struct {
		name string
		rows []RecentNumber
		want int
	}{
		{"new student", nil, 3},
		{"all wrong", []RecentNumber{{Number: 1, Total: 10}}, 3},
		{"one lucky answer", []RecentNumber{{Number: 1, Total: 1, Correct: 1}}, 3},
		{"two prerequisites insufficient", readyNumbers(2), 3},
		{"first stage ready", readyNumbers(3), 6},
		{"middle stage ready", readyNumbers(6), 9},
		{"part one incomplete", readyNumbers(11), 12},
		{"part two unlocked", readyNumbers(12), 15},
		{"advanced stage", readyNumbers(15), 18},
		{"hard task success does not skip basics", []RecentNumber{{Number: 19, Total: 10, Correct: 10}}, 3},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := TrainingCeiling(tc.rows, 19); got != tc.want {
				t.Fatalf("got %d, want %d", got, tc.want)
			}
		})
	}
	rows := readyNumbers(12)
	rows[0].Total, rows[0].Correct = 10, 7
	if got := TrainingCeiling(rows, 19); got != 3 {
		t.Fatalf("regression must close hard stages, got %d", got)
	}
	rows[0].Correct = 8
	if got := TrainingCeiling(rows, 19); got != 15 {
		t.Fatalf("recent recovery should reopen stages, got %d", got)
	}
	if got := TrainingCeiling(readyNumbers(19), 19); got != 19 {
		t.Fatalf("exam boundary: %d", got)
	}
}

func TestMathTaskRules(t *testing.T) {
	points := []int{2, 3, 2, 2, 3, 4, 4}
	for i, want := range points {
		part, mode, max := TaskRules(SubjectMath, i+13)
		if part != 2 || mode != "manual" || max != want {
			t.Fatalf("number %d: %d %s %d", i+13, part, mode, max)
		}
	}
	for _, subject := range []SubjectCode{SubjectMath, SubjectRus, SubjectInf, SubjectSoc} {
		part, mode, max := TaskRules(subject, 1)
		if part != 1 || mode != "auto" || max != 1 {
			t.Fatal("short-answer rules changed")
		}
	}
	if part, mode, _ := TaskRules(SubjectRus, 13); part != 1 || mode != "auto" {
		t.Fatal("only math is enabled")
	}
	if err := (AnswerSchema{Type: AnswerWritten}).Validate(); err != nil {
		t.Fatal(err)
	}
	if err := (AnswerSchema{Type: AnswerNumber}).Validate(); err == nil {
		t.Fatal("automatic answers still require a key")
	}
}
