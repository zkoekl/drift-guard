package drift

import (
	"bufio"
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/zkoekl/drift-guard/internal/rules"
)

// ResolveGlobs loads a scope list. An explicit scope file wins, then a pull
// request body, then plan.md front matter, then .drift-guard-scope.
func ResolveGlobs(dir, scopeFile, prBody string) ([]string, string) {
	if scopeFile != "" {
		return readGlobFile(scopeFile), "scope file"
	}
	if g := globsFromBody(prBody); len(g) > 0 {
		return g, "pull request body"
	}
	if g := globsFromPlan(filepath.Join(dir, "plan.md")); len(g) > 0 {
		return g, "plan.md"
	}
	alt := filepath.Join(dir, ".drift-guard-scope")
	if st, err := os.Stat(alt); err == nil && !st.IsDir() {
		return readGlobFile(alt), ".drift-guard-scope"
	}
	return nil, ""
}

// Outside returns changed files that match none of the globs.
func Outside(files, globs []string) []string {
	var out []string
	for _, file := range files {
		file = strings.TrimSpace(filepath.ToSlash(file))
		if file == "" {
			continue
		}
		matched := false
		for _, g := range globs {
			if rules.MatchGlob(strings.TrimSpace(g), file) {
				matched = true
				break
			}
		}
		if !matched {
			out = append(out, file)
		}
	}
	return out
}

func readGlobFile(path string) []string {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	return parseGlobLines(string(b))
}

func parseGlobLines(text string) []string {
	var out []string
	sc := bufio.NewScanner(strings.NewReader(text))
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if strings.HasPrefix(line, "- ") {
			line = strings.TrimSpace(line[2:])
		} else if strings.HasPrefix(line, "* ") {
			line = strings.TrimSpace(line[2:])
		}
		line = strings.Trim(line, "`\"'")
		if line != "" {
			out = append(out, line)
		}
	}
	return out
}

func globsFromBody(body string) []string {
	const start = "<!-- drift-guard-scope:start -->"
	const end = "<!-- drift-guard-scope:end -->"
	i := strings.Index(body, start)
	if i < 0 {
		return nil
	}
	rest := body[i+len(start):]
	j := strings.Index(rest, end)
	if j < 0 {
		return nil
	}
	return parseGlobLines(rest[:j])
}

func globsFromPlan(path string) []string {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	s := string(b)
	if !strings.HasPrefix(s, "---\n") {
		return nil
	}
	rest := s[len("---\n"):]
	end := strings.Index(rest, "\n---")
	if end < 0 {
		return nil
	}
	return globsUnderKey(rest[:end], "scope")
}

func globsUnderKey(front, key string) []string {
	var out []string
	in := false
	for _, line := range strings.Split(front, "\n") {
		trim := strings.TrimSpace(line)
		if !in {
			if trim == key+":" {
				in = true
			}
			continue
		}
		if trim == "" {
			continue
		}
		if strings.HasPrefix(trim, "- ") {
			item := strings.Trim(strings.TrimSpace(strings.TrimPrefix(trim, "- ")), `"'`)
			if item != "" {
				out = append(out, item)
			}
			continue
		}
		break
	}
	return out
}

// GitChanged lists files from `git diff`. staged uses the index.
// Otherwise the range is base...head.
func GitChanged(dir, base, head string, staged bool) ([]string, string, error) {
	args := []string{"-C", dir, "diff", "--name-only", "--diff-filter=ACMR"}
	if staged {
		args = append(args, "--cached")
	} else {
		args = append(args, base+"..."+head)
	}
	cmd := exec.Command("git", args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = err.Error()
		}
		return nil, msg, err
	}
	var files []string
	sc := bufio.NewScanner(&stdout)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line != "" {
			files = append(files, line)
		}
	}
	return files, "", sc.Err()
}

// ReadPathList reads newline-separated paths. "-" reads standard input from r.
func ReadPathList(path string, stdin []byte) ([]string, error) {
	var b []byte
	var err error
	if path == "-" {
		b = stdin
	} else {
		b, err = os.ReadFile(path)
		if err != nil {
			return nil, err
		}
	}
	var files []string
	sc := bufio.NewScanner(bytes.NewReader(b))
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line != "" {
			files = append(files, line)
		}
	}
	return files, sc.Err()
}
