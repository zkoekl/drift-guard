package drift

import (
	"regexp"
	"strings"
	"unicode/utf8"

	"github.com/zkoekl/drift-guard/internal/rules"
)

// SemanticConfig is the deterministic conversation experiment.
// Empty lists mean the built-in defaults. AnchorMin <= 0 means 1.
// No model is used.
type SemanticConfig struct {
	Goal       []string
	Source     []string
	Proof      []string
	Assumption []string
	AnchorMin  int
	Pushback   []string
	Agreement  []string
}

// DefaultSemantic is the built-in marker names, phrase lists, and anchor minimum.
func DefaultSemantic() SemanticConfig {
	return SemanticConfig{
		Goal:       []string{"ЦЕЛЬ", "GOAL"},
		Source:     []string{"ИСТОЧНИК", "SOURCE"},
		Proof:      []string{"ДОКАЗАНО", "PROOF"},
		Assumption: []string{"ДОПУЩЕНИЕ", "ASSUMPTION"},
		AnchorMin:  1,
		Pushback: []string{
			"ты не прав",
			"ты неправа",
			"а разве",
			"you're wrong",
			"you are wrong",
			"are you sure",
			"that's wrong",
			"that is wrong",
			"это не так",
			"неверно",
		},
		Agreement: []string{
			"вы правы",
			"you're absolutely right",
			"you are absolutely right",
			"you're right",
			"you are right",
			"i was wrong",
			"я ошибся",
			"я был неправ",
			"я была неправа",
			"согласен",
		},
	}
}

// SemanticInput is one reply plus the session evidence the checks can see.
// Opened is the read set plus edited paths (a source line may name either).
// Reads is files actually read or opened for content. Memory uses Reads only.
type SemanticInput struct {
	Text     string
	Changed  []string
	Commands []rules.Command
	Reads    []string
	Opened   []string
	Anchors  []string
	Messages []Message
	Config   SemanticConfig
	Pushback bool
	Evidence bool
	Prev     string
	User     string
}

// Semantic runs the four checks. Disputed findings do not block.
// A missing or empty marker asks the stop hook to follow up. The canary check
// is separate and may follow up as well. The two share one cap.
func Semantic(in SemanticInput) []Finding {
	cfg := fillSemantic(in.Config)
	text := in.Text
	var out []Finding
	if missing := missingMarkers(text, cfg); len(missing) > 0 {
		out = append(out, Finding{
			ID:       "answer-markers",
			Channel:  "conversation",
			Mode:     "log",
			Followup: true,
			Reason:   "missing or empty marker: " + strings.Join(missing, ", "),
		})
	}
	if ok, reason := citationDispute(text, cfg, in.Opened, in.Commands); ok {
		out = append(out, Finding{
			ID:       "marker-citation",
			Channel:  "conversation",
			Mode:     "log",
			Disputed: true,
			Reason:   reason,
		})
	}
	goal := firstMarkerValue(text, cfg.Goal)
	if ok, reason := AnchorMiss(in.Anchors, cfg.AnchorMin, goal, text, in.Changed); ok {
		out = append(out, Finding{
			ID:       "anchor-miss",
			Channel:  "conversation",
			Mode:     "log",
			Disputed: true,
			Reason:   reason,
		})
	}
	if ok, reason := sycophancy(in, cfg); ok {
		out = append(out, Finding{
			ID:       "sycophancy",
			Channel:  "conversation",
			Mode:     "log",
			Disputed: true,
			Reason:   reason,
		})
	}
	if ok, reason := answeredFromMemory(text, in.Reads); ok {
		out = append(out, Finding{
			ID:       "answered-from-memory",
			Channel:  "conversation",
			Mode:     "log",
			Disputed: true,
			Reason:   reason,
		})
	}
	return out
}

func fillSemantic(cfg SemanticConfig) SemanticConfig {
	d := DefaultSemantic()
	if len(cfg.Goal) == 0 {
		cfg.Goal = d.Goal
	}
	if len(cfg.Source) == 0 {
		cfg.Source = d.Source
	}
	if len(cfg.Proof) == 0 {
		cfg.Proof = d.Proof
	}
	// Assumption is accepted and optional. missingMarkers does not require it.
	if len(cfg.Assumption) == 0 {
		cfg.Assumption = d.Assumption
	}
	if cfg.AnchorMin <= 0 {
		cfg.AnchorMin = d.AnchorMin
	}
	if len(cfg.Pushback) == 0 {
		cfg.Pushback = d.Pushback
	}
	if len(cfg.Agreement) == 0 {
		cfg.Agreement = d.Agreement
	}
	return cfg
}

func missingMarkers(text string, cfg SemanticConfig) []string {
	var missing []string
	groups := []struct {
		names []string
		label string
	}{
		{cfg.Goal, firstName(cfg.Goal)},
		{cfg.Source, firstName(cfg.Source)},
		{cfg.Proof, firstName(cfg.Proof)},
	}
	for _, g := range groups {
		if len(markerValues(text, g.names)) == 0 {
			missing = append(missing, g.label)
		}
	}
	return missing
}

func firstName(names []string) string {
	if len(names) == 0 {
		return ""
	}
	return names[0]
}

func markerValues(text string, names []string) []string {
	var vals []string
	for _, line := range strings.Split(text, "\n") {
		_, val, ok := splitMarker(strings.TrimSpace(line), names)
		if !ok {
			continue
		}
		val = strings.TrimSpace(val)
		if val != "" {
			vals = append(vals, val)
		}
	}
	return vals
}

func firstMarkerValue(text string, names []string) string {
	vals := markerValues(text, names)
	if len(vals) == 0 {
		return ""
	}
	return vals[0]
}

func splitMarker(line string, names []string) (name, val string, ok bool) {
	i := strings.IndexByte(line, ':')
	if i <= 0 {
		return "", "", false
	}
	n := strings.TrimSpace(line[:i])
	for _, want := range names {
		if strings.EqualFold(n, want) {
			return want, line[i+1:], true
		}
	}
	return "", "", false
}

func citationDispute(text string, cfg SemanticConfig, opened []string, commands []rules.Command) (bool, string) {
	// An absent marker is the follow-up finding. Citation runs only on values that exist.
	var problems []string
	for _, src := range markerValues(text, cfg.Source) {
		if file, ok := fileCite(src); ok {
			if !wasOpened(file, opened) {
				problems = append(problems, "source not opened: "+file)
			}
			continue
		}
		if !commandOK(src, commands) {
			problems = append(problems, "source command not exit 0")
		}
	}
	for _, proof := range markerValues(text, cfg.Proof) {
		if !commandOK(proof, commands) {
			problems = append(problems, "proof command not exit 0")
		}
	}
	if len(problems) == 0 {
		return false, ""
	}
	if len(problems) > 3 {
		problems = problems[:3]
	}
	return true, "disputed citation: " + strings.Join(problems, "; ")
}

var fileCiteRe = regexp.MustCompile(`^([\w./-]+\.[A-Za-z0-9]{1,8})(?::\d+)?$`)

func fileCite(value string) (string, bool) {
	m := fileCiteRe.FindStringSubmatch(strings.TrimSpace(value))
	if m == nil {
		return "", false
	}
	return m[1], true
}

func wasOpened(cited string, opened []string) bool {
	c := normPath(cited)
	if c == "" {
		return false
	}
	base := pathBase(c)
	for _, r := range opened {
		n := normPath(r)
		if n == "" {
			continue
		}
		if n == c || strings.HasSuffix(n, "/"+c) || strings.HasSuffix(c, "/"+n) {
			return true
		}
		if !strings.Contains(c, "/") && (n == base || strings.HasSuffix(n, "/"+base)) {
			return true
		}
	}
	return false
}

func normPath(p string) string {
	p = strings.TrimSpace(p)
	p = strings.TrimPrefix(p, "./")
	p = strings.ReplaceAll(p, "\\", "/")
	return strings.Trim(p, "/")
}

func pathBase(p string) string {
	if i := strings.LastIndex(p, "/"); i >= 0 {
		return p[i+1:]
	}
	return p
}

func commandOK(proof string, commands []rules.Command) bool {
	want := strings.Join(strings.Fields(proof), " ")
	if want == "" {
		return false
	}
	for _, c := range commands {
		if c.ExitCode != 0 {
			continue
		}
		got := strings.Join(strings.Fields(c.Command), " ")
		if got == "" {
			continue
		}
		if got == want || strings.Contains(got, want) {
			return true
		}
	}
	return false
}

// AnchorMiss is disputed when the prompt yielded anchors and the goal line,
// the changed paths, and the answer together share fewer than the minimum.
// A prompt with no anchors does not fire. need is min(minShared, len(anchors)).
func AnchorMiss(anchors []string, minShared int, goal, answer string, changed []string) (bool, string) {
	if len(anchors) == 0 {
		return false, ""
	}
	if minShared <= 0 {
		minShared = 1
	}
	need := minShared
	if need > len(anchors) {
		need = len(anchors)
	}
	hay := goal + "\n" + strings.Join(changed, "\n") + "\n" + answer
	hit := 0
	for _, a := range anchors {
		if anchorHit(a, hay) {
			hit++
		}
	}
	if hit >= need {
		return false, ""
	}
	return true, "disputed anchors: shared " + itoa(hit) + " of " + itoa(len(anchors)) + ", need " + itoa(need)
}

func anchorHit(anchor, hay string) bool {
	if anchor == "" || hay == "" {
		return false
	}
	if numberRe.MatchString(anchor) && !strings.ContainsAny(anchor, "._-/") && !strings.Contains(anchor, " ") {
		return numberIn(anchor, hay)
	}
	return strings.Contains(hay, anchor)
}

func numberIn(anchor, hay string) bool {
	rest := hay
	for {
		i := strings.Index(rest, anchor)
		if i < 0 {
			return false
		}
		beforeOK := i == 0 || !isDigitByte(rest[i-1])
		end := i + len(anchor)
		afterOK := end >= len(rest) || !isDigitByte(rest[end])
		// A decimal point means this digit run is part of a larger number token.
		if beforeOK && i > 0 && rest[i-1] == '.' {
			beforeOK = false
		}
		if afterOK && end < len(rest) && rest[end] == '.' {
			afterOK = false
		}
		if beforeOK && afterOK {
			return true
		}
		rest = rest[i+len(anchor):]
	}
}

func isDigitByte(b byte) bool {
	return b >= '0' && b <= '9'
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b [16]byte
	i := len(b)
	neg := n < 0
	if neg {
		n = -n
	}
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		b[i] = '-'
	}
	return string(b[i:])
}

var (
	// Single quotes count only when they are delimiters, not possessives or contractions.
	quoteRe  = regexp.MustCompile(`"([^"\n]{2,80})"|(?:^|[\t\n ])'([^'\n]{2,80})'(?:$|[\t\n .,;:!?])`)
	pathRe   = regexp.MustCompile(`(?:\.{1,2}/)?(?:[\w.-]+/)+[\w.-]*[\w]`)
	fileRe   = regexp.MustCompile(`\b[\w.-]+\.(?:go|py|json|ya?ml|md|txt|java|rs|toml|sh|ts|tsx|js|html|css|xml|conf|ini|cfg)\b`)
	snakeRe  = regexp.MustCompile(`\b[A-Za-z][A-Za-z0-9]*(?:_[A-Za-z0-9]+)+\b`)
	camelRe  = regexp.MustCompile(`\b(?:[A-Z][a-z0-9]+(?:[A-Z][a-z0-9]+)+|[A-Z]{2,}[a-z0-9]+(?:[A-Z][a-z0-9]+)*)\b`)
	numberRe = regexp.MustCompile(`\b\d+(?:\.\d+)+\b|\b\d+\b`)
)

// ExtractAnchors pulls file names, code identifiers, paths, numbers, and quoted strings.
// Order is quotes, paths, file names, snake_case, CamelCase, numbers. Duplicates drop.
func ExtractAnchors(text string) []string {
	var found []string
	seen := map[string]bool{}
	add := func(s string) {
		s = strings.TrimSpace(s)
		if s == "" || seen[s] {
			return
		}
		seen[s] = true
		found = append(found, s)
	}
	for _, m := range quoteRe.FindAllStringSubmatch(text, -1) {
		if m[1] != "" {
			add(m[1])
		} else {
			add(m[2])
		}
	}
	for _, m := range pathRe.FindAllString(text, -1) {
		add(m)
	}
	for _, m := range fileRe.FindAllString(text, -1) {
		add(m)
	}
	for _, m := range snakeRe.FindAllString(text, -1) {
		add(m)
	}
	for _, m := range camelRe.FindAllString(text, -1) {
		add(m)
	}
	for _, m := range numberRe.FindAllString(text, -1) {
		add(m)
	}
	return found
}

const sycophancyReason = "sycophancy: conclusion changed after pushback and no tool call in between"

func sycophancy(in SemanticInput, cfg SemanticConfig) (bool, string) {
	if len(in.Messages) > 0 {
		return Sycophancy(in.Messages, cfg)
	}
	if !in.Pushback || in.Evidence {
		return false, ""
	}
	if opensAgreement(in.Text, cfg.Agreement) || reverses(in.Prev, in.Text) {
		if strings.TrimSpace(in.Prev) == "" || strings.TrimSpace(in.Text) == "" {
			return false, ""
		}
		return true, sycophancyReason
	}
	return false, ""
}

// Sycophancy is the flip check: a pushback phrase, then the next assistant
// answer agrees or reverses the previous claim, with no tool, command, or
// file read on the messages in between. Log only. Not a model judgment.
func Sycophancy(messages []Message, cfg SemanticConfig) (bool, string) {
	cfg = fillSemantic(cfg)
	for i, m := range messages {
		if !strings.EqualFold(m.Role, "user") || !containsPhrase(m.Text, cfg.Pushback) {
			continue
		}
		if !assistantBefore(messages[:i]) {
			continue
		}
		prev := lastAssistant(messages[:i])
		for j := i + 1; j < len(messages); j++ {
			if hasEvidence(messages[j]) {
				break
			}
			if !strings.EqualFold(messages[j].Role, "assistant") || strings.TrimSpace(messages[j].Text) == "" {
				continue
			}
			if opensAgreement(messages[j].Text, cfg.Agreement) || reverses(prev, messages[j].Text) {
				return true, sycophancyReason
			}
			break
		}
	}
	return false, ""
}

func hasEvidence(m Message) bool {
	return len(m.ToolCalls) > 0 || len(m.Reads) > 0 || len(m.Commands) > 0
}

func containsPhrase(text string, phrases []string) bool {
	low := strings.ToLower(text)
	for _, p := range phrases {
		p = strings.ToLower(strings.TrimSpace(p))
		if p != "" && strings.Contains(low, p) {
			return true
		}
	}
	return false
}

func opensAgreement(text string, phrases []string) bool {
	low := strings.ToLower(strings.TrimSpace(text))
	for _, p := range phrases {
		p = strings.ToLower(strings.TrimSpace(p))
		if p == "" {
			continue
		}
		idx := strings.Index(low, p)
		if idx >= 0 && idx <= 40 {
			return true
		}
	}
	return false
}

var (
	negEN  = regexp.MustCompile(`(?i)\b(not|no|never|isn't|aren't|wasn't|weren't|don't|doesn't|didn't|cannot|can't|won't)\b`)
	negRU  = regexp.MustCompile(`(?i)(?:^|[^\p{L}])(не|нет|никогда)(?:$|[^\p{L}])`)
	wordRe = regexp.MustCompile(`\p{L}{4,}`)
)

func reverses(prev, next string) bool {
	if strings.TrimSpace(prev) == "" || strings.TrimSpace(next) == "" {
		return false
	}
	if negated(prev) == negated(next) {
		return false
	}
	return sharedContent(prev, next) >= 2
}

func negated(s string) bool {
	return negEN.MatchString(s) || negRU.MatchString(s)
}

func sharedContent(a, b string) int {
	left := contentSet(a)
	n := 0
	for w := range contentSet(b) {
		if left[w] {
			n++
		}
	}
	return n
}

var contentStop = map[string]bool{
	"that": true, "this": true, "with": true, "from": true, "have": true,
	"were": true, "your": true, "will": true, "just": true, "only": true,
	"into": true, "about": true, "there": true, "their": true, "what": true,
	"when": true, "where": true, "which": true, "would": true, "could": true,
	"should": true, "assistant": true, "user": true, "they": true, "them": true,
	"then": true, "than": true, "been": true, "being": true, "does": true,
	"done": true, "said": true, "says": true, "also": true, "some": true,
	"such": true, "each": true, "other": true, "after": true, "before": true,
	"это": true, "этот": true, "быть": true, "если": true, "когда": true,
	"только": true, "также": true, "через": true, "после": true, "перед": true,
	"между": true, "который": true, "которая": true,
}

func contentSet(s string) map[string]bool {
	out := map[string]bool{}
	for _, w := range wordRe.FindAllString(strings.ToLower(s), -1) {
		if contentStop[w] {
			continue
		}
		out[w] = true
	}
	return out
}

var versionRe = regexp.MustCompile(`\bv?\d+\.\d+(?:\.\d+)?\b`)

func answeredFromMemory(text string, reads []string) (bool, string) {
	var unread []string
	seen := map[string]bool{}
	for _, p := range mentionedPaths(text) {
		if seen[p] || wasOpened(p, reads) {
			seen[p] = true
			continue
		}
		seen[p] = true
		unread = append(unread, p)
	}
	versions := versionRe.FindAllString(text, -1)
	versionNote := len(versions) > 0 && len(reads) == 0
	if len(unread) == 0 && !versionNote {
		return false, ""
	}
	var parts []string
	if len(unread) > 0 {
		show := unread
		if len(show) > 3 {
			show = show[:3]
		}
		parts = append(parts, "unread path: "+strings.Join(show, ", "))
	}
	if versionNote {
		v := versions[0]
		parts = append(parts, "version with no file read: "+v)
	}
	return true, "disputed memory: " + strings.Join(parts, "; ")
}

func mentionedPaths(text string) []string {
	var out []string
	seen := map[string]bool{}
	add := func(s string) {
		s = strings.TrimSpace(s)
		if s == "" || seen[s] || strings.Contains(s, "://") {
			return
		}
		seen[s] = true
		out = append(out, s)
	}
	for _, m := range pathRe.FindAllString(text, -1) {
		add(m)
	}
	for _, m := range fileRe.FindAllString(text, -1) {
		add(m)
	}
	return out
}

func mergeUnique(base, extra []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, s := range append(append([]string{}, base...), extra...) {
		if s == "" || seen[s] {
			continue
		}
		seen[s] = true
		out = append(out, s)
	}
	return out
}

func anchorsFromMessages(messages []Message) []string {
	var out []string
	for _, m := range messages {
		if strings.EqualFold(m.Role, "user") {
			out = mergeUnique(out, ExtractAnchors(m.Text))
		}
	}
	return out
}

func readsFromMessages(messages []Message, extra []string) []string {
	out := append([]string{}, extra...)
	for _, m := range messages {
		out = append(out, m.Reads...)
	}
	return out
}

func truncateRunes(s string, n int) string {
	if n <= 0 || s == "" {
		return ""
	}
	if utf8.RuneCountInString(s) <= n {
		return s
	}
	i := 0
	for count := 0; count < n && i < len(s); count++ {
		_, size := utf8.DecodeRuneInString(s[i:])
		i += size
	}
	return s[:i]
}
