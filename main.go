package main

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/richardwilkes/atexit"
	"github.com/richardwilkes/cmdline"
)

func main() {
	cmdline.AppName = "Dirt"
	cmdline.AppVersion = "1.0"
	cmdline.CopyrightYears = "2017"
	cmdline.CopyrightHolder = "Richard A. Wilkes"

	timeout := 5 * time.Minute
	fastOnly := false
	onlyOne := false

	var buffer bytes.Buffer
	buffer.WriteString(`Run linting checks against Go code. Two groups of linters are executed, a "fast" group and a "slow" group. The fast group consists of `)
	for i, one := range FastLinters {
		if i != 0 {
			buffer.WriteString(", ")
		}
		buffer.WriteString(linterName(one))
	}
	buffer.WriteString(`. The slow group consists of `)
	for i, one := range SlowLinters {
		if i != 0 {
			buffer.WriteString(", ")
		}
		buffer.WriteString(linterName(one))
	}
	buffer.WriteByte('.')

	cl := cmdline.New(true)
	cl.Description = buffer.String()
	cl.NewDurationOption(&timeout).SetSingle('t').SetName("timeout").SetArg("duration").SetUsage("Sets the timeout. If the linters run longer than this, they will be terminated and an error will be returned")
	cl.NewBoolOption(&fastOnly).SetSingle('f').SetName("fastonly").SetUsage("When set, only the fast linters are run")
	cl.NewBoolOption(&onlyOne).SetSingle('1').SetName("one").SetUsage("When set, only the last started invocation for the repo will complete; any others will be terminated")
	cl.Parse(os.Args[1:])

	l, err := newLint(".", linters(fastOnly))
	if err != nil {
		fmt.Println(err)
		atexit.Exit(1)
	}

	if onlyOne {
		for {
			lockFile := filepath.Join(l.repoPath, ".dirtlock")
			if f, err := os.OpenFile(lockFile, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0666); err == nil {
				fmt.Fprint(f, os.Getpid())
				if err = f.Close(); err != nil {
					fmt.Printf("Unable to write lock file: %s\n", lockFile)
					atexit.Exit(1)
				}
				atexit.Register(func() {
					if os.Remove(lockFile) != nil {
						fmt.Printf("Unable to remove lock file: %s\n", lockFile)
					}
				})
				break
			} else {
				if data, err := ioutil.ReadFile(lockFile); err == nil {
					if pid, err := strconv.Atoi(string(data)); err == nil {
						if p, err := os.FindProcess(pid); err == nil {
							p.Kill()
						}
						os.Remove(lockFile)
					}
				}
			}
		}
	}

	atexit.Exit(l.run(timeout))
}
