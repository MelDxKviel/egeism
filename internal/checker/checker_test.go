package checker

import (
	"testing"

	"egeism/internal/domain"
)

func num(tol float64, correct ...string) domain.AnswerSchema {
	return domain.AnswerSchema{Type: domain.AnswerNumber, Correct: correct, Tolerance: tol}
}

func str(ci, yo bool, correct ...string) domain.AnswerSchema {
	return domain.AnswerSchema{Type: domain.AnswerString, Correct: correct, CI: ci, YoFold: yo}
}

func set(tok domain.TokenMode, correct ...string) domain.AnswerSchema {
	return domain.AnswerSchema{Type: domain.AnswerSet, Correct: correct, Token: tok}
}

func seq(tok domain.TokenMode, correct ...string) domain.AnswerSchema {
	return domain.AnswerSchema{Type: domain.AnswerSequence, Correct: correct, Token: tok}
}

// TestChecker is the §7 safety net: every documented case plus the negatives.
func TestChecker(t *testing.T) {
	cases := []struct {
		name   string
		schema domain.AnswerSchema
		raw    string
		want   bool
	}{
		// --- number: comma vs dot ---
		{"comma==dot", num(0, "0,5"), "0.5", true},
		{"dot==dot", num(0, "0,5"), "0,5", true},
		{"number mismatch", num(0, "0,5"), "0.6", false},

		// --- number: unicode minus ---
		{"minus sign U+2212", num(0, "-3"), "−3", true},
		{"en dash U+2013", num(0, "-3"), "–3", true},
		{"em dash U+2014", num(0, "-3"), "—3", true},
		{"ascii minus", num(0, "-3"), "-3", true},
		{"wrong sign", num(0, "-3"), "3", false},

		// --- number: spaces stripped ---
		{"spaces in number", num(0, "1000"), "1 000", true},
		{"nbsp in number", num(0, "1000"), "1 000", true},

		// --- number: tolerance ---
		{"pi within tol", num(0.01, "3.14159"), "3.14", true},
		{"pi outside tol", num(0.0001, "3.14159"), "3.14", false},
		{"tol boundary inclusive", num(0.001, "0,5"), "0.501", true},
		{"tol just outside", num(0.001, "0,5"), "0.5011", false},

		// --- number: multiple accepted forms ---
		{"matches any correct", num(0, "0,5", "1/2", "0.50"), "0.50", true},

		// --- number: negatives ---
		{"empty number", num(0, "5"), "", false},
		{"garbage number", num(0, "5"), "abc", false},
		{"trailing junk number", num(0, "3.14"), "3.14x", false},

		// --- string: spaces / case with ci+yo ---
		{"trim+case+yo", str(true, true, "ещё"), " Ещё ", true},
		{"internal spaces collapsed", str(true, false, "красная площадь"), "красная   площадь", true},
		{"ci off is strict", str(false, false, "Ещё"), "ещё", false},

		// --- string: ё/е folding ---
		{"yo folds to e", str(true, true, "ещё"), "еще", true},
		{"yo both correct forms", str(true, true, "ещё", "еще"), "ёщё", true},
		{"no yo fold strict", str(true, false, "ещё"), "еще", false},

		// --- string: negatives ---
		{"empty string", str(true, true, "ответ"), "", false},
		{"wrong word", str(true, true, "ответ"), "вопрос", false},

		// --- set: char tokens, order/duplicates ignored ---
		{"set no separators", set(domain.TokenChar, "2", "4", "5"), "245", true},
		{"set with commas", set(domain.TokenChar, "2", "4", "5"), "2,4,5", true},
		{"set reordered", set(domain.TokenChar, "2", "4", "5"), "524", true},
		{"set with spaces", set(domain.TokenChar, "2", "4", "5"), "2 4 5", true},
		{"set dedup", set(domain.TokenChar, "2", "4", "5"), "22455", true},
		{"set correct as one string", set(domain.TokenChar, "245"), "5,4,2", true},
		{"set extra token", set(domain.TokenChar, "2", "4", "5"), "2456", false},
		{"set missing token", set(domain.TokenChar, "2", "4", "5"), "24", false},
		{"set empty", set(domain.TokenChar, "2", "4", "5"), "", false},

		// --- set: split mode, multi-char tokens ---
		{"set split multichar", set(domain.TokenSplit, "12", "34"), "34, 12", true},
		{"set split mismatch", set(domain.TokenSplit, "12", "34"), "1, 2, 3, 4", false},

		// --- sequence: order matters ---
		{"sequence in order", seq(domain.TokenChar, "1", "2", "3"), "123", true},
		{"sequence wrong order", seq(domain.TokenChar, "1", "2", "3"), "312", false},
		{"sequence correct as string", seq(domain.TokenChar, "123"), "123", true},
		{"sequence spaced input", seq(domain.TokenChar, "1", "2", "3"), "1 2 3", true},
		{"sequence length mismatch", seq(domain.TokenChar, "1", "2", "3"), "12", false},
		{"sequence empty", seq(domain.TokenChar, "1", "2", "3"), "", false},
		{"sequence split words", seq(domain.TokenSplit, "аб", "вг"), "аб вг", true},
		{"sequence split reordered", seq(domain.TokenSplit, "аб", "вг"), "вг аб", false},

		// --- unknown type ---
		{"unknown type", domain.AnswerSchema{Type: "bogus", Correct: []string{"1"}}, "1", false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := Check(tc.schema, tc.raw)
			if got != tc.want {
				t.Errorf("Check(%+v, %q) = %v, want %v", tc.schema, tc.raw, got, tc.want)
			}
		})
	}
}

func TestNormalizeNumber(t *testing.T) {
	cases := []struct {
		in   string
		want string // canonical big.Rat string, empty when ok=false
		ok   bool
	}{
		{"0,5", "1/2", true},
		{"1 000,25", "4001/4", true},
		{"−3,5", "-7/2", true},
		{"1/2", "1/2", true},
		{"", "", false},
		{"abc", "", false},
	}
	for _, tc := range cases {
		got, ok := normalizeNumber(tc.in)
		if ok != tc.ok {
			t.Errorf("normalizeNumber(%q) ok = %v, want %v", tc.in, ok, tc.ok)
			continue
		}
		if ok && got.RatString() != tc.want {
			t.Errorf("normalizeNumber(%q) = %s, want %s", tc.in, got.RatString(), tc.want)
		}
	}
}
