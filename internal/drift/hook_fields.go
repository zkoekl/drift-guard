package drift

import (
	"encoding/json"

	"github.com/zkoekl/drift-guard/internal/rules"
)

func stringField(m map[string]any, key string) string {
	if m == nil {
		return ""
	}
	s, _ := m[key].(string)
	return s
}

func stringList(v any) []string {
	switch t := v.(type) {
	case []any:
		var out []string
		for _, item := range t {
			if s, ok := item.(string); ok && s != "" {
				out = append(out, s)
			}
		}
		return out
	case []string:
		return t
	default:
		return nil
	}
}

func commandList(v any) []rules.Command {
	arr, ok := v.([]any)
	if !ok {
		return nil
	}
	var out []rules.Command
	for _, item := range arr {
		m, ok := item.(map[string]any)
		if !ok {
			continue
		}
		out = append(out, rules.Command{
			Command:  stringField(m, "command"),
			ExitCode: numField(m, "exit_code"),
		})
	}
	return out
}

func asMap(v any) map[string]any {
	switch t := v.(type) {
	case map[string]any:
		return t
	case string:
		var m map[string]any
		if json.Unmarshal([]byte(t), &m) == nil {
			return m
		}
	}
	return nil
}

func numField(m map[string]any, key string) int {
	if m == nil {
		return 0
	}
	return num(m[key])
}

func num(v any) int {
	switch n := v.(type) {
	case float64:
		return int(n)
	case int:
		return n
	case json.Number:
		i, _ := n.Int64()
		return int(i)
	default:
		return 0
	}
}
