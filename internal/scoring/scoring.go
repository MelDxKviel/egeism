// Package scoring turns part-1 solve accuracy into a forecast score (§11 M5).
//
// IMPORTANT: this is a documented placeholder. A faithful forecast needs the
// official ФИПИ primary→test conversion tables (they change yearly and differ
// per subject) and, ideally, part-2 modelling — part 1 alone is a fraction of
// the exam. The shape here is final; only the numbers must be replaced with the
// real tables before this is shown as a promise to a student.
package scoring

import "egeism/internal/domain"

// Forecast is a score prediction for one subject.
type Forecast struct {
	Subject          domain.SubjectCode `json:"subject"`
	Accuracy         float64            `json:"accuracy"`           // part-1 solve ratio [0,1]
	PrimaryEstimate  int                `json:"primary_estimate"`   // estimated primary points (part 1)
	PrimaryMax       int                `json:"primary_max"`        // max primary points modelled
	TestScore        int                `json:"test_score"`         // estimated 0..100 test score
	Note             string             `json:"note"`               // honesty caveat for the UI
}

// primaryMaxPart1 approximates the max primary points obtainable from part 1 by
// subject. Placeholder values — replace with the current official spec.
var primaryMaxPart1 = map[domain.SubjectCode]int{
	domain.SubjectRus:  33,
	domain.SubjectMath: 12,
	domain.SubjectInf:  27,
	domain.SubjectSoc:  22,
}

const placeholderNote = "Оценка приблизительная: построена только по части 1 и на временной шкале перевода. Заменить на официальные таблицы ФИПИ."

// Predict maps a subject's part-1 accuracy to a forecast. The primary estimate
// is accuracy × part-1 max; the test score is a linear placeholder scaled so
// full part-1 mastery lands near a realistic part-1-only ceiling.
func Predict(subject domain.SubjectCode, accuracy float64) Forecast {
	if accuracy < 0 {
		accuracy = 0
	}
	if accuracy > 1 {
		accuracy = 1
	}
	maxPrimary, ok := primaryMaxPart1[subject]
	if !ok {
		maxPrimary = 30
	}
	primary := int(accuracy*float64(maxPrimary) + 0.5)

	// Placeholder conversion: part 1 alone realistically caps a test score
	// around ~70, so scale accuracy into [0,70]. Real tables replace this.
	const part1Ceiling = 70
	test := int(accuracy*float64(part1Ceiling) + 0.5)

	return Forecast{
		Subject:         subject,
		Accuracy:        accuracy,
		PrimaryEstimate: primary,
		PrimaryMax:      maxPrimary,
		TestScore:       test,
		Note:            placeholderNote,
	}
}
