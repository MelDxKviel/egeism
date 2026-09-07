package domain

import "github.com/google/uuid"

type SolutionPhoto struct {
	ID          uuid.UUID `json:"id"`
	AttemptID   uuid.UUID `json:"attempt_id"`
	TaskID      uuid.UUID `json:"task_id"`
	ObjectKey   string    `json:"-"`
	ContentType string    `json:"content_type"`
	SizeBytes   int64     `json:"size_bytes"`
}
type AnswerGrade struct {
	AnswerID uuid.UUID `json:"answer_id"`
	Points   *int32    `json:"points"`
	Comment  string    `json:"comment"`
}

// TaskRules is the subject-specific exam policy. Storage and review are generic;
// enabling another subject only requires defining its written-task rules here.
func TaskRules(subject SubjectCode, number int) (part int, gradingMode string, maxPoints int) {
	if subject == SubjectMath && number >= 13 && number <= 19 {
		points := 2
		if number == 14 || number == 17 {
			points = 3
		}
		if number >= 18 {
			points = 4
		}
		return 2, "manual", points
	}
	return 1, "auto", 1
}

type RecentNumber struct {
	Number  int
	Total   int64
	Correct int64
}

// TrainingCeiling opens three numbers at a time. Each prerequisite needs at
// least five graded answers and 80% accuracy over its latest ten answers.
// Recent performance lets a struggling student recover without being trapped
// by their lifetime mistakes, and closes harder stages after a regression.
func TrainingCeiling(stats []RecentNumber, lastNumber int) int {
	byNumber := map[int]RecentNumber{}
	for _, row := range stats {
		byNumber[row.Number] = row
	}
	ceiling := 3
	for ceiling < lastNumber {
		ready := true
		for n := 1; n <= ceiling; n++ {
			s := byNumber[n]
			if s.Total < 5 || s.Correct*5 < s.Total*4 {
				ready = false
				break
			}
		}
		if !ready {
			break
		}
		ceiling += 3
	}
	if ceiling > lastNumber {
		ceiling = lastNumber
	}
	return ceiling
}
