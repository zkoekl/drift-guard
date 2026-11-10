package drift

import (
	"strings"

	"github.com/zkoekl/drift-guard/internal/rules"
)

// Finding is one deterministic rule hit. Mode log does not change the process exit.
// Mode block is a code-channel failure for `drift-guard check` (exit 1).
// Scan always exits 0. The stop hook exits 0 and may set followup_message.
// Disputed findings are logged and shown in the report. They do not block and
// do not follow up. Followup is the missing-marker check and a missing or stale
// canary code, both at stop, sharing one cap of two.
type Finding struct {
	ID       string
	Channel  string
	Mode     string
	Reason   string
	Disputed bool
	Followup bool
}

// Input is the checker input. Empty ReplyLanguage or TestCommand means that rule is off.
type Input struct {
	AssistantText string
	ReplyLanguage string
	TestCommand   string
	TestGlobs     []string
	ChangedPaths  []string
	Commands      []rules.Command
}

// Evaluate runs the reply-language check and the done-without-tests check.
func Evaluate(in Input) []Finding {
	var out []Finding
	if rules.ReplyLanguageViolation(in.ReplyLanguage, in.AssistantText) {
		out = append(out, Finding{
			ID:      "reply-language",
			Channel: "conversation",
			Mode:    "log",
			Reason:  "reply is not the required language",
		})
	}
	if rules.DoneWithoutTests(in.TestCommand, in.TestGlobs, in.ChangedPaths, in.Commands) {
		out = append(out, Finding{
			ID:      "done-without-tests",
			Channel: "code",
			Mode:    "block",
			Reason:  "changed paths have no successful test command",
		})
	}
	return out
}

// Message is one transcript turn. ToolCalls, Reads, or Commands mean a tool
// ran on that turn and count as new evidence for the sycophancy check.
type Message struct {
	Role      string          `json:"role"`
	Text      string          `json:"text"`
	ToolCalls []string        `json:"tool_calls,omitempty"`
	Reads     []string        `json:"reads,omitempty"`
	Commands  []rules.Command `json:"commands,omitempty"`
}

// Flip is the sycophancy pattern: pushback, then the next answer agrees or
// reverses the previous claim, and no tool call sits between them.
// It is log-only. It is not a model judgment.
func Flip(messages []Message) (bool, string) {
	return Sycophancy(messages, SemanticConfig{})
}

func assistantBefore(messages []Message) bool {
	for _, m := range messages {
		if strings.EqualFold(m.Role, "assistant") && strings.TrimSpace(m.Text) != "" {
			return true
		}
	}
	return false
}

// Block reports whether any finding must fail `drift-guard check`.
func Block(findings []Finding) bool {
	for _, f := range findings {
		if f.Mode == "block" {
			return true
		}
	}
	return false
}

// IDs returns rule ids in order.
func IDs(findings []Finding) []string {
	if len(findings) == 0 {
		return []string{}
	}
	out := make([]string, len(findings))
	for i, f := range findings {
		out[i] = f.ID
	}
	return out
}

// AggregateMode is block if any finding blocks, else log if there is a finding.
func AggregateMode(findings []Finding) string {
	if len(findings) == 0 {
		return ""
	}
	if Block(findings) {
		return "block"
	}
	return "log"
}

// Channel is code if any finding is on the code channel.
func Channel(findings []Finding) string {
	ch := "conversation"
	for _, f := range findings {
		if f.Channel == "code" {
			return "code"
		}
		if f.Channel != "" {
			ch = f.Channel
		}
	}
	if len(findings) == 0 {
		return ""
	}
	return ch
}
