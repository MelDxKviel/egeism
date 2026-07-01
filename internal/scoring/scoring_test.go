package scoring

import (
	"testing"

	"egeism/internal/domain"
)

func TestPredict(t *testing.T) {
	cases := []struct {
		name        string
		subject     domain.SubjectCode
		accuracy    float64
		wantPrimary int
		wantTest    int
		wantAcc     float64
	}{
		{"math half", domain.SubjectMath, 0.5, 6, 35, 0.5},
		{"math full", domain.SubjectMath, 1.0, 12, 70, 1.0},
		{"rus zero", domain.SubjectRus, 0.0, 0, 0, 0.0},
		{"clamp above 1", domain.SubjectInf, 1.5, 27, 70, 1.0},
		{"clamp below 0", domain.SubjectSoc, -0.2, 0, 0, 0.0},
		{"unknown subject uses default max", domain.SubjectCode("xxx"), 1.0, 30, 70, 1.0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := Predict(tc.subject, tc.accuracy)
			if f.PrimaryEstimate != tc.wantPrimary {
				t.Errorf("primary = %d, want %d", f.PrimaryEstimate, tc.wantPrimary)
			}
			if f.TestScore != tc.wantTest {
				t.Errorf("test score = %d, want %d", f.TestScore, tc.wantTest)
			}
			if f.Accuracy != tc.wantAcc {
				t.Errorf("accuracy = %v, want %v", f.Accuracy, tc.wantAcc)
			}
			if f.Note == "" {
				t.Error("note should carry the honesty caveat")
			}
		})
	}
}
