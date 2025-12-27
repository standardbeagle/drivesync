//go:build !windows

package main

import (
	"os"
	"os/exec"
	"syscall"
)

// isAdmin checks if running as root
func isAdmin() bool {
	return os.Geteuid() == 0
}

// elevate re-runs the program with sudo
func elevate() error {
	sudo, err := exec.LookPath("sudo")
	if err != nil {
		return err
	}

	args := append([]string{sudo}, os.Args...)
	return syscall.Exec(sudo, args, os.Environ())
}

// getElevatePrompt returns the prompt for elevation
func getElevatePrompt() string {
	return "This program requires root privileges.\nPress Enter to restart with sudo, or Q to quit."
}
