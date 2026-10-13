package approve

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
)

// LoadDecisions reads a decision record. Trailing data is refused so a
// half-written file cannot approve anything by accident.
func LoadDecisions(r io.Reader) (Decisions, error) {
	data, err := io.ReadAll(r)
	if err != nil {
		return Decisions{}, &DecisionsError{Reason: "cannot read the decision record", Cause: err}
	}
	if len(bytes.TrimSpace(data)) == 0 {
		return Decisions{}, &DecisionsError{Reason: "the decision record is empty"}
	}

	decoder := json.NewDecoder(bytes.NewReader(data))
	var decisions Decisions
	if err := decoder.Decode(&decisions); err != nil {
		return Decisions{}, &DecisionsError{Reason: "the decision record is not valid JSON", Cause: err}
	}

	var trailing json.RawMessage
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return Decisions{}, &DecisionsError{Reason: "the decision record has trailing data after the JSON value"}
	}
	return decisions, nil
}

// LoadDecisionsFile reads a decision record from a file.
func LoadDecisionsFile(path string) (Decisions, error) {
	file, err := os.Open(path)
	if err != nil {
		return Decisions{}, &DecisionsError{
			Reason: fmt.Sprintf("cannot read the decision record %s", path),
			Cause:  err,
		}
	}
	defer file.Close()

	decisions, err := LoadDecisions(file)
	if err != nil {
		return Decisions{}, &DecisionsError{
			Reason: fmt.Sprintf("invalid decision record %s", path),
			Cause:  err,
		}
	}
	return decisions, nil
}
