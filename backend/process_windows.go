package main

import (
	"os/exec"
	"strconv"
	"syscall"
)

func prepareProcess(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
}

func terminateProcess(cmd *exec.Cmd) error {
	// A Windows venv python.exe is a launcher with a child Python process.
	// Stop that owned process tree so a timed-out inference cannot keep the GPU.
	stop := exec.Command("taskkill.exe", "/PID", strconv.Itoa(cmd.Process.Pid), "/T", "/F")
	prepareProcess(stop)
	if err := stop.Run(); err != nil {
		return cmd.Process.Kill()
	}
	return nil
}
