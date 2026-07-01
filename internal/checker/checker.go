// Package checker is the answer-comparison engine (§7). It is the heart of the
// product: a naive `answer == correct` produces false "wrong" verdicts and the
// student quits within a day. Comparison is driven by the task's AnswerSchema,
// which describes HOW to compare, not just the literal correct string.
//
// The engine is pure and dependency-free (domain types only) so its table test
// suite is the project's safety net.
package checker

import (
	"math/big"

	"egeism/internal/domain"
)

// Check reports whether raw is a correct answer under the schema. Unknown
// schema types and unparseable answers return false rather than erroring: the
// caller wants a verdict, and a bad schema is caught by AnswerSchema.Validate
// at ingest time.
func Check(schema domain.AnswerSchema, raw string) bool {
	switch schema.Type {
	case domain.AnswerNumber:
		return checkNumber(schema, raw)
	case domain.AnswerString:
		return checkString(schema, raw)
	case domain.AnswerSet:
		return checkSet(schema, raw)
	case domain.AnswerSequence:
		return checkSequence(schema, raw)
	default:
		return false
	}
}

func checkNumber(s domain.AnswerSchema, raw string) bool {
	got, ok := normalizeNumber(raw)
	if !ok {
		return false
	}
	tol := new(big.Rat).SetFloat64(s.Tolerance)
	if tol == nil { // tolerance was NaN/Inf; treat as exact
		tol = new(big.Rat)
	}
	for _, c := range s.Correct {
		want, ok := normalizeNumber(c)
		if !ok {
			continue
		}
		diff := new(big.Rat).Sub(got, want)
		diff.Abs(diff)
		if diff.Cmp(tol) <= 0 {
			return true
		}
	}
	return false
}

func checkString(s domain.AnswerSchema, raw string) bool {
	got := normalizeString(raw, s.CI, s.YoFold)
	for _, c := range s.Correct {
		if normalizeString(c, s.CI, s.YoFold) == got {
			return true
		}
	}
	return false
}

func checkSet(s domain.AnswerSchema, raw string) bool {
	mode := s.TokenOrDefault()
	got := toSet(tokenize(raw, mode))
	want := toSet(correctTokens(s.Correct, mode))
	if len(got) != len(want) {
		return false
	}
	for tok := range want {
		if !got[tok] {
			return false
		}
	}
	return true
}

func checkSequence(s domain.AnswerSchema, raw string) bool {
	mode := s.TokenOrDefault()
	got := tokenize(raw, mode)
	want := correctTokens(s.Correct, mode)
	if len(got) != len(want) {
		return false
	}
	for i := range want {
		if got[i] != want[i] {
			return false
		}
	}
	return true
}

// toSet dedups tokens into a set (order-insensitive comparison, §7).
func toSet(tokens []string) map[string]bool {
	set := make(map[string]bool, len(tokens))
	for _, t := range tokens {
		set[t] = true
	}
	return set
}
