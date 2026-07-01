package ingest

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

// DatasetSource ingests a "ready dataset with answers" — the chosen §9 path.
// It reads our normalized RawTask records from a local file OR an http(s) URL,
// in either a JSON array or JSONL (one object per line, the common dataset
// shape). It is source-agnostic on purpose: whatever bank you settle on
// (converted from an ЕГЭ dataset with answers), you emit this normalized shape
// and only this one package ever changes.
type DatasetSource struct {
	provider string
	location string // file path or http(s) URL
}

// NewDatasetSource builds a dataset source from a file path or URL.
func NewDatasetSource(provider, location string) *DatasetSource {
	return &DatasetSource{provider: provider, location: location}
}

// Name implements Source.
func (d *DatasetSource) Name() string { return d.provider }

// Fetch loads and decodes the dataset (JSON array or JSONL, local or remote).
func (d *DatasetSource) Fetch(ctx context.Context) ([]RawTask, error) {
	data, err := d.read(ctx)
	if err != nil {
		return nil, err
	}
	raws, err := DecodeRawTasks(data)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", d.location, err)
	}
	return raws, nil
}

// DecodeRawTasks parses a normalized task blob: a JSON array or JSONL (one
// object per line). Shared by the dataset source and the teacher upload endpoint.
func DecodeRawTasks(data []byte) ([]RawTask, error) {
	trimmed := bytes.TrimSpace(data)
	if len(trimmed) == 0 {
		return nil, fmt.Errorf("empty input")
	}
	if trimmed[0] == '[' {
		var raws []RawTask
		if err := json.Unmarshal(trimmed, &raws); err != nil {
			return nil, fmt.Errorf("decode json array: %w", err)
		}
		return raws, nil
	}
	return decodeJSONL(trimmed)
}

func (d *DatasetSource) read(ctx context.Context) ([]byte, error) {
	if strings.HasPrefix(d.location, "http://") || strings.HasPrefix(d.location, "https://") {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, d.location, nil)
		if err != nil {
			return nil, err
		}
		client := &http.Client{Timeout: 60 * time.Second}
		resp, err := client.Do(req)
		if err != nil {
			return nil, fmt.Errorf("fetch %s: %w", d.location, err)
		}
		defer resp.Body.Close()
		if resp.StatusCode >= 300 {
			return nil, fmt.Errorf("fetch %s: status %d", d.location, resp.StatusCode)
		}
		return io.ReadAll(resp.Body)
	}
	return os.ReadFile(d.location)
}

// decodeJSONL parses one RawTask per non-empty line.
func decodeJSONL(data []byte) ([]RawTask, error) {
	var raws []RawTask
	sc := bufio.NewScanner(bytes.NewReader(data))
	sc.Buffer(make([]byte, 0, 1024*1024), 8*1024*1024) // allow long lines
	line := 0
	for sc.Scan() {
		line++
		b := bytes.TrimSpace(sc.Bytes())
		if len(b) == 0 {
			continue
		}
		var rt RawTask
		if err := json.Unmarshal(b, &rt); err != nil {
			return nil, fmt.Errorf("jsonl line %d: %w", line, err)
		}
		raws = append(raws, rt)
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}
	return raws, nil
}
