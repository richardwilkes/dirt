package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/nightlyone/lockfile"
	"github.com/richardwilkes/toolbox/atexit"
	"github.com/richardwilkes/toolbox/cmdline"
)

func main() {
	cmdline.AppName = "Dirt"
	cmdline.AppVersion = "1.0"
	cmdline.CopyrightYears = "2017"
	cmdline.CopyrightHolder = "Richard A. Wilkes"

	timeout := 5 * time.Minute
	fastOnly := false
	onlyOne := false
	forceInstall := false
	parallel := false
	var disallowedImports []string
	var disallowedFunctions []string

	var buffer strings.Builder
	buffer.WriteString(`Run linting checks against Go code. Two groups of linters are executed, a "fast" group and a "slow" group. The fast group consists of `)
	for i, one := range FastLinters {
		if i != 0 {
			buffer.WriteString(", ")
		}
		buffer.WriteString(one.Name())
	}
	buffer.WriteString(`. The slow group consists of `)
	for i, one := range SlowLinters {
		if i != 0 {
			buffer.WriteString(", ")
		}
		buffer.WriteString(one.Name())
	}
	buffer.WriteByte('.')

	cl := cmdline.New(true)
	cl.Description = buffer.String()
	cl.NewDurationOption(&timeout).SetSingle('t').SetName("timeout").SetArg("duration").SetUsage("Sets the timeout. If the linters run longer than this, they will be terminated and an error will be returned")
	cl.NewBoolOption(&fastOnly).SetSingle('f').SetName("fast-only").SetUsage("When set, only the fast linters are run")
	cl.NewBoolOption(&onlyOne).SetSingle('1').SetName("one").SetUsage("When set, only the last started invocation for the repo will complete; any others will be terminated")
	cl.NewBoolOption(&forceInstall).SetSingle('F').SetName("force-install").SetUsage("When set, the linters will be reinstalled, then the process will exit")
	cl.NewStringArrayOption(&disallowedImports).SetSingle('i').SetName("disallow-import").SetArg("import").SetUsage("Treat use of the specified import as an error. May be specified multiple times")
	cl.NewStringArrayOption(&disallowedFunctions).SetSingle('d').SetName("disallow-function").SetArg("function").SetUsage("Treat use of the specified function as an error. May be specified multiple times")
	cl.NewBoolOption(&parallel).SetSingle('p').SetName("parallel").SetUsage("When set, run the linters in parallel")
	cl.Parse(os.Args[1:])

	selected := selectLinters(fastOnly)
	for _, one := range selected {
		one.Install(forceInstall)
	}
	if forceInstall {
		atexit.Exit(0)
	}

	l, err := newLint(".", selected, disallowedImports, disallowedFunctions, parallel)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		atexit.Exit(1)
	}

	go monitorRunningCmds()
	if onlyOne {
		path := filepath.Join(l.repoPath, ".dirtlock")
		lf, err := lockfile.New(path)
		if err != nil {
			fmt.Fprintln(os.Stderr, "Unable to obtain lock file: ", path)
			atexit.Exit(1)
		}
		for lf.TryLock() != nil {
			if p, err := lf.GetOwner(); err == nil {
				ignoreError(syscall.Kill(p.Pid, syscall.SIGINT))
			}
			time.Sleep(100 * time.Millisecond)
		}
		atexit.Register(func() {
			done := make(chan bool)
			killRunningCmdsChan <- done
			<-done
			ignoreError(lf.Unlock())
		})
	}

	atexit.Exit(l.run(timeout))
}

func ignoreError(err error) {
}
