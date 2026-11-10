package main

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func TestScanAcceptsLogFlagBeforeOrAfterPath(t *testing.T) {
	dir := t.TempDir()
	transcript := filepath.Join(dir, "transcript.json")
	body := []byte(`{"origin":"synthetic","session_id":"order","assistant_text":"ok","messages":[{"role":"assistant","text":"ok"}]}`)
	if err := os.WriteFile(transcript, body, 0o644); err != nil {
		t.Fatal(err)
	}
	orders := []struct {
		name string
		args []string
		log  string
	}{
		{
			name: "flag before path",
			args: []string{"scan", "--log", filepath.Join(dir, "before.jsonl"), transcript},
			log:  filepath.Join(dir, "before.jsonl"),
		},
		{
			name: "flag after path",
			args: []string{"scan", transcript, "--log", filepath.Join(dir, "after.jsonl")},
			log:  filepath.Join(dir, "after.jsonl"),
		},
	}
	for _, tc := range orders {
		t.Run(tc.name, func(t *testing.T) {
			code := run(tc.args)
			if code != 0 {
				t.Fatalf("exit %d", code)
			}
			got, err := os.ReadFile(tc.log)
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Contains(got, []byte(`"source":"scan"`)) {
				t.Fatalf("log %s", got)
			}
		})
	}
}
