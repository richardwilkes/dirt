package main

import "os"

func kill(process *os.Process) error {
	return process.Kill()
}
