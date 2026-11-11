package drift

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func initFeatureRepo(t *testing.T, root string) {
	t.Helper()
	git := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = root
		cmd.Env = append(os.Environ(),
			"GIT_AUTHOR_NAME=drift-guard",
			"GIT_AUTHOR_EMAIL=drift-guard@example.com",
			"GIT_COMMITTER_NAME=drift-guard",
			"GIT_COMMITTER_EMAIL=drift-guard@example.com",
		)
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	git("init")
	if err := os.WriteFile(filepath.Join(root, "README.md"), []byte("hello\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	git("add", "README.md")
	git("commit", "-m", "init")
	git("branch", "-M", "main")
	git("checkout", "-b", "feature")
}

func TestFeatureBranchPush(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	git := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = root
		cmd.Env = append(os.Environ(),
			"GIT_AUTHOR_NAME=drift-guard",
			"GIT_AUTHOR_EMAIL=drift-guard@example.com",
			"GIT_COMMITTER_NAME=drift-guard",
			"GIT_COMMITTER_EMAIL=drift-guard@example.com",
		)
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	git("init")
	git("branch", "-M", "main")
	if err := os.WriteFile(filepath.Join(root, "README.md"), []byte("hello\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	git("add", "README.md")
	git("commit", "-m", "init")
	git("checkout", "-b", "feature")
	git("update-ref", "refs/remotes/origin/main", "HEAD")
	git("symbolic-ref", "refs/remotes/origin/HEAD", "refs/remotes/origin/main")
	git("tag", "v1.0.0")

	p := testPaths(t)
	p.Root = root
	ctx := gitPushContext(root)
	if ctx.Current != "feature" || ctx.Default != "main" {
		t.Fatalf("context: %#v", ctx)
	}
	if !ctx.Tags["v1.0.0"] {
		t.Fatalf("tags: %#v", ctx.Tags)
	}
	allow := mustHandle(t, p, `{"hook_event_name":"beforeShellExecution","command":"git push -u origin HEAD","conversation_id":"feat"}`)
	if allow["permission"] != "allow" {
		t.Fatalf("feature push: %#v", allow)
	}
	named := mustHandle(t, p, `{"hook_event_name":"beforeShellExecution","command":"git push -u origin feature","conversation_id":"feat"}`)
	if named["permission"] != "allow" {
		t.Fatalf("named feature: %#v", named)
	}
	denied := []string{
		"git push origin main",
		"git push origin HEAD:main",
		"git push --force origin feature",
		"git push --force-with-lease origin feature",
		"git push origin --tags",
		"git push origin v1.0.0",
		"git push origin :main",
	}
	for _, cmd := range denied {
		got := mustHandle(t, p, `{"hook_event_name":"beforeShellExecution","command":`+jsonString(cmd)+`,"conversation_id":"feat"}`)
		if got["permission"] != "deny" {
			t.Fatalf("%s: %#v", cmd, got)
		}
	}

	git("remote", "add", "origin", root)
	git("config", "branch.feature.remote", "origin")
	git("config", "branch.feature.merge", "refs/heads/main")
	bare := mustHandle(t, p, `{"hook_event_name":"beforeShellExecution","command":"git push","conversation_id":"feat"}`)
	if bare["permission"] != "deny" {
		t.Fatalf("upstream main: %#v", bare)
	}
}

func TestCanaryRefreshSkipsGitignoredFile(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	git := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = root
		cmd.Env = append(os.Environ(),
			"GIT_AUTHOR_NAME=drift-guard",
			"GIT_AUTHOR_EMAIL=drift-guard@example.com",
			"GIT_COMMITTER_NAME=drift-guard",
			"GIT_COMMITTER_EMAIL=drift-guard@example.com",
		)
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	if err := os.WriteFile(filepath.Join(root, ".gitignore"), []byte(".drift-guard/\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	agents := filepath.Join(root, "AGENTS.md")
	if err := os.WriteFile(agents, []byte("# Rules\n\ndrift-guard-canary: pending\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	git("init")
	git("add", ".gitignore", "AGENTS.md")
	git("commit", "-m", "init")
	canary := filepath.Join(root, ".drift-guard", "canary.md")
	if err := os.MkdirAll(filepath.Dir(canary), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(canary, []byte("drift-guard-canary: pending\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := RefreshCanaries(root, []string{".drift-guard/canary.md"}, "dogfood-session", "00112233445566778899aabbccddeeff"); err != nil {
		t.Fatal(err)
	}
	body, err := os.ReadFile(agents)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(body), "drift-guard-canary: pending") {
		t.Fatal("tracked AGENTS.md changed")
	}
	canaryBody, err := os.ReadFile(canary)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(canaryBody), "pending") {
		t.Fatal("gitignored canary was not refreshed")
	}
	status := exec.Command("git", "status", "--porcelain")
	status.Dir = root
	out, err := status.Output()
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(string(out)) != "" {
		t.Fatalf("git status: %s", out)
	}
}
