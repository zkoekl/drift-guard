package drift

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"strings"
)

// Event is one JSONL record. Fields follow docs/METRICS.md.
// TokensJudge is always 0 in this prototype.
type Event struct {
	TS                     string   `json:"ts"`
	Kind                   string   `json:"kind"`
	SessionID              string   `json:"session_id,omitempty"`
	Source                 string   `json:"source,omitempty"`
	Activation             string   `json:"activation,omitempty"`
	Channel                string   `json:"channel,omitempty"`
	Mode                   string   `json:"mode,omitempty"`
	Violations             int      `json:"violations,omitempty"`
	RuleIDs                []string `json:"rule_ids,omitempty"`
	ElapsedMS              int64    `json:"elapsed_ms,omitempty"`
	TimeToDetectMS         *int64   `json:"time_to_detect_ms"`
	TokensJudge            int      `json:"tokens_judge"`
	TokensRulesInjected    int      `json:"tokens_rules_injected"`
	TokensFollowupEstimate int      `json:"tokens_followup_estimate"`
	Reason                 string   `json:"reason,omitempty"`
	Disputed               bool     `json:"disputed,omitempty"`
	DisputedIDs            []string `json:"disputed_ids,omitempty"`
	FalseAlarm             *bool    `json:"false_alarm,omitempty"`
	FoundBy                string   `json:"found_by,omitempty"`
	Precision              *float64 `json:"precision,omitempty"`
	Recall                 *float64 `json:"recall,omitempty"`
	TP                     *int     `json:"tp,omitempty"`
	FP                     *int     `json:"fp,omitempty"`
	FN                     *int     `json:"fn,omitempty"`
	InScope                string   `json:"in_scope,omitempty"`
	LinesAdded             int      `json:"lines_added,omitempty"`
	LinesRemoved           int      `json:"lines_removed,omitempty"`
	ProposalID             string   `json:"proposal_id,omitempty"`
}

// AppendJSONL writes one JSON object and flushes. It does not rewrite earlier lines.
func AppendJSONL(path string, v any) error {
	b, err := json.Marshal(v)
	if err != nil {
		return err
	}
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	if _, err := f.Write(append(b, '\n')); err != nil {
		return err
	}
	return f.Sync()
}

// LoadEvents reads JSONL. A SQLite file is rejected; this prototype reads JSONL only.
func LoadEvents(path string) ([]Event, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	if bytes.HasPrefix(b, []byte("SQLite format 3")) {
		return nil, errors.New("this prototype reads JSONL only")
	}
	var out []Event
	sc := bufio.NewScanner(bytes.NewReader(b))
	sc.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	lineNo := 0
	for sc.Scan() {
		lineNo++
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		var ev Event
		if err := json.Unmarshal([]byte(line), &ev); err != nil {
			return nil, err
		}
		out = append(out, ev)
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}
	return out, nil
}
