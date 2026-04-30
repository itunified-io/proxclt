package trace

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
)

// Reader reads Records from a JSONL trace file in order.
type Reader struct {
	file *os.File
	scan *bufio.Scanner
}

// NewReader opens path for reading.
func NewReader(path string) (*Reader, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("trace reader: open: %w", err)
	}
	s := bufio.NewScanner(f)
	// Default Scanner buffer is 64KB; some Records (large MCP outputs)
	// may exceed that. Bump to 4MB.
	s.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	return &Reader{file: f, scan: s}, nil
}

// Next returns the next Record, or io.EOF at the end.
func (r *Reader) Next() (*Record, error) {
	for r.scan.Scan() {
		line := r.scan.Bytes()
		if len(line) == 0 {
			continue
		}
		var rec Record
		if err := json.Unmarshal(line, &rec); err != nil {
			return nil, fmt.Errorf("trace reader: unmarshal: %w", err)
		}
		return &rec, nil
	}
	if err := r.scan.Err(); err != nil {
		return nil, err
	}
	return nil, io.EOF
}

// All reads every Record into a slice.
func (r *Reader) All() ([]Record, error) {
	var out []Record
	for {
		rec, err := r.Next()
		if err == io.EOF {
			return out, nil
		}
		if err != nil {
			return out, err
		}
		out = append(out, *rec)
	}
}

// Close releases the underlying file.
func (r *Reader) Close() error {
	return r.file.Close()
}

// LoadAll is a convenience: open, read all, verify chain, close.
// Returns the records and an error if the chain is broken.
func LoadAll(path string) ([]Record, error) {
	r, err := NewReader(path)
	if err != nil {
		return nil, err
	}
	defer r.Close()
	recs, err := r.All()
	if err != nil {
		return recs, err
	}
	if idx, cerr := VerifyChain(recs); cerr != nil {
		return recs, fmt.Errorf("trace reader: %w (at record %d)", cerr, idx)
	}
	return recs, nil
}
