package main

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/nightlyone/lockfile"
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
	forceInstall := false
	var disallowedImports []string
	var disallowedFunctions []string

	var buffer bytes.Buffer
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
	cl.NewBoolOption(&forceInstall).SetSingle('F').SetName("force-install").SetUsage("When set, the linters will be reinstalled")
	cl.NewStringArrayOption(&disallowedImports).SetSingle('i').SetName("disallow-import").SetArg("import").SetUsage("Treat use of the specified import as an error. May be specified multiple times")
	cl.NewStringArrayOption(&disallowedFunctions).SetSingle('d').SetName("disallow-function").SetArg("function").SetUsage("Treat use of the specified function as an error. May be specified multiple times")
	cl.Parse(os.Args[1:])

	selected := selectLinters(fastOnly)
	if forceInstall {
		for _, one := range selected {
			one.Install(true)
		}
	}
	l, err := newLint(".", selected, disallowedImports, disallowedFunctions)
	if err != nil {
		fmt.Println(err)
		atexit.Exit(1)
	}

	if onlyOne {
		path := filepath.Join(l.repoPath, ".dirtlock")
		lf, err := lockfile.New(path)
		if err != nil {
			fmt.Println("Unable to obtain lock file: ", path)
			atexit.Exit(1)
		}
		for lf.TryLock() != nil {
			if p, err := lf.GetOwner(); err == nil {
				p.Kill()
			}
		}
		atexit.Register(func() { lf.Unlock() })
	}

	atexit.Exit(l.run(timeout))
}
