package main

import "os/exec"

var addRunningCmdChan = make(chan *exec.Cmd)
var removeRunningCmdChan = make(chan *exec.Cmd)
var killRunningCmdsChan = make(chan chan bool)
var runningCmds = make(map[*exec.Cmd]*exec.Cmd)

func monitorRunningCmds() {
	for {
		select {
		case cmd := <-addRunningCmdChan:
			runningCmds[cmd] = cmd
		case cmd := <-removeRunningCmdChan:
			delete(runningCmds, cmd)
		case done := <-killRunningCmdsChan:
			for k := range runningCmds {
				ignoreError(k.Process.Kill())
			}
			done <- true
		}
	}
}
