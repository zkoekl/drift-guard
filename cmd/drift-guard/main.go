package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/zkoekl/drift-guard/internal/drift"
)

// version is the build version. Set with -ldflags "-X main.version=...".
var version = "dev"

func main() {
	os.Exit(run(os.Args[1:]))
}

func run(args []string) int {
	if len(args) == 0 {
		usage()
		return 2
	}
	switch args[0] {
	case "-v", "--version":
		fmt.Println(version)
		return 0
	case "hook":
		return cmdHook()
	case "check":
		return cmdCheck(args[1:])
	case "scan":
		return cmdScan(args[1:])
	case "report":
		return cmdReport(args[1:])
	case "scope":
		return cmdScope(args[1:])
	case "canary":
		return cmdCanary(args[1:])
	case "-h", "--help", "help":
		usage()
		return 0
	default:
		usage()
		return 2
	}
}

func usage() {
	fmt.Fprintf(os.Stderr, "drift-guard %s\n\n", version)
	fmt.Fprint(os.Stderr, `  drift-guard --version
  drift-guard hook
  drift-guard check --input file.json [--log events.jsonl]
  drift-guard scan [--log events.jsonl] <transcript>
  drift-guard scan <transcript> [--log events.jsonl]
  drift-guard report [--log events.jsonl] [--output file.html] [--no-open]
  drift-guard scope [--require-scope] [--scope-file f] [--files f|-] [--staged]
                    [--base ref --head ref]
  drift-guard canary refresh --session ID [--rules f] [--root dir]
                    [--secret-file f]

Exit 0 is clean, 1 is a violation, 2 is a usage error.
The stop hook exits 0 even when it asks the agent to continue.
Scan exits 0. Findings go to the log.
canary refresh exits 1 when a required file is missing. It does not print the code.
`)
}

func cmdCanary(args []string) int {
	if len(args) == 0 || args[0] != "refresh" {
		usage()
		return 2
	}
	fs := flag.NewFlagSet("canary refresh", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	session := fs.String("session", "", "session or conversation id")
	rulesPath := fs.String("rules", "", "rules json")
	root := fs.String("root", "", "project root")
	secretFile := fs.String("secret-file", "", "local secret file")
	if err := fs.Parse(args[1:]); err != nil || strings.TrimSpace(*session) == "" {
		usage()
		return 2
	}
	paths := drift.PathsFromEnv()
	if *rulesPath == "" {
		*rulesPath = paths.Rules
	}
	if *root == "" {
		*root = paths.Root
	}
	if *secretFile == "" {
		state := paths.State
		if state == "" {
			state = ".drift-guard"
		}
		*secretFile = filepath.Join(state, "canary-secret")
	}
	res, err := drift.RunCanaryRefresh(*root, *rulesPath, *secretFile, *session)
	if err != nil {
		fmt.Fprintln(os.Stderr, "canary refresh failed")
		return 1
	}
	if res.Off {
		fmt.Println("canary off")
		return 0
	}
	for _, path := range res.Updated {
		fmt.Printf("refreshed %s\n", path)
	}
	for _, path := range res.Missing {
		fmt.Printf("missing %s\n", path)
	}
	if len(res.Missing) > 0 {
		return 1
	}
	return 0
}

func cmdHook() int {
	raw, err := io.ReadAll(os.Stdin)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	out, code := drift.Handle(raw, drift.PathsFromEnv(), time.Now().UTC())
	if _, err := os.Stdout.Write(out); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	return code
}

func cmdCheck(args []string) int {
	fs := flag.NewFlagSet("check", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	input := fs.String("input", "", "fixture json")
	logPath := fs.String("log", "events.jsonl", "events.jsonl path")
	if err := fs.Parse(args); err != nil || *input == "" {
		usage()
		return 2
	}
	res, err := drift.CheckFile(*input, *logPath, time.Now().UTC())
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 2
	}
	enc := json.NewEncoder(os.Stdout)
	if err := enc.Encode(res); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 2
	}
	return res.Exit
}

func cmdScan(args []string) int {
	fs := flag.NewFlagSet("scan", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	logPath := fs.String("log", "events.jsonl", "events.jsonl path")
	// flag.FlagSet stops at the first positional, so `--log` after the
	// transcript path would otherwise be left unparsed.
	if err := fs.Parse(flagsBeforePositionals(args, map[string]bool{"log": true})); err != nil || fs.NArg() != 1 {
		usage()
		return 2
	}
	res, err := drift.ScanFile(fs.Arg(0), *logPath, time.Now().UTC())
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 2
	}
	enc := json.NewEncoder(os.Stdout)
	if err := enc.Encode(res); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 2
	}
	return res.Exit
}

func cmdReport(args []string) int {
	fs := flag.NewFlagSet("report", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	logPath := fs.String("log", "events.jsonl", "events.jsonl path")
	output := fs.String("output", "drift-guard-report.html", "html path")
	noOpen := fs.Bool("no-open", false, "do not open a browser")
	if err := fs.Parse(args); err != nil {
		usage()
		return 2
	}
	events, err := drift.LoadEvents(*logPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 2
	}
	page := drift.ReportHTML(events)
	if err := os.WriteFile(*output, []byte(page), 0o644); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 2
	}
	fmt.Println(*output)
	if !*noOpen && isTerminal(os.Stdout) {
		// Best effort. A missing opener is not a failure.
		_ = openFile(*output)
	}
	return 0
}

func cmdScope(args []string) int {
	fs := flag.NewFlagSet("scope", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	require := fs.Bool("require-scope", false, "exit 1 when no scope is declared")
	scopeFile := fs.String("scope-file", "", "glob file")
	files := fs.String("files", "", "newline-separated paths, or - for stdin")
	staged := fs.Bool("staged", false, "staged paths")
	base := fs.String("base", "", "diff base")
	head := fs.String("head", "", "diff head")
	if err := fs.Parse(args); err != nil {
		usage()
		return 2
	}
	dir, err := os.Getwd()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 2
	}
	var changed []string
	switch {
	case *files != "":
		var stdin []byte
		if *files == "-" {
			stdin, err = io.ReadAll(os.Stdin)
			if err != nil {
				fmt.Fprintln(os.Stderr, err)
				return 2
			}
		}
		changed, err = drift.ReadPathList(*files, stdin)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 2
		}
	case *staged:
		var msg string
		changed, msg, err = drift.GitChanged(dir, "", "", true)
		if err != nil {
			fmt.Fprintln(os.Stderr, msg)
			return 2
		}
	case *base != "" || *head != "":
		if *base == "" || *head == "" {
			fmt.Fprintln(os.Stderr, "scope: --base and --head are both required")
			return 2
		}
		var msg string
		changed, msg, err = drift.GitChanged(dir, *base, *head, false)
		if err != nil {
			fmt.Fprintln(os.Stderr, msg)
			return 2
		}
	default:
		usage()
		return 2
	}
	globs, source := drift.ResolveGlobs(dir, *scopeFile, os.Getenv("DRIFT_GUARD_PR_BODY"))
	if len(globs) == 0 {
		if *require {
			fmt.Fprintln(os.Stderr, "error: no scope declared")
			return 1
		}
		fmt.Fprintln(os.Stderr, "warning: no scope declared; scope check skipped")
		return 0
	}
	outside := drift.Outside(changed, globs)
	if len(outside) == 0 {
		fmt.Printf("scope ok (%s, %d files)\n", source, len(changed))
		return 0
	}
	fmt.Fprintf(os.Stderr, "scope violation (%s):\n", source)
	for _, path := range outside {
		fmt.Printf("outside: %s\n", path)
	}
	return 1
}

// flagsBeforePositionals moves flags in front of positionals.
// A name in takesValue consumes the following argument unless the flag is `--name=value`.
// `--` ends flag parsing; everything after it stays positional.
func flagsBeforePositionals(args []string, takesValue map[string]bool) []string {
	var flags, positional []string
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if arg == "--" {
			positional = append(positional, args[i+1:]...)
			break
		}
		name, inline := flagName(arg)
		if name == "" {
			positional = append(positional, arg)
			continue
		}
		flags = append(flags, arg)
		if !inline && takesValue[name] && i+1 < len(args) {
			i++
			flags = append(flags, args[i])
		}
	}
	return append(flags, positional...)
}

func flagName(arg string) (name string, inline bool) {
	if arg == "-" || !strings.HasPrefix(arg, "-") {
		return "", false
	}
	body := strings.TrimLeft(arg, "-")
	if body == "" {
		return "", false
	}
	if eq := strings.IndexByte(body, '='); eq >= 0 {
		return body[:eq], true
	}
	return body, false
}

func isTerminal(f *os.File) bool {
	st, err := f.Stat()
	if err != nil {
		return false
	}
	return st.Mode()&os.ModeCharDevice != 0
}
