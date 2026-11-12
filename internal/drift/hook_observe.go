package drift

import (
	"encoding/json"
	"strings"
	"time"

	"github.com/zkoekl/drift-guard/internal/rules"
)

func shellCommand(event string, payload map[string]any) (string, bool) {
	switch event {
	case "beforeShellExecution", "afterShellExecution":
		return stringField(payload, "command"), true
	case "preToolUse", "postToolUse", "PreToolUse", "PostToolUse":
		name := stringField(payload, "tool_name")
		if !isShellTool(name) {
			return "", false
		}
		in := asMap(payload["tool_input"])
		return stringField(in, "command"), true
	default:
		return "", false
	}
}

func isShellTool(name string) bool {
	switch strings.ToLower(name) {
	case "shell", "bash", "powershell":
		return true
	default:
		return false
	}
}

func observe(event string, payload map[string]any, state *sessionState, now time.Time) {
	switch event {
	case "postToolUse", "PostToolUse", "afterShellExecution":
		state.Evidence = true
		if cmd, ok := shellCommand(event, payload); ok && cmd != "" {
			exitCode := -1
			if n, ok := commandExit(payload); ok {
				exitCode = n
			}
			state.Commands = append(state.Commands, rules.Command{Command: cmd, ExitCode: exitCode})
			state.Reads = mergeUnique(state.Reads, shellReads(cmd))
		}
		if isReadTool(stringField(payload, "tool_name")) {
			if path := inputPath(payload); path != "" {
				state.Reads = mergeUnique(state.Reads, []string{path})
			}
		}
		if isWriteTool(stringField(payload, "tool_name")) {
			if path := inputPath(payload); path != "" {
				rememberCreated(state, path)
			}
		}
		if path := toolPath(payload); path != "" {
			state.Files = append(state.Files, fileNote{Path: path, At: now.UTC()})
		}
	case "afterFileEdit":
		state.Evidence = true
		if path := stringField(payload, "file_path"); path != "" {
			state.Files = append(state.Files, fileNote{Path: path, At: now.UTC()})
			rememberCreated(state, path)
		}
	}
}

func isReadTool(name string) bool {
	switch strings.ToLower(name) {
	case "read", "read_file", "readfile", "open", "open_file", "view":
		return true
	default:
		return false
	}
}

func inputPath(payload map[string]any) string {
	in := asMap(payload["tool_input"])
	for _, key := range []string{"path", "file_path", "target_file"} {
		if p := stringField(in, key); p != "" {
			return p
		}
	}
	return stringField(payload, "file_path")
}

func shellReads(command string) []string {
	fields := strings.Fields(command)
	i := 0
	for i < len(fields) && isEnvAssignLocal(fields[i]) {
		i++
	}
	if i >= len(fields) || !isReader(fields[i]) {
		return nil
	}
	var out []string
	for _, f := range fields[i+1:] {
		if strings.HasPrefix(f, "-") {
			continue
		}
		if strings.Contains(f, "/") || fileRe.MatchString(f) {
			out = append(out, f)
		}
	}
	return out
}

func isReader(tok string) bool {
	base := tok
	if i := strings.LastIndex(tok, "/"); i >= 0 {
		base = tok[i+1:]
	}
	switch strings.ToLower(base) {
	case "cat", "head", "tail", "less", "more", "bat", "nl", "open", "sed", "awk", "type":
		return true
	default:
		return false
	}
}

func isEnvAssignLocal(s string) bool {
	eq := strings.IndexByte(s, '=')
	return eq > 0 && !strings.Contains(s[:eq], "/")
}

func toolPath(payload map[string]any) string {
	name := strings.ToLower(stringField(payload, "tool_name"))
	switch name {
	case "write", "edit", "strreplace", "delete":
	default:
		return ""
	}
	in := asMap(payload["tool_input"])
	for _, key := range []string{"path", "file_path", "target_file"} {
		if p := stringField(in, key); p != "" {
			return p
		}
	}
	return ""
}

// commandExit reads a command result from a PostToolUse payload.
// Claude Code documents tool_response (https://code.claude.com/docs/en/hooks).
// For Bash the shape is {stdout, stderr, interrupted, isImage}, with no exit
// code. PostToolUse fires after a successful tool call; interrupted means the
// command did not finish. Cursor payloads still use tool_output or output,
// often with exitCode or exit_code.
func commandExit(payload map[string]any) (int, bool) {
	for _, key := range []string{"tool_response", "tool_output", "output"} {
		if n, ok := exitFromOutput(payload[key]); ok {
			return n, true
		}
	}
	return -1, false
}

func exitFromOutput(v any) (int, bool) {
	switch t := v.(type) {
	case map[string]any:
		if n, ok := t["exitCode"]; ok {
			return num(n), true
		}
		if n, ok := t["exit_code"]; ok {
			return num(n), true
		}
		if interruptedTrue(t["interrupted"]) {
			return 1, true
		}
		if _, ok := t["stdout"]; ok {
			return 0, true
		}
		if _, ok := t["stderr"]; ok {
			return 0, true
		}
	case string:
		var m map[string]any
		if json.Unmarshal([]byte(t), &m) == nil {
			return exitFromOutput(m)
		}
	}
	return -1, false
}

func interruptedTrue(v any) bool {
	switch t := v.(type) {
	case bool:
		return t
	case string:
		return strings.EqualFold(t, "true")
	default:
		return false
	}
}

func assistantText(payload map[string]any) string {
	for _, key := range []string{"last_assistant_message", "text"} {
		if s := stringField(payload, key); s != "" {
			return s
		}
	}
	return ""
}

func changedPaths(payload map[string]any, state sessionState) []string {
	var out []string
	seen := map[string]bool{}
	add := func(p string) {
		if p == "" || seen[p] {
			return
		}
		seen[p] = true
		out = append(out, p)
	}
	for _, p := range stringList(payload["modified_files"]) {
		add(p)
	}
	for _, note := range state.Files {
		add(note.Path)
	}
	return out
}
