package drift

import (
	"context"
	"os/exec"
	"strings"
	"time"

	"github.com/zkoekl/drift-guard/internal/rules"
)

func gitPushContext(root string) rules.PushContext {
	if strings.TrimSpace(root) == "" {
		root = "."
	}
	cur := gitOne(root, "rev-parse", "--abbrev-ref", "HEAD")
	if cur == "HEAD" {
		cur = ""
	}
	up := trackingBranch(gitOne(root, "rev-parse", "--abbrev-ref", "--symbolic-full-name", "@{upstream}"))
	def := trackingBranch(gitOne(root, "symbolic-ref", "--short", "refs/remotes/origin/HEAD"))
	tags := map[string]bool{}
	for _, name := range gitMany(root, "for-each-ref", "--format=%(refname:short)", "refs/tags") {
		tags[name] = true
	}
	return rules.PushContext{Current: cur, Upstream: up, Default: def, Tags: tags}
}

func trackingBranch(ref string) string {
	ref = strings.TrimSpace(ref)
	const remotePrefix = "refs/remotes/"
	if strings.HasPrefix(ref, remotePrefix) {
		rest := strings.TrimPrefix(ref, remotePrefix)
		if i := strings.IndexByte(rest, '/'); i >= 0 && i+1 < len(rest) {
			return rest[i+1:]
		}
		return ""
	}
	if i := strings.IndexByte(ref, '/'); i >= 0 && i+1 < len(ref) {
		return ref[i+1:]
	}
	return ""
}

func gitOne(root string, args ...string) string {
	lines := gitMany(root, args...)
	if len(lines) == 0 {
		return ""
	}
	return lines[0]
}

func gitMany(root string, args ...string) []string {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = root
	out, err := cmd.Output()
	if err != nil {
		return nil
	}
	var lines []string
	for _, line := range strings.Split(string(out), "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			lines = append(lines, line)
		}
	}
	return lines
}
