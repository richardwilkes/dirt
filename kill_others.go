// +build !windows

package main

import (
	"os"
	"syscall"
)

func kill(process *os.Process) error {
	return syscall.Kill(process.Pid, syscall.SIGINT) // @allow
}
