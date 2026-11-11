package rules

import "strings"

// PushContext is the local git state used to classify a push.
// An empty Current means the branch could not be read.
// main and master are always protected. Default is an extra name, usually origin/HEAD.
// Tags lists short tag names that exist in the repository.
type PushContext struct {
	Current  string
	Upstream string
	Default  string
	Tags     map[string]bool
}

// PushDenied reports whether command contains a git push that must be blocked.
// A normal push of a feature branch is allowed. A push to main, master, or the
// default branch is denied, as is a force push, a tag push, --all, and --mirror.
// A push whose destination cannot be named is denied.
func PushDenied(command string, ctx PushContext) bool {
	for _, seg := range splitTop(command) {
		if segmentPushDenied(seg, ctx) {
			return true
		}
	}
	return false
}

func segmentPushDenied(seg string, ctx PushContext) bool {
	toks := tokenize(seg)
	i := 0
	for i < len(toks) && isEnvAssign(toks[i]) {
		i++
	}
	if i < len(toks) && strings.EqualFold(toks[i], "sudo") {
		i++
		for i < len(toks) && isEnvAssign(toks[i]) {
			i++
		}
	}
	if i < len(toks) && isShell(toks[i]) {
		for j := i + 1; j < len(toks); j++ {
			if (toks[j] == "-c" || toks[j] == "-lc") && j+1 < len(toks) && PushDenied(toks[j+1], ctx) {
				return true
			}
		}
	}
	at := gitPushIndex(toks)
	if at < 0 {
		return false
	}
	return classifyPush(toks[at+1:], ctx)
}

func gitPushIndex(toks []string) int {
	i := 0
	for i < len(toks) && isEnvAssign(toks[i]) {
		i++
	}
	if i < len(toks) && strings.EqualFold(toks[i], "sudo") {
		i++
		for i < len(toks) && isEnvAssign(toks[i]) {
			i++
		}
	}
	if i >= len(toks) || !strings.EqualFold(toks[i], "git") {
		return -1
	}
	i++
	for i < len(toks) {
		t := toks[i]
		if strings.EqualFold(t, "push") {
			return i
		}
		switch strings.ToLower(t) {
		case "-c", "-C", "--git-dir", "--namespace", "--work-tree":
			i += 2
			continue
		}
		if strings.HasPrefix(t, "-") {
			i++
			continue
		}
		return -1
	}
	return -1
}

func classifyPush(args []string, ctx PushContext) bool {
	force := false
	tags := false
	all := false
	mirror := false
	del := false
	var pos []string
	for i := 0; i < len(args); i++ {
		a := args[i]
		if a == "--" {
			pos = append(pos, args[i+1:]...)
			break
		}
		if strings.HasPrefix(a, "-") && a != "-" {
			kind, takes := pushFlag(a)
			switch kind {
			case "force":
				force = true
			case "tags":
				tags = true
			case "all":
				all = true
			case "mirror":
				mirror = true
			case "delete":
				del = true
			}
			if takes && i+1 < len(args) && !strings.HasPrefix(args[i+1], "-") {
				i++
			}
			continue
		}
		pos = append(pos, a)
	}
	if force || tags || all || mirror {
		return true
	}
	specs := refspecsOf(pos)
	if len(specs) == 0 {
		if del {
			return true
		}
		return defaultPushDenied(ctx)
	}
	for _, spec := range specs {
		if refspecDenied(spec, ctx) {
			return true
		}
	}
	return false
}

func pushFlag(a string) (kind string, takes bool) {
	if strings.HasPrefix(a, "--") {
		name := strings.TrimPrefix(a, "--")
		if eq := strings.IndexByte(name, '='); eq >= 0 {
			name = name[:eq]
			takes = false
		}
		switch name {
		case "force", "force-with-lease", "force-if-includes":
			return "force", false
		case "tags", "follow-tags":
			return "tags", false
		case "all":
			return "all", false
		case "mirror":
			return "mirror", false
		case "delete":
			return "delete", false
		case "push-option", "repo", "receive-pack", "exec":
			return "", !strings.Contains(a, "=")
		default:
			return "", false
		}
	}
	cluster := strings.TrimPrefix(a, "-")
	for i := 0; i < len(cluster); i++ {
		switch cluster[i] {
		case 'f':
			kind = "force"
		case 'd':
			kind = "delete"
		case 'o':
			if i == len(cluster)-1 {
				takes = true
			}
		}
	}
	return kind, takes
}

func refspecsOf(pos []string) []string {
	if len(pos) == 0 {
		return nil
	}
	if strings.Contains(pos[0], ":") {
		return pos
	}
	if len(pos) == 1 {
		return nil
	}
	return pos[1:]
}

func refspecDenied(spec string, ctx PushContext) bool {
	if strings.HasPrefix(spec, "+") {
		return true
	}
	src, dst, colon := splitRefspec(spec)
	if colon && (src == "" || dst == "") {
		name := dst
		if name == "" {
			name = src
		}
		return destDenied(name, ctx)
	}
	dest := src
	if colon {
		dest = dst
	}
	return destDenied(dest, ctx)
}

func splitRefspec(spec string) (src, dst string, colon bool) {
	i := strings.IndexByte(spec, ':')
	if i < 0 {
		return spec, "", false
	}
	return spec[:i], spec[i+1:], true
}

func destDenied(name string, ctx PushContext) bool {
	if name == "" {
		return true
	}
	if strings.HasPrefix(name, "refs/tags/") {
		return true
	}
	name = strings.TrimPrefix(name, "refs/heads/")
	if ctx.Tags[name] {
		return true
	}
	if name == "HEAD" {
		return ctx.Current == "" || protectedBranch(ctx.Current, ctx)
	}
	return protectedBranch(name, ctx)
}

func defaultPushDenied(ctx PushContext) bool {
	if ctx.Current == "" || protectedBranch(ctx.Current, ctx) {
		return true
	}
	if ctx.Upstream != "" && protectedBranch(ctx.Upstream, ctx) {
		return true
	}
	return false
}

func protectedBranch(name string, ctx PushContext) bool {
	if strings.EqualFold(name, "main") || strings.EqualFold(name, "master") {
		return true
	}
	return ctx.Default != "" && strings.EqualFold(name, ctx.Default)
}
