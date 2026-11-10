//go:build windows

package main

import "os/exec"

func openFile(path string) error {
	return exec.Command("rundll32", "url.dll,FileProtocolHandler", path).Start()
}
