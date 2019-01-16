package main

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/nightlyone/lockfile"
	"github.com/richardwilkes/toolbox/atexit"
	"github.com/richardwilkes/toolbox/cmdline"
)

func main() {
	cmdline.AppName = "Dirt"
	cmdline.AppVersion = "1.2.1"
	cmdline.CopyrightYears = "2017-2018"
	cmdline.CopyrightHolder = "Richard A. Wilkes"
	cmdline.AppIdentifier = "com.trollworks.dirt"

	timeout := 5 * time.Minute
	fastOnly := false
	onlyOne := false
	forceInstall := false
	archive := false
	var installFrom string
	parallel := false
	dryRun := false
	goos := runtime.GOOS
	goarch := runtime.GOARCH
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
	cl.NewStringOption(&installFrom).SetName("install-from-archive").SetArg("url or path").SetUsage("When set, the linters will be installed by extracting them from the specified archive instead of building it from source, then the process will exit")
	cl.NewBoolOption(&archive).SetName("archive").SetUsage("When set, creates an archive containing the linters that can be used later with --install-from-archive, then exits")
	cl.NewStringOption(&goos).SetName("os").SetUsage("The GOOS value to use with the --archive option")
	cl.NewStringOption(&goarch).SetName("arch").SetUsage("The GOARCH value to use with the --archive option")
	cl.NewStringArrayOption(&disallowedImports).SetSingle('i').SetName("disallow-import").SetArg("import").SetUsage("Treat use of the specified import as an error. May be specified multiple times")
	cl.NewStringArrayOption(&disallowedFunctions).SetSingle('d').SetName("disallow-function").SetArg("function").SetUsage("Treat use of the specified function as an error. May be specified multiple times")
	cl.NewBoolOption(&parallel).SetSingle('p').SetName("parallel").SetUsage("When set, run the linters in parallel")
	cl.NewBoolOption(&dryRun).SetSingle('n').SetName("dry-run").SetUsage("When set, just print the commands that would be issued and then exit")
	cl.Parse(os.Args[1:])

	if archive {
		if err := Archive(goos, goarch); err != nil {
			fmt.Fprintln(os.Stderr, err)
			atexit.Exit(1)
		}
		atexit.Exit(0)
	}

	if installFrom != "" {
		if err := InstallFromArchive(installFrom); err != nil {
			fmt.Fprintln(os.Stderr, err)
			atexit.Exit(1)
		}
		atexit.Exit(0)
	}

	selected := selectLinters(fastOnly)
	for _, one := range selected {
		one.Install(forceInstall)
	}
	if forceInstall {
		atexit.Exit(0)
	}

	l, err := newLint(selected, disallowedImports, disallowedFunctions, parallel, dryRun)
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
				kill(p) // @allow
			}
			time.Sleep(100 * time.Millisecond)
		}
		atexit.Register(func() {
			done := make(chan bool)
			killRunningCmdsChan <- done
			<-done
			lf.Unlock() // @allow
		})
	}

	atexit.Exit(l.run(timeout))
}
