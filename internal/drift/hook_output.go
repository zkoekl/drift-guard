package drift

import (
	"encoding/json"
)

const pushDenyReason = "A push to main, master, the default branch, a force push, or a tag push is denied."

func denyJSON(event string) []byte {
	return denyPermissionJSON(event, pushDenyReason, "Push the feature branch without --force. Do not push main, the default branch, or a tag.")
}

func denyPermissionJSON(event, userMsg, agentMsg string) []byte {
	if isClaude(event) {
		b, _ := json.Marshal(map[string]any{
			"hookSpecificOutput": map[string]any{
				"hookEventName":            "PreToolUse",
				"permissionDecision":       "deny",
				"permissionDecisionReason": userMsg,
			},
		})
		return append(b, '\n')
	}
	b, _ := json.Marshal(map[string]any{
		"permission":    "deny",
		"user_message":  userMsg,
		"agent_message": agentMsg,
	})
	return append(b, '\n')
}

func allowJSON(event string) []byte {
	if isClaude(event) {
		return []byte("{}\n")
	}
	return []byte("{\"permission\":\"allow\"}\n")
}

func stopJSON(event, follow string) []byte {
	if follow == "" {
		return []byte("{}\n")
	}
	if isClaude(event) {
		b, _ := json.Marshal(map[string]any{
			"decision": "block",
			"reason":   follow,
		})
		return append(b, '\n')
	}
	b, _ := json.Marshal(map[string]string{"followup_message": follow})
	return append(b, '\n')
}
