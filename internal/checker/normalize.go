package checker

import (
	"math/big"
	"strings"
	"unicode"

	"egeism/internal/domain"
)

// minusReplacer maps the unicode dash/minus variants that show up in copied
// exam text onto an ASCII hyphen-minus so numbers parse.
var minusReplacer = strings.NewReplacer(
	"−", "-", // − MINUS SIGN
	"–", "-", // – EN DASH
	"—", "-", // — EM DASH
	"‐", "-", // ‐ HYPHEN
	"‑", "-", // ‑ NON-BREAKING HYPHEN
)

// normalizeNumber applies the §7 number normalization and parses the result as
// an exact rational. Both the student answer and each correct form go through
// this identically. big.Rat (not float64) is used so tolerance-boundary
// comparisons like |0.501-0.5| <= 0.001 are exact, per the plan's insistence on
// decimal parsing. As a bonus, big.Rat.SetString also accepts fractions ("1/2")
// and scientific notation.
func normalizeNumber(s string) (*big.Rat, bool) {
	s = strings.TrimSpace(s)
	s = minusReplacer.Replace(s)
	s = strings.ReplaceAll(s, ",", ".")
	s = stripSpaces(s)
	if s == "" {
		return nil, false
	}
	r, ok := new(big.Rat).SetString(s)
	if !ok {
		return nil, false
	}
	return r, true
}

// stripSpaces removes every unicode space rune (incl. NBSP U+00A0).
func stripSpaces(s string) string {
	return strings.Map(func(r rune) rune {
		if unicode.IsSpace(r) {
			return -1
		}
		return r
	}, s)
}

// normalizeString applies the §7 string normalization: trim, collapse internal
// whitespace, optional case-insensitivity and ё→е folding.
func normalizeString(s string, ci, yoFold bool) string {
	// strings.Fields both trims and collapses internal whitespace runs.
	s = strings.Join(strings.Fields(s), " ")
	if ci {
		s = strings.ToLower(s)
	}
	if yoFold {
		s = foldYo(s)
	}
	return s
}

// foldYo maps ё→е / Ё→Е on both cases so folding is order-independent w.r.t.
// case-lowering.
func foldYo(s string) string {
	return strings.NewReplacer("ё", "е", "Ё", "Е").Replace(s)
}

// tokenize splits an answer into comparable tokens per the token mode (§7).
func tokenize(s string, mode domain.TokenMode) []string {
	switch mode {
	case domain.TokenSplit:
		// Cut on separators, keep multi-character tokens; drop empties.
		return strings.FieldsFunc(s, isSeparator)
	default: // TokenChar: drop separators, each remaining letter/digit is a token.
		var toks []string
		for _, r := range s {
			if unicode.IsLetter(r) || unicode.IsDigit(r) {
				toks = append(toks, string(r))
			}
		}
		return toks
	}
}

// isSeparator reports whether r delimits tokens in split mode.
func isSeparator(r rune) bool {
	if unicode.IsSpace(r) {
		return true
	}
	switch r {
	case ',', ';', '.', '|', '/', ':':
		return true
	}
	return false
}

// correctTokens flattens every correct form into one token list under mode.
// Each element is tokenized and concatenated, so both ["2","4","5"] and ["245"]
// yield the same canonical answer (§7 note: correct for set/sequence is the
// token list of the single correct answer, not a list of alternatives).
func correctTokens(correct []string, mode domain.TokenMode) []string {
	var out []string
	for _, c := range correct {
		out = append(out, tokenize(c, mode)...)
	}
	return out
}
