//go:build !windows

package main

import "os/exec"

func prepareProcess(cmd *exec.Cmd) {}

func terminateProcess(cmd *exec.Cmd) error {
	return cmd.Process.Kill()
}
