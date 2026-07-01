package domain

import (
	"encoding/json"
	"fmt"
)

// AnswerType discriminates AnswerSchema. It is the JSONB "type" field.
type AnswerType string

const (
	// AnswerNumber is a single numeric value compared with a tolerance.
	AnswerNumber AnswerType = "number"
	// AnswerString is a word/phrase compared with optional case- and yo-folding.
	AnswerString AnswerType = "string"
	// AnswerSet is an unordered multiset of tokens ("указите номера").
	AnswerSet AnswerType = "set"
	// AnswerSequence is an ordered list of tokens (order matters).
	AnswerSequence AnswerType = "sequence"
)

// Valid reports whether t is one of the known answer types.
func (t AnswerType) Valid() bool {
	switch t {
	case AnswerNumber, AnswerString, AnswerSet, AnswerSequence:
		return true
	default:
		return false
	}
}

// TokenMode controls how set/sequence answers are tokenized.
type TokenMode string

const (
	// TokenChar strips separators and treats each character as a token:
	// "245" == "2,4,5" == "2 4 5".
	TokenChar TokenMode = "char"
	// TokenSplit cuts on separators, for multi-character tokens.
	TokenSplit TokenMode = "split"
)

// AnswerSchema is the JSONB payload stored in tasks.answer_schema. It does not
// hold "the right answer string" but a description of HOW to compare, so the
// checker never reports a correct answer as wrong on formatting alone (§7).
//
// It is a flat discriminated union: only the fields relevant to Type are used.
type AnswerSchema struct {
	Type    AnswerType `json:"type"`
	Correct []string   `json:"correct"`

	// number
	Tolerance float64 `json:"tolerance,omitempty"`

	// string
	CI     bool `json:"ci,omitempty"`      // case-insensitive
	YoFold bool `json:"yo_fold,omitempty"` // fold ё -> е on both sides

	// set / sequence
	Token TokenMode `json:"token,omitempty"`
}

// Validate checks the schema is internally consistent. Ingest and admin editing
// must reject schemas that fail this so bad data never reaches the checker.
func (s AnswerSchema) Validate() error {
	if !s.Type.Valid() {
		return fmt.Errorf("answer schema: unknown type %q", s.Type)
	}
	if len(s.Correct) == 0 {
		return fmt.Errorf("answer schema: correct must be non-empty")
	}
	switch s.Type {
	case AnswerNumber:
		if s.Tolerance < 0 {
			return fmt.Errorf("answer schema: tolerance must be >= 0")
		}
	case AnswerSet, AnswerSequence:
		if s.Token == "" {
			// default is char; not an error, normalized in checker.
			return nil
		}
		if s.Token != TokenChar && s.Token != TokenSplit {
			return fmt.Errorf("answer schema: unknown token mode %q", s.Token)
		}
	}
	return nil
}

// TokenOrDefault returns the token mode, defaulting to TokenChar.
func (s AnswerSchema) TokenOrDefault() TokenMode {
	if s.Token == "" {
		return TokenChar
	}
	return s.Token
}

// ParseAnswerSchema decodes a JSONB blob into an AnswerSchema and validates it.
func ParseAnswerSchema(raw []byte) (AnswerSchema, error) {
	var s AnswerSchema
	if err := json.Unmarshal(raw, &s); err != nil {
		return AnswerSchema{}, fmt.Errorf("answer schema: %w", err)
	}
	if err := s.Validate(); err != nil {
		return AnswerSchema{}, err
	}
	return s, nil
}
