// Command hookbench is the minimal hook timed in docs/bench.md.
// It reads one JSON object, runs the git-push rule and the done-without-tests
// rule, appends one JSON line, and writes one JSON object to stdout.
package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/zkoekl/drift-guard/internal/rules"
)

type input struct {
	Command      string          `json:"command"`
	TestCommand  string          `json:"test_command"`
	Globs        []string        `json:"globs"`
	ChangedPaths []string        `json:"changed_paths"`
	Commands     []rules.Command `json:"commands"`
}

type vector struct {
	input
	Name     string `json:"name"`
	WantPush bool   `json:"want_push"`
	WantDone bool   `json:"want_done"`
}

func main() {
	if len(os.Args) > 1 && os.Args[1] == "--self-test" {
		if len(os.Args) != 3 {
			fmt.Fprintln(os.Stderr, "usage: hookbench --self-test bench/vectors.json")
			os.Exit(2)
		}
		os.Exit(selfTest(os.Args[2]))
	}
	raw, err := io.ReadAll(os.Stdin)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	var in input
	if err := json.Unmarshal(raw, &in); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	push, done := eval(in)
	if err := appendEvent(push, done); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	enc := json.NewEncoder(os.Stdout)
	out := map[string]any{
		"permission":         "allow",
		"done_without_tests": done,
	}
	if push {
		out["permission"] = "deny"
		out["user_message"] = "git push is denied"
		out["agent_message"] = "git push is denied"
	}
	if err := enc.Encode(out); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func eval(in input) (bool, bool) {
	return rules.IsGitPush(in.Command), rules.DoneWithoutTests(in.TestCommand, in.Globs, in.ChangedPaths, in.Commands)
}

func appendEvent(push, done bool) error {
	path := os.Getenv("DRIFT_GUARD_LOG")
	if path == "" {
		path = "events.jsonl"
	}
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	rec := map[string]any{
		"ts":                 time.Now().UTC().Format(time.RFC3339Nano),
		"kind":               "verdict",
		"push":               push,
		"done_without_tests": done,
	}
	b, err := json.Marshal(rec)
	if err != nil {
		return err
	}
	if _, err := f.Write(append(b, '\n')); err != nil {
		return err
	}
	return f.Sync()
}

func selfTest(path string) int {
	b, err := os.ReadFile(path)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	var vectors []vector
	if err := json.Unmarshal(b, &vectors); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	for _, v := range vectors {
		push, done := eval(v.input)
		if push != v.WantPush || done != v.WantDone {
			fmt.Fprintf(os.Stderr, "fail: %s push=%v done=%v\n", v.Name, push, done)
			return 1
		}
	}
	fmt.Printf("ok %d\n", len(vectors))
	return 0
}
