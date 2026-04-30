package trace

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// Writer appends Records to a JSONL file, hash-chaining each entry.
//
// Concurrency: safe for one Writer per file. Multiple writers to the same
// file would race on the chain and produce a broken trace; callers are
// expected to maintain one writer per trace file (typical: per skill run).
type Writer struct {
	mu       sync.Mutex
	path     string
	file     *os.File
	buf      *bufio.Writer
	prevHash string
}

// NewWriter opens path for append. If the file does not exist, creates
// it with parent directories. If it exists, reads the last line to
// resume the hash chain.
func NewWriter(path string) (*Writer, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, fmt.Errorf("trace writer: mkdir: %w", err)
	}

	prev := ""
	if _, err := os.Stat(path); err == nil {
		// File exists — read last record to resume chain
		last, lerr := readLastRecord(path)
		if lerr == nil && last != nil {
			prev = last.Hash
		}
	}

	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return nil, fmt.Errorf("trace writer: open: %w", err)
	}
	return &Writer{
		path:     path,
		file:     f,
		buf:      bufio.NewWriter(f),
		prevHash: prev,
	}, nil
}

// Write appends a Record to the trace, computing PrevHash + Hash + TS
// (if zero) automatically. Caller's Hash field is overwritten.
func (w *Writer) Write(rec Record) error {
	w.mu.Lock()
	defer w.mu.Unlock()

	if rec.SchemaVersion == 0 {
		rec.SchemaVersion = CurrentSchemaVersion
	}
	if rec.TS == 0 {
		rec.TS = time.Now().UnixNano()
	}
	rec.PrevHash = w.prevHash

	h, err := rec.ComputeHash()
	if err != nil {
		return fmt.Errorf("trace writer: compute hash: %w", err)
	}
	rec.Hash = h

	b, err := json.Marshal(rec)
	if err != nil {
		return fmt.Errorf("trace writer: marshal: %w", err)
	}
	if _, err := w.buf.Write(b); err != nil {
		return fmt.Errorf("trace writer: write: %w", err)
	}
	if err := w.buf.WriteByte('\n'); err != nil {
		return fmt.Errorf("trace writer: write newline: %w", err)
	}
	w.prevHash = h
	return nil
}

// Sync flushes buffered writes to disk. Call after a logical batch or
// before reading the trace from another process.
func (w *Writer) Sync() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if err := w.buf.Flush(); err != nil {
		return err
	}
	return w.file.Sync()
}

// Close flushes + closes the underlying file.
func (w *Writer) Close() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if err := w.buf.Flush(); err != nil {
		_ = w.file.Close()
		return err
	}
	return w.file.Close()
}

// Path returns the trace file path.
func (w *Writer) Path() string {
	return w.path
}

// readLastRecord scans the file once to return the final non-empty line
// as a Record. Used by NewWriter to resume the chain. Linear cost in
// file size; acceptable for typical trace files (a few hundred KB).
func readLastRecord(path string) (*Record, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	if len(data) == 0 {
		return nil, nil
	}
	// Find last newline
	end := len(data)
	if data[end-1] == '\n' {
		end--
	}
	start := end
	for start > 0 && data[start-1] != '\n' {
		start--
	}
	if start == end {
		return nil, nil
	}
	var rec Record
	if err := json.Unmarshal(data[start:end], &rec); err != nil {
		return nil, err
	}
	return &rec, nil
}
