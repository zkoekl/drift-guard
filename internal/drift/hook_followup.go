package drift

import (
	"strings"
	"unicode/utf8"
)

func wantsFollowup(findings []Finding) bool {
	for _, f := range findings {
		if f.Followup {
			return true
		}
	}
	return false
}

func composeFollowup(findings []Finding, testCommand string, cfg SemanticConfig, lang string) string {
	var parts []string
	if Block(findings) {
		parts = append(parts, followupText(testCommand, lang))
	}
	if hasFollowupID(findings, "answer-markers") {
		parts = append(parts, markerFollowup(cfg, lang))
	}
	if msg := canaryFollowup(findings, lang); msg != "" {
		parts = append(parts, msg)
	}
	return capBytes(strings.Join(parts, " "), 500)
}

func hasFollowupID(findings []Finding, id string) bool {
	for _, f := range findings {
		if f.Followup && f.ID == id {
			return true
		}
	}
	return false
}

func markerFollowup(cfg SemanticConfig, lang string) string {
	cfg = fillSemantic(cfg)
	var msg string
	if russian(lang) {
		msg = "drift-guard: добавьте непустые строки " + cfg.Goal[0] + ":, " + cfg.Source[0] +
			": (файл:строка или команда) и " + cfg.Proof[0] + ": (команда с кодом 0)."
	} else {
		msg = "drift-guard: add non-empty lines " + cfg.Goal[0] + ":, " + cfg.Source[0] +
			": (file:line or command), and " + cfg.Proof[0] + ": (a command that exited 0)."
	}
	return capBytes(msg, 240)
}

func russian(lang string) bool {
	return strings.EqualFold(strings.TrimSpace(lang), "ru")
}

func capBytes(s string, n int) string {
	if n <= 0 || s == "" {
		return ""
	}
	if len(s) <= n {
		return s
	}
	cut := n
	for cut > 0 && !utf8.RuneStart(s[cut]) {
		cut--
	}
	return s[:cut]
}

func followupText(testCommand, lang string) string {
	cmd := strings.TrimSpace(testCommand)
	if cmd == "" {
		if russian(lang) {
			cmd = "нужную тестовую команду"
		} else {
			cmd = "the required test command"
		}
	}
	cmd = capBytes(cmd, 80)
	var msg string
	if russian(lang) {
		msg = "drift-guard: изменённые файлы есть, а `" + cmd + "` не завершилась с кодом 0. Запустите её до остановки."
	} else {
		msg = "drift-guard: edited paths changed and `" + cmd + "` did not exit 0. Run it before stopping."
	}
	return capBytes(msg, 500)
}

func estTokens(s string) int {
	if s == "" {
		return 0
	}
	n := len(s) / 4
	if len(s)%4 != 0 {
		n++
	}
	return n
}

func joinReasons(findings []Finding) string {
	parts := make([]string, 0, len(findings))
	for _, f := range findings {
		if f.Reason != "" {
			parts = append(parts, f.ID+": "+f.Reason)
		}
	}
	return strings.Join(parts, "; ")
}
