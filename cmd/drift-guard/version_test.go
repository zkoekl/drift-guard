package main

import (
	"bytes"
	"os"
	"testing"
)

func TestVersionFlags(t *testing.T) {
	if version != "dev" {
		t.Fatalf("default version = %q, want dev", version)
	}
	for _, arg := range []string{"--version", "-v"} {
		t.Run(arg, func(t *testing.T) {
			code, out := captureStdout(t, []string{arg})
			if code != 0 {
				t.Fatalf("exit %d, output %q", code, out)
			}
			if out != version+"\n" {
				t.Fatalf("output %q, want %q", out, version+"\n")
			}
		})
	}
}

func captureStdout(t *testing.T, args []string) (int, string) {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	old := os.Stdout
	os.Stdout = w
	defer func() { os.Stdout = old }()
	code := run(args)
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	if _, err := buf.ReadFrom(r); err != nil {
		t.Fatal(err)
	}
	return code, buf.String()
}
