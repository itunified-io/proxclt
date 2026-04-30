package trace

import (
	"encoding/json"
	"testing"
)

func TestRecord_RoundTrip(t *testing.T) {
	rec := Record{
		SchemaVersion: CurrentSchemaVersion,
		TS:            1234567890,
		StepID:        "/lab-up step 5",
		Tool:          "Bash",
		Input:         json.RawMessage(`{"command":"qm create 2701"}`),
		Output:        json.RawMessage(`{"exit":0,"stdout":"VM 2701 created"}`),
		Decision:      DecisionExecuted,
		DurationMs:    1234,
	}
	h, err := rec.ComputeHash()
	if err != nil {
		t.Fatalf("ComputeHash: %v", err)
	}
	rec.Hash = h

	out, err := json.Marshal(rec)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}

	var got Record
	if err := json.Unmarshal(out, &got); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if got.StepID != rec.StepID {
		t.Errorf("step_id roundtrip: got %q want %q", got.StepID, rec.StepID)
	}
	if got.Tool != rec.Tool {
		t.Errorf("tool roundtrip: got %q want %q", got.Tool, rec.Tool)
	}
	if got.Decision != rec.Decision {
		t.Errorf("decision roundtrip: got %q want %q", got.Decision, rec.Decision)
	}
	if got.Hash != rec.Hash {
		t.Errorf("hash roundtrip: got %q want %q", got.Hash, rec.Hash)
	}
}

func TestRecord_Validate(t *testing.T) {
	rec := Record{
		SchemaVersion: CurrentSchemaVersion,
		TS:            1,
		StepID:        "test",
		Tool:          "Bash",
		Input:         json.RawMessage(`{}`),
		Decision:      DecisionExecuted,
	}
	h, _ := rec.ComputeHash()
	rec.Hash = h

	if err := rec.Validate(); err != nil {
		t.Errorf("valid record failed validation: %v", err)
	}

	// Tamper with output — should fail validation
	rec.Output = json.RawMessage(`{"tampered":true}`)
	if err := rec.Validate(); err != ErrHashMismatch {
		t.Errorf("expected ErrHashMismatch on tamper, got %v", err)
	}
}

func TestRecord_UnknownSchema(t *testing.T) {
	rec := Record{
		SchemaVersion: 999,
		Tool:          "Bash",
		Input:         json.RawMessage(`{}`),
		Decision:      DecisionExecuted,
	}
	rec.Hash, _ = rec.ComputeHash()
	if err := rec.Validate(); err != ErrUnknownSchema {
		t.Errorf("expected ErrUnknownSchema, got %v", err)
	}
}

func TestVerifyChain_Valid(t *testing.T) {
	recs := []Record{
		makeRec(t, "step1", "", 1),
		makeRec(t, "step2", "", 2),
		makeRec(t, "step3", "", 3),
	}
	// Chain them
	for i := 1; i < len(recs); i++ {
		recs[i].PrevHash = recs[i-1].Hash
		recs[i].Hash, _ = recs[i].ComputeHash()
	}
	idx, err := VerifyChain(recs)
	if err != nil {
		t.Errorf("valid chain rejected at index %d: %v", idx, err)
	}
	if idx != -1 {
		t.Errorf("valid chain returned non-(-1) index: %d", idx)
	}
}

func TestVerifyChain_Broken(t *testing.T) {
	recs := []Record{
		makeRec(t, "step1", "", 1),
		makeRec(t, "step2", "", 2),
	}
	recs[1].PrevHash = "deadbeef" // wrong prev_hash
	recs[1].Hash, _ = recs[1].ComputeHash()
	idx, err := VerifyChain(recs)
	if err != ErrChainBroken {
		t.Errorf("expected ErrChainBroken, got %v", err)
	}
	if idx != 1 {
		t.Errorf("expected break at index 1, got %d", idx)
	}
}

func TestVerifyChain_GenesisHasNoPrevHash(t *testing.T) {
	// First record with non-empty prev_hash should fail
	rec := makeRec(t, "step1", "shouldbeempty", 1)
	idx, err := VerifyChain([]Record{rec})
	if err != ErrChainBroken {
		t.Errorf("expected ErrChainBroken, got %v", err)
	}
	if idx != 0 {
		t.Errorf("expected break at index 0, got %d", idx)
	}
}

// makeRec builds a Record with the given step_id, prev_hash, and a unique TS.
// Hash is computed.
func makeRec(t *testing.T, stepID, prevHash string, ts int64) Record {
	t.Helper()
	r := Record{
		SchemaVersion: CurrentSchemaVersion,
		TS:            ts,
		StepID:        stepID,
		Tool:          "Bash",
		Input:         json.RawMessage(`{}`),
		Decision:      DecisionExecuted,
		PrevHash:      prevHash,
	}
	h, err := r.ComputeHash()
	if err != nil {
		t.Fatalf("ComputeHash: %v", err)
	}
	r.Hash = h
	return r
}
