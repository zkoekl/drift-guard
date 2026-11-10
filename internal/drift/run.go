package drift

import (
	"encoding/json"
	"os"
	"strings"
	"time"

	"github.com/zkoekl/drift-guard/internal/rules"
)

// FileInput is a synthetic case or transcript. The expected object, when present
// in the file, is ignored here.
type FileInput struct {
	CaseID           string          `json:"case_id"`
	Origin           string          `json:"origin"`
	SessionID        string          `json:"session_id"`
	AssistantText    string          `json:"assistant_text"`
	ReplyLanguage    string          `json:"reply_language"`
	TestCommand      string          `json:"test_command"`
	TestPathGlobs    []string        `json:"test_path_globs"`
	ChangedPaths     []string        `json:"changed_paths"`
	Commands         []rules.Command `json:"commands"`
	Messages         []Message       `json:"messages"`
	Reads            []string        `json:"reads"`
	Anchors          []string        `json:"anchors"`
	AnchorMinShared  int             `json:"anchor_min_shared"`
	PushbackPhrases  []string        `json:"pushback_phrases"`
	AgreementPhrases []string        `json:"agreement_phrases"`
	Markers          *struct {
		Goal       []string `json:"goal"`
		Source     []string `json:"source"`
		Proof      []string `json:"proof"`
		Assumption []string `json:"assumption"`
	} `json:"markers"`
	CanaryFiles  []string `json:"canary_files,omitempty"`
	CanarySecret string   `json:"canary_secret,omitempty"`
}

// Result is the machine-readable summary printed by check and scan.
type Result struct {
	RuleIDs     []string `json:"rule_ids"`
	DisputedIDs []string `json:"disputed_ids,omitempty"`
	Mode        string   `json:"mode"`
	Exit        int      `json:"exit"`
}

// CheckFile evaluates the two checks. A block finding exits 1. Log findings exit 0.
func CheckFile(path, logPath string, now time.Time) (Result, error) {
	in, err := loadFileInput(path)
	if err != nil {
		return Result{Exit: 2}, err
	}
	findings := Evaluate(toInput(in))
	if err := writeVerdict(logPath, now, sidOf(in), "cli", findings); err != nil {
		return Result{Exit: 2}, err
	}
	res := Result{RuleIDs: IDs(findings), Mode: AggregateMode(findings), Exit: 0}
	if Block(findings) {
		res.Exit = 1
	}
	return res, nil
}

// ScanFile evaluates a saved transcript. The process exit is always 0.
// Block-shaped findings are still recorded with their mode. Flip is log-only.
func ScanFile(path, logPath string, now time.Time) (Result, error) {
	in, err := loadFileInput(path)
	if err != nil {
		return Result{Exit: 2}, err
	}
	base := toInput(in)
	findings := Evaluate(base)
	findings = append(findings, Semantic(semanticInput(in, base.AssistantText))...)
	findings = append(findings, CheckCanary(canaryFromFile(in, base.AssistantText))...)
	if err := writeVerdict(logPath, now, sidOf(in), "scan", findings); err != nil {
		return Result{Exit: 2}, err
	}
	return Result{RuleIDs: IDs(findings), DisputedIDs: disputedIDs(findings), Mode: AggregateMode(findings), Exit: 0}, nil
}

func loadFileInput(path string) (FileInput, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return FileInput{}, err
	}
	var in FileInput
	if err := json.Unmarshal(b, &in); err != nil {
		return FileInput{}, err
	}
	return in, nil
}

func toInput(in FileInput) Input {
	text := in.AssistantText
	if strings.TrimSpace(text) == "" {
		text = lastAssistant(in.Messages)
	}
	return Input{
		AssistantText: text,
		ReplyLanguage: in.ReplyLanguage,
		TestCommand:   in.TestCommand,
		TestGlobs:     in.TestPathGlobs,
		ChangedPaths:  in.ChangedPaths,
		Commands:      in.Commands,
	}
}

func lastAssistant(messages []Message) string {
	for i := len(messages) - 1; i >= 0; i-- {
		if strings.EqualFold(messages[i].Role, "assistant") && strings.TrimSpace(messages[i].Text) != "" {
			return messages[i].Text
		}
	}
	return ""
}

func sidOf(in FileInput) string {
	if in.SessionID != "" {
		return in.SessionID
	}
	if in.CaseID != "" {
		return in.CaseID
	}
	return "unknown"
}

func semanticInput(in FileInput, text string) SemanticInput {
	cfg := semanticFromRules(loadRules(rulesPathFromEnv()))
	if in.Markers != nil {
		if len(in.Markers.Goal) > 0 {
			cfg.Goal = in.Markers.Goal
		}
		if len(in.Markers.Source) > 0 {
			cfg.Source = in.Markers.Source
		}
		if len(in.Markers.Proof) > 0 {
			cfg.Proof = in.Markers.Proof
		}
		if len(in.Markers.Assumption) > 0 {
			cfg.Assumption = in.Markers.Assumption
		}
	}
	if in.AnchorMinShared > 0 {
		cfg.AnchorMin = in.AnchorMinShared
	}
	if len(in.PushbackPhrases) > 0 {
		cfg.Pushback = in.PushbackPhrases
	}
	if len(in.AgreementPhrases) > 0 {
		cfg.Agreement = in.AgreementPhrases
	}
	reads := readsFromMessages(in.Messages, in.Reads)
	anchors := anchorsFromMessages(in.Messages)
	if len(in.Anchors) > 0 {
		anchors = mergeUnique(in.Anchors, anchors)
	}
	opened := mergeUnique(reads, in.ChangedPaths)
	return SemanticInput{
		Text:     text,
		Changed:  in.ChangedPaths,
		Commands: in.Commands,
		Reads:    reads,
		Opened:   opened,
		Anchors:  anchors,
		Messages: in.Messages,
		Config:   cfg,
	}
}

func canaryFromFile(in FileInput, text string) CanaryInput {
	if len(in.CanaryFiles) == 0 {
		return CanaryInput{}
	}
	return CanaryInput{
		Text:      text,
		Files:     in.CanaryFiles,
		SessionID: sidOf(in),
		Secret:    in.CanarySecret,
		Reads:     readsFromMessages(in.Messages, in.Reads),
	}
}

func rulesPathFromEnv() string {
	if p := os.Getenv("DRIFT_GUARD_RULES"); p != "" {
		return p
	}
	if st, err := os.Stat("drift-guard.rules.json"); err == nil && !st.IsDir() {
		return "drift-guard.rules.json"
	}
	return ""
}

func writeVerdict(logPath string, now time.Time, sid, source string, findings []Finding) error {
	if now.IsZero() {
		now = time.Now().UTC()
	}
	ev := Event{
		TS:                     stamp(now),
		Kind:                   "verdict",
		SessionID:              sid,
		Source:                 source,
		Channel:                Channel(findings),
		Mode:                   AggregateMode(findings),
		Violations:             len(findings),
		RuleIDs:                IDs(findings),
		ElapsedMS:              0,
		TokensJudge:            0,
		TokensRulesInjected:    0,
		TokensFollowupEstimate: 0,
		Reason:                 joinReasons(findings),
		Disputed:               anyDisputed(findings),
		DisputedIDs:            disputedIDs(findings),
	}
	return AppendJSONL(logPath, ev)
}

func anyDisputed(findings []Finding) bool {
	return len(disputedIDs(findings)) > 0
}

func disputedIDs(findings []Finding) []string {
	var out []string
	for _, f := range findings {
		if f.Disputed {
			out = append(out, f.ID)
		}
	}
	return out
}

// timeParse is the small date parser report.go uses, kept next to the layout.
func timeParse(day string) (time.Time, error) {
	return time.Parse("2006-01-02", day)
}
