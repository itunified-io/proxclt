package trace

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"
)

// mockExec returns a fixed output per (Tool, Input) tuple.
type mockExec struct {
	outputs map[string]json.RawMessage
	errs    map[string]error
}

func (m *mockExec) Execute(_ context.Context, rec Record) (json.RawMessage, error) {
	key := rec.Tool + "|" + string(rec.Input)
	if e, ok := m.errs[key]; ok {
		return nil, e
	}
	if o, ok := m.outputs[key]; ok {
		return o, nil
	}
	return rec.Output, nil // default: matches recorded
}

func TestWriter_RoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "trace.jsonl")

	w, err := NewWriter(path)
	if err != nil {
		t.Fatalf("NewWriter: %v", err)
	}

	for i := 0; i < 3; i++ {
		rec := Record{
			StepID:   "step" + string(rune('1'+i)),
			Tool:     "Bash",
			Input:    json.RawMessage(`{"command":"true"}`),
			Output:   json.RawMessage(`{"exit":0}`),
			Decision: DecisionExecuted,
		}
		if err := w.Write(rec); err != nil {
			t.Fatalf("Write %d: %v", i, err)
		}
	}
	if err := w.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	recs, err := LoadAll(path)
	if err != nil {
		t.Fatalf("LoadAll: %v", err)
	}
	if len(recs) != 3 {
		t.Fatalf("want 3 records, got %d", len(recs))
	}
	if recs[0].PrevHash != "" {
		t.Errorf("genesis prev_hash should be empty, got %q", recs[0].PrevHash)
	}
	if recs[1].PrevHash != recs[0].Hash || recs[2].PrevHash != recs[1].Hash {
		t.Errorf("chain not linked: %q %q %q", recs[0].Hash, recs[1].PrevHash, recs[2].PrevHash)
	}
}

func TestWriter_ResumesChain(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "trace.jsonl")

	w1, _ := NewWriter(path)
	_ = w1.Write(Record{StepID: "a", Tool: "Bash", Input: json.RawMessage(`{}`), Decision: DecisionExecuted})
	w1.Close()

	// Reopen and append
	w2, err := NewWriter(path)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	if err := w2.Write(Record{StepID: "b", Tool: "Bash", Input: json.RawMessage(`{}`), Decision: DecisionExecuted}); err != nil {
		t.Fatalf("append: %v", err)
	}
	w2.Close()

	recs, err := LoadAll(path)
	if err != nil {
		t.Fatalf("LoadAll: %v", err)
	}
	if len(recs) != 2 {
		t.Fatalf("want 2 records after resume, got %d", len(recs))
	}
	if recs[1].PrevHash != recs[0].Hash {
		t.Errorf("chain not resumed: prev=%q want=%q", recs[1].PrevHash, recs[0].Hash)
	}
}

func TestReplay_Strict_AllMatch(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "trace.jsonl")
	w, _ := NewWriter(path)
	_ = w.Write(Record{StepID: "s1", Tool: "Bash", Input: json.RawMessage(`{"cmd":"a"}`), Output: json.RawMessage(`{"exit":0}`), Decision: DecisionExecuted})
	_ = w.Write(Record{StepID: "s2", Tool: "Bash", Input: json.RawMessage(`{"cmd":"b"}`), Output: json.RawMessage(`{"exit":0}`), Decision: DecisionExecuted})
	w.Close()

	recs, _ := LoadAll(path)
	exec := &mockExec{outputs: map[string]json.RawMessage{}}
	var buf bytes.Buffer
	results, err := Replay(context.Background(), recs, exec, ModeStrict, &buf)
	if err != nil {
		t.Fatalf("Replay: %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("want 2 results, got %d", len(results))
	}
	for i, r := range results {
		if !r.Match {
			t.Errorf("result[%d] should match: %+v", i, r)
		}
	}
}

func TestReplay_Strict_DivergenceFails(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "trace.jsonl")
	w, _ := NewWriter(path)
	_ = w.Write(Record{StepID: "s1", Tool: "Bash", Input: json.RawMessage(`{"cmd":"a"}`), Output: json.RawMessage(`{"exit":0}`), Decision: DecisionExecuted})
	w.Close()

	recs, _ := LoadAll(path)
	exec := &mockExec{outputs: map[string]json.RawMessage{
		"Bash|" + `{"cmd":"a"}`: json.RawMessage(`{"exit":1}`), // diverges
	}}
	_, err := Replay(context.Background(), recs, exec, ModeStrict, io.Discard)
	if err == nil {
		t.Fatal("expected divergence error in ModeStrict")
	}
}

func TestReplay_Observe_LogsButReturnsNil(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "trace.jsonl")
	w, _ := NewWriter(path)
	_ = w.Write(Record{StepID: "s1", Tool: "Bash", Input: json.RawMessage(`{"cmd":"a"}`), Output: json.RawMessage(`{"exit":0}`), Decision: DecisionExecuted})
	w.Close()

	recs, _ := LoadAll(path)
	exec := &mockExec{outputs: map[string]json.RawMessage{
		"Bash|" + `{"cmd":"a"}`: json.RawMessage(`{"exit":1}`),
	}}
	var buf bytes.Buffer
	results, err := Replay(context.Background(), recs, exec, ModeObserve, &buf)
	if err != nil {
		t.Fatalf("ModeObserve should not error on divergence: %v", err)
	}
	if !results[0].Diverged {
		t.Error("result should be marked diverged")
	}
	if !bytes.Contains(buf.Bytes(), []byte("DIFF")) {
		t.Error("log should mention DIFF")
	}
}

func TestReplay_DryRun_SkipsExecution(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "trace.jsonl")
	w, _ := NewWriter(path)
	_ = w.Write(Record{StepID: "s1", Tool: "Bash", Input: json.RawMessage(`{}`), Output: json.RawMessage(`{}`), Decision: DecisionExecuted})
	w.Close()

	recs, _ := LoadAll(path)
	exec := &mockExec{errs: map[string]error{
		"Bash|" + `{}`: errors.New("should not be called"),
	}}
	_, err := Replay(context.Background(), recs, exec, ModeDryRun, io.Discard)
	if err != nil {
		t.Errorf("ModeDryRun should not call executor: %v", err)
	}
}

func TestReplay_SkipsNonExecutedDecisions(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "trace.jsonl")
	w, _ := NewWriter(path)
	_ = w.Write(Record{StepID: "s1", Tool: "Bash", Input: json.RawMessage(`{}`), Decision: DecisionDenied, DenyRule: "GLB-001"})
	w.Close()

	recs, _ := LoadAll(path)
	calls := 0
	exec := execFunc(func(_ context.Context, _ Record) (json.RawMessage, error) {
		calls++
		return nil, nil
	})
	_, err := Replay(context.Background(), recs, exec, ModeStrict, io.Discard)
	if err != nil {
		t.Errorf("denied records should not be re-executed: %v", err)
	}
	if calls != 0 {
		t.Errorf("expected 0 exec calls, got %d", calls)
	}
}

// execFunc adapts a function to the Executor interface.
type execFunc func(context.Context, Record) (json.RawMessage, error)

func (f execFunc) Execute(ctx context.Context, rec Record) (json.RawMessage, error) {
	return f(ctx, rec)
}

// Sanity: trace files written by Writer are valid JSONL and can be
// concatenated with other valid traces (concatenation is invalid as a
// chain, but each line individually is a parseable Record).
func TestTraceFile_IsJSONL(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "trace.jsonl")
	w, _ := NewWriter(path)
	_ = w.Write(Record{StepID: "x", Tool: "Bash", Input: json.RawMessage(`{}`), Decision: DecisionExecuted})
	w.Close()

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	lines := bytes.Split(bytes.TrimRight(data, "\n"), []byte("\n"))
	for _, ln := range lines {
		var rec Record
		if err := json.Unmarshal(ln, &rec); err != nil {
			t.Errorf("line not valid JSON: %s", ln)
		}
	}
}
