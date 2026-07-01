package ingest

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
)

// FileSource ingests a JSON array of RawTask from disk. This is the cleanest
// "no hands + has answers" path from §9 (a curated dataset), and doubles as a
// seed source for local dev. Real scraper/LLM sources implement the same Source
// interface.
type FileSource struct {
	provider string
	path     string
}

// NewFileSource builds a file-backed source. provider labels the origin in
// tasks.source for dedup/provenance.
func NewFileSource(provider, path string) *FileSource {
	return &FileSource{provider: provider, path: path}
}

// Name implements Source.
func (f *FileSource) Name() string { return f.provider }

// Fetch reads and decodes the file.
func (f *FileSource) Fetch(_ context.Context) ([]RawTask, error) {
	data, err := os.ReadFile(f.path)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", f.path, err)
	}
	var raws []RawTask
	if err := json.Unmarshal(data, &raws); err != nil {
		return nil, fmt.Errorf("decode %s: %w", f.path, err)
	}
	return raws, nil
}
