package drift

import (
	"html"
	"sort"
	"strconv"
	"strings"
)

// ReportHTML is one self-contained page. It has no network URL and no script source.
func ReportHTML(events []Event) string {
	var b strings.Builder
	b.WriteString("<!DOCTYPE html>\n<html lang=\"en\">\n<head>\n<meta charset=\"utf-8\">\n<title>drift-guard report</title>\n<style>\n")
	b.WriteString(reportCSS)
	b.WriteString("</style>\n</head>\n<body>\n<h1>drift-guard report</h1>\n")
	b.WriteString("<p>Local file. Nothing is loaded from the network.</p>\n")
	writeSessions(&b, events)
	writeAmbiguous(&b, events)
	writeDisputed(&b, events)
	writeUnrequested(&b, events)
	writeLogOnly(&b, events)
	writeCatches(&b, events)
	writeFalseAlarms(&b, events)
	writeFixture(&b, events)
	writeFoundBy(&b, events)
	writeTokens(&b, events)
	writeChurn(&b, events)
	writeVerdicts(&b, events)
	b.WriteString("</body>\n</html>\n")
	return b.String()
}

const reportCSS = `
body { font: 15px/1.4 sans-serif; margin: 2rem; color: #1a1a1a; background: #fff; }
h1 { font-size: 1.4rem; }
h2 { font-size: 1.05rem; margin-top: 1.6rem; }
section { margin-bottom: 1.2rem; }
.empty { color: #555; }
.chart { max-width: 40rem; }
.row { display: flex; gap: 0.5rem; align-items: center; margin: 0.25rem 0; }
.label { width: 11rem; overflow: hidden; text-overflow: ellipsis; }
.track { flex: 1; background: #eee; height: 0.8rem; }
.bar { display: block; background: #246; height: 0.8rem; }
.n { width: 4rem; text-align: right; }
table { border-collapse: collapse; }
td, th { border: 1px solid #ccc; padding: 0.2rem 0.5rem; text-align: left; }
`

func writeSessions(b *strings.Builder, events []Event) {
	type info struct {
		active     bool
		inactive   bool
		activation string
	}
	m := map[string]*info{}
	for _, ev := range events {
		if ev.SessionID == "" {
			continue
		}
		if ev.Kind != "session_active" && ev.Kind != "session_inactive" {
			continue
		}
		inf := m[ev.SessionID]
		if inf == nil {
			inf = &info{}
			m[ev.SessionID] = inf
		}
		if ev.Kind == "session_inactive" {
			inf.inactive = true
		}
		if ev.Kind == "session_active" {
			inf.active = true
			if ev.Activation != "" {
				inf.activation = ev.Activation
			}
		}
	}
	b.WriteString("<section id=\"sessions\">\n<h2>Sessions</h2>\n")
	if len(m) == 0 {
		b.WriteString("<p class=\"empty\">No data yet.</p>\n</section>\n")
		return
	}
	activeN, inactiveN := 0, 0
	activeIDs := make([]string, 0)
	for id, inf := range m {
		if inf.active {
			activeN++
			activeIDs = append(activeIDs, id)
			continue
		}
		if inf.inactive {
			inactiveN++
		}
	}
	sort.Strings(activeIDs)
	b.WriteString("<p>Active sessions: ")
	b.WriteString(strconv.Itoa(activeN))
	b.WriteString(". Inactive sessions: ")
	b.WriteString(strconv.Itoa(inactiveN))
	b.WriteString(".</p>\n")
	if len(activeIDs) == 0 {
		b.WriteString("</section>\n")
		return
	}
	b.WriteString("<table>\n<tr><th>Session</th><th>Activation</th></tr>\n")
	for _, id := range activeIDs {
		act := m[id].activation
		if act == "" {
			act = "unspecified"
		}
		b.WriteString("<tr><td>")
		b.WriteString(html.EscapeString(id))
		b.WriteString("</td><td>")
		b.WriteString(html.EscapeString(act))
		b.WriteString("</td></tr>\n")
	}
	b.WriteString("</table>\n</section>\n")
}

func writeDisputed(b *strings.Builder, events []Event) {
	var rows []Event
	for _, ev := range events {
		if ev.Disputed || hasDisputedRule(ev) {
			rows = append(rows, ev)
		}
	}
	b.WriteString("<section id=\"disputed\">\n<h2>Disputed</h2>\n")
	if len(rows) == 0 {
		b.WriteString("<p class=\"empty\">No data yet.</p>\n")
	} else {
		b.WriteString("<ul>\n")
		for _, ev := range rows {
			ids := ev.DisputedIDs
			if len(ids) == 0 {
				ids = ev.RuleIDs
			}
			b.WriteString("<li>")
			b.WriteString(html.EscapeString(ev.TS + " " + ev.SessionID + " " + strings.Join(ids, ",") + " " + ev.Reason))
			b.WriteString("</li>\n")
		}
		b.WriteString("</ul>\n")
	}
	b.WriteString("</section>\n")
}

func hasDisputedRule(ev Event) bool {
	for _, id := range ev.RuleIDs {
		switch id {
		case "marker-citation", "anchor-miss", "sycophancy", "answered-from-memory", "flip-after-pushback", "canary-read":
			return true
		}
	}
	return false
}

func writeAmbiguous(b *strings.Builder, events []Event) {
	var rows []Event
	for _, ev := range events {
		if hasRule(ev, "flip-after-pushback") || hasRule(ev, "sycophancy") {
			rows = append(rows, ev)
		}
	}
	b.WriteString("<section id=\"ambiguous\">\n<h2>Ambiguous</h2>\n")
	if len(rows) == 0 {
		b.WriteString("<p class=\"empty\">No data yet.</p>\n")
	} else {
		b.WriteString("<ul>\n")
		for _, ev := range rows {
			b.WriteString("<li>")
			b.WriteString(html.EscapeString(ev.TS + " " + ev.SessionID + " " + ev.Reason))
			b.WriteString("</li>\n")
		}
		b.WriteString("</ul>\n")
	}
	b.WriteString("</section>\n")
}

func writeUnrequested(b *strings.Builder, events []Event) {
	var rows []Event
	for _, ev := range events {
		if ev.Kind == "unrequested-delete" {
			rows = append(rows, ev)
		}
	}
	b.WriteString("<section id=\"unrequested-delete\">\n<h2>Unrequested deletes</h2>\n")
	if len(rows) == 0 {
		b.WriteString("<p class=\"empty\">No data yet.</p>\n</section>\n")
		return
	}
	b.WriteString("<ul>\n")
	for _, ev := range rows {
		line := ev.TS + " " + ev.Path
		if ev.Tool != "" {
			line += " tool=" + ev.Tool
		}
		if ev.Command != "" {
			line += " command=" + ev.Command
		}
		if ev.Mode != "" {
			line += " mode=" + ev.Mode
		}
		line += " prompt_known=" + strconv.FormatBool(ev.PromptKnown)
		line += " vague_prompt=" + strconv.FormatBool(ev.VaguePrompt)
		line += " disputed=" + strconv.FormatBool(ev.Disputed)
		if ev.Reason != "" {
			line += " " + ev.Reason
		}
		b.WriteString("<li>")
		b.WriteString(html.EscapeString(line))
		b.WriteString("</li>\n")
	}
	b.WriteString("</ul>\n</section>\n")
}

func writeLogOnly(b *strings.Builder, events []Event) {
	b.WriteString("<section id=\"log-only\">\n<h2>Log only</h2>\n")
	n := 0
	b.WriteString("<ul>\n")
	for _, ev := range events {
		if ev.Kind != "verdict" || ev.Mode != "log" {
			continue
		}
		if len(ev.RuleIDs) == 1 && (ev.RuleIDs[0] == "flip-after-pushback" || ev.RuleIDs[0] == "sycophancy") {
			continue
		}
		if hasDisputedRule(ev) && !hasRule(ev, "answer-markers") && !hasRule(ev, "canary-code") && !hasRule(ev, "reply-language") && !hasRule(ev, "done-without-tests") {
			continue
		}
		if len(ev.RuleIDs) == 0 {
			continue
		}
		n++
		b.WriteString("<li>")
		b.WriteString(html.EscapeString(ev.TS + " " + strings.Join(ev.RuleIDs, ",") + " " + ev.Reason))
		b.WriteString("</li>\n")
	}
	b.WriteString("</ul>\n")
	if n == 0 {
		b.WriteString("<p class=\"empty\">No data yet.</p>\n")
	}
	b.WriteString("</section>\n")
}

func writeCatches(b *strings.Builder, events []Event) {
	counts := map[string]int{}
	for _, ev := range events {
		if ev.Kind == "verdict" && ev.Violations > 0 {
			counts[day(ev.TS)] += ev.Violations
		}
	}
	writeBars(b, "catches", "Catches over time", counts)
}

func writeFalseAlarms(b *strings.Builder, events []Event) {
	counts := map[string]int{}
	for _, ev := range events {
		if ev.Kind == "label" && ev.FalseAlarm != nil && *ev.FalseAlarm {
			counts[day(ev.TS)]++
		}
	}
	writeBars(b, "false-alarms", "False alarms over time", counts)
}

func writeFixture(b *strings.Builder, events []Event) {
	latest := map[string]Event{}
	for _, ev := range events {
		if ev.Kind != "fixture_run" {
			continue
		}
		key := ev.InScope
		if key == "" {
			key = "fixtures"
		}
		latest[key] = ev
	}
	b.WriteString("<section id=\"fixtures\">\n<h2>Precision and recall</h2>\n")
	if len(latest) == 0 {
		b.WriteString("<p class=\"empty\">No data yet.</p>\n</section>\n")
		return
	}
	keys := make([]string, 0, len(latest))
	for k := range latest {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	b.WriteString("<div class=\"chart\">\n")
	for _, k := range keys {
		ev := latest[k]
		b.WriteString("<p>")
		b.WriteString(html.EscapeString(k))
		b.WriteString("</p>\n")
		if ev.Precision == nil {
			b.WriteString("<p>precision undefined</p>\n")
		} else {
			writeOneBar(b, "precision", *ev.Precision)
		}
		if ev.Recall == nil {
			b.WriteString("<p>recall undefined</p>\n")
		} else {
			writeOneBar(b, "recall", *ev.Recall)
		}
	}
	b.WriteString("</div>\n</section>\n")
}

func writeFoundBy(b *strings.Builder, events []Event) {
	counts := map[string]int{}
	for _, ev := range events {
		if ev.Kind != "label" || ev.FalseAlarm == nil || *ev.FalseAlarm {
			continue
		}
		who := ev.FoundBy
		if who == "" {
			who = "unlabeled"
		}
		counts[who]++
	}
	writeBars(b, "found-by", "User-caught vs tool-caught", counts)
}

func writeTokens(b *strings.Builder, events []Event) {
	counts := map[string]int{}
	anyVerdict := false
	for _, ev := range events {
		if ev.Kind != "verdict" {
			continue
		}
		anyVerdict = true
		sid := ev.SessionID
		if sid == "" {
			sid = "unknown"
		}
		counts[sid] += ev.TokensJudge + ev.TokensRulesInjected + ev.TokensFollowupEstimate
	}
	b.WriteString("<section id=\"tokens\">\n<h2>Tokens per session</h2>\n")
	if !anyVerdict {
		b.WriteString("<p class=\"empty\">No data yet.</p>\n</section>\n")
		return
	}
	b.WriteString("<p>Estimates use ceil(characters / 4). Judge tokens are 0.</p>\n")
	writeBarMap(b, counts)
	b.WriteString("</section>\n")
}

func writeChurn(b *strings.Builder, events []Event) {
	type row struct{ add, del int }
	weeks := map[string]*row{}
	for _, ev := range events {
		if ev.Kind != "rules_changed" {
			continue
		}
		w := week(ev.TS)
		if weeks[w] == nil {
			weeks[w] = &row{}
		}
		weeks[w].add += ev.LinesAdded
		weeks[w].del += ev.LinesRemoved
	}
	b.WriteString("<section id=\"churn\">\n<h2>Rule churn</h2>\n")
	if len(weeks) == 0 {
		b.WriteString("<p class=\"empty\">No data yet.</p>\n</section>\n")
		return
	}
	keys := make([]string, 0, len(weeks))
	for k := range weeks {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	b.WriteString("<table>\n<tr><th>Week</th><th>Lines added</th><th>Lines removed</th></tr>\n")
	for _, k := range keys {
		r := weeks[k]
		b.WriteString("<tr><td>")
		b.WriteString(html.EscapeString(k))
		b.WriteString("</td><td>")
		b.WriteString(strconv.Itoa(r.add))
		b.WriteString("</td><td>")
		b.WriteString(strconv.Itoa(r.del))
		b.WriteString("</td></tr>\n")
	}
	b.WriteString("</table>\n</section>\n")
}

func writeVerdicts(b *strings.Builder, events []Event) {
	b.WriteString("<section id=\"verdicts\">\n<h2>Verdicts</h2>\n<table>\n")
	b.WriteString("<tr><th>Time</th><th>Source</th><th>Mode</th><th>Rules</th><th>Violations</th></tr>\n")
	n := 0
	for _, ev := range events {
		if ev.Kind != "verdict" && ev.Kind != "" {
			continue
		}
		if ev.Kind != "verdict" {
			continue
		}
		n++
		b.WriteString("<tr><td>")
		b.WriteString(html.EscapeString(ev.TS))
		b.WriteString("</td><td>")
		b.WriteString(html.EscapeString(ev.Source))
		b.WriteString("</td><td>")
		b.WriteString(html.EscapeString(ev.Mode))
		b.WriteString("</td><td>")
		b.WriteString(html.EscapeString(strings.Join(ev.RuleIDs, ", ")))
		b.WriteString("</td><td>")
		b.WriteString(strconv.Itoa(ev.Violations))
		b.WriteString("</td></tr>\n")
	}
	b.WriteString("</table>\n")
	if n == 0 {
		b.WriteString("<p class=\"empty\">No data yet.</p>\n")
	}
	b.WriteString("</section>\n")
}

func writeBars(b *strings.Builder, id, title string, counts map[string]int) {
	b.WriteString("<section id=\"")
	b.WriteString(id)
	b.WriteString("\">\n<h2>")
	b.WriteString(html.EscapeString(title))
	b.WriteString("</h2>\n")
	if len(counts) == 0 {
		b.WriteString("<p class=\"empty\">No data yet.</p>\n</section>\n")
		return
	}
	writeBarMap(b, counts)
	b.WriteString("</section>\n")
}

func writeBarMap(b *strings.Builder, counts map[string]int) {
	keys := make([]string, 0, len(counts))
	max := 0
	for k, n := range counts {
		keys = append(keys, k)
		if n > max {
			max = n
		}
	}
	sort.Strings(keys)
	if max < 1 {
		max = 1
	}
	b.WriteString("<div class=\"chart\">\n")
	for _, k := range keys {
		n := counts[k]
		width := n * 100 / max
		b.WriteString("<div class=\"row\"><span class=\"label\">")
		b.WriteString(html.EscapeString(k))
		b.WriteString("</span><span class=\"track\"><span class=\"bar\" style=\"width:")
		b.WriteString(strconv.Itoa(width))
		b.WriteString("%\"></span></span><span class=\"n\">")
		b.WriteString(strconv.Itoa(n))
		b.WriteString("</span></div>\n")
	}
	b.WriteString("</div>\n")
}

func writeOneBar(b *strings.Builder, label string, value float64) {
	if value < 0 {
		value = 0
	}
	if value > 1 {
		value = 1
	}
	width := int(value * 100)
	b.WriteString("<div class=\"row\"><span class=\"label\">")
	b.WriteString(html.EscapeString(label))
	b.WriteString("</span><span class=\"track\"><span class=\"bar\" style=\"width:")
	b.WriteString(strconv.Itoa(width))
	b.WriteString("%\"></span></span><span class=\"n\">")
	b.WriteString(html.EscapeString(strconv.FormatFloat(value, 'f', 3, 64)))
	b.WriteString("</span></div>\n")
}

func hasRule(ev Event, id string) bool {
	for _, r := range ev.RuleIDs {
		if r == id {
			return true
		}
	}
	return false
}

func day(ts string) string {
	if len(ts) >= 10 {
		return ts[:10]
	}
	if ts == "" {
		return "unknown"
	}
	return ts
}

func week(ts string) string {
	// ISO-ish bucket: the date is enough for an empty prototype log.
	// Group by the Monday of that calendar week without pulling in extra code.
	d := day(ts)
	if len(d) < 10 {
		return d
	}
	t, err := timeParse(d)
	if err != nil {
		return d
	}
	wd := int(t.Weekday())
	if wd == 0 {
		wd = 7
	}
	mon := t.AddDate(0, 0, -(wd - 1))
	return mon.Format("2006-01-02")
}
