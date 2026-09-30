package api

import (
	"testing"

	"egeism/internal/domain"
)

func TestBankRefillIncludesMissingNumbers(t *testing.T) {
	if !needsBankRefill(nil, domain.SubjectMath) {
		t.Fatal("cold bank must be filled")
	}
	counts := []domain.NumberAvailability{}
	for n := 1; n <= 19; n++ {
		counts = append(counts, domain.NumberAvailability{Number: n, Active: 10})
	}
	if needsBankRefill(counts, domain.SubjectMath) {
		t.Fatal("stocked bank should not contact source")
	}
	if !needsBankRefill(counts[:18], domain.SubjectMath) {
		t.Fatal("absent number needs replenishment")
	}
	counts[0].Active = 9
	if !needsBankRefill(counts, domain.SubjectMath) {
		t.Fatal("low stock needs replenishment")
	}
}
