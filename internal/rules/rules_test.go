package rules

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestIsGitPush(t *testing.T) {
	yes := []string{
		"git push",
		"git push origin main",
		"  git   push  ",
		"GIT PUSH",
		"sudo git push",
		"FOO=bar git push",
		"git -C /tmp/repo push",
		"git --git-dir=/tmp/x push",
		"git status && git push",
		"cd repo; git push",
		"git pull | git push",
		`sh -c "git push"`,
		"bash -lc 'git push origin'",
	}
	no := []string{
		"",
		"git status",
		"git pull",
		"git commit -m 'please git push later'",
		"echo git push",
		"echo \"git push\"",
		"notgit push",
	}
	for _, c := range yes {
		if !IsGitPush(c) {
			t.Errorf("expected push: %q", c)
		}
	}
	for _, c := range no {
		if IsGitPush(c) {
			t.Errorf("expected no push: %q", c)
		}
	}
}

func TestDoneWithoutTests(t *testing.T) {
	py := []string{"**/*.py"}
	changed := []string{"src/learn.py"}
	if !DoneWithoutTests("pytest", py, changed, nil) {
		t.Fatal("missing pytest should fire")
	}
	ok := []Command{{Command: "pytest -q", ExitCode: 0}}
	if DoneWithoutTests("pytest", py, changed, ok) {
		t.Fatal("successful pytest should pass")
	}
	failed := []Command{{Command: "pytest -q", ExitCode: 1}}
	if !DoneWithoutTests("pytest", py, changed, failed) {
		t.Fatal("failed pytest should fire")
	}
	if DoneWithoutTests("", py, changed, nil) {
		t.Fatal("unnamed test command must not fire")
	}
	if DoneWithoutTests("pytest", py, nil, nil) {
		t.Fatal("no changed path must not fire")
	}
	if DoneWithoutTests("pytest", []string{"gameplay/**"}, changed, nil) {
		t.Fatal("non-matching glob must not fire")
	}
}

func TestReplyLanguage(t *testing.T) {
	english := "This report covers three numbered rounds. Each round is written in English and describes the series."
	russian := "Этот отчёт описывает три раунда одной серии и написан на русском языке полностью."
	if !ReplyLanguageViolation("ru", english) {
		t.Fatal("English under a Russian rule should fire")
	}
	if ReplyLanguageViolation("ru", russian) {
		t.Fatal("Russian under a Russian rule should not fire")
	}
	if !ReplyLanguageViolation("en", russian) {
		t.Fatal("Russian under an English rule should fire")
	}
	if ReplyLanguageViolation("en", english) {
		t.Fatal("English under an English rule should not fire")
	}
	if ReplyLanguageViolation("", english) || ReplyLanguageViolation("fr", english) {
		t.Fatal("unnamed or unsupported language must not fire")
	}
	if ReplyLanguageViolation("ru", "too short") {
		t.Fatal("short text must not fire")
	}
}

func TestMatchGlob(t *testing.T) {
	cases := []struct {
		pat, name string
		want      bool
	}{
		{"**/*.py", "src/learn.py", true},
		{"gameplay/**", "gameplay/loop.js", true},
		{"gameplay/**", ".github/workflows/build.yml", false},
		{"cmd/**", "cmd/drift-guard/main.go", true},
		{".gitignore", ".gitignore", true},
		{"docs/**", "docs/hooks/README.md", true},
	}
	for _, tc := range cases {
		if got := MatchGlob(tc.pat, tc.name); got != tc.want {
			t.Errorf("MatchGlob(%q, %q)=%v want %v", tc.pat, tc.name, got, tc.want)
		}
	}
}

func TestBenchVectors(t *testing.T) {
	path := findVectors(t)
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var vectors []struct {
		Name         string    `json:"name"`
		Command      string    `json:"command"`
		TestCommand  string    `json:"test_command"`
		Globs        []string  `json:"globs"`
		ChangedPaths []string  `json:"changed_paths"`
		Commands     []Command `json:"commands"`
		WantPush     bool      `json:"want_push"`
		WantDone     bool      `json:"want_done"`
	}
	if err := json.Unmarshal(b, &vectors); err != nil {
		t.Fatal(err)
	}
	if len(vectors) < 8 {
		t.Fatalf("expected at least 8 vectors, got %d", len(vectors))
	}
	for _, v := range vectors {
		if got := IsGitPush(v.Command); got != v.WantPush {
			t.Errorf("%s push got %v", v.Name, got)
		}
		if got := DoneWithoutTests(v.TestCommand, v.Globs, v.ChangedPaths, v.Commands); got != v.WantDone {
			t.Errorf("%s done got %v", v.Name, got)
		}
	}
}

func findVectors(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for {
		cand := filepath.Join(dir, "bench", "vectors.json")
		if _, err := os.Stat(cand); err == nil {
			return cand
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("bench/vectors.json not found")
		}
		dir = parent
	}
}
