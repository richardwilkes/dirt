package main

import (
	"fmt"
	"os"
	"os/exec"

	"github.com/richardwilkes/toolbox/atexit"
)

// Argument substitution constants
const (
	REPO  = "@repo"
	PKGS  = "@pkgs"
	DIRS  = "@dirs"
	FILES = "@files"
)

var (
	// FastLinters holds the linters that are known to execute quickly.
	FastLinters = []linter{
		{cmd: "gofmt", args: []string{"-l", "-s", FILES}},
		{cmd: "goimports", args: []string{"-l", FILES}, pkg: "golang.org/x/tools/cmd/goimports"},
		{cmd: "golint", args: []string{PKGS}, pkg: "golang.org/x/lint/golint"},
		{cmd: "ineffassign", args: []string{REPO}, pkg: "github.com/gordonklaus/ineffassign"},
		{cmd: "misspell", args: []string{"-locale", "US", FILES}, pkg: "github.com/client9/misspell/cmd/misspell"},
		{cmd: "go", args: []string{"tool", "vet", "-all", "-shadow", DIRS}},
	}
	// SlowLinters holds the linters that are known to execute slowly.
	SlowLinters = []linter{
		{cmd: "staticcheck", args: []string{"-checks", "all,-ST1000,-ST1005", PKGS}, pkg: "honnef.co/go/tools/cmd/staticcheck"},
		{cmd: "errcheck", args: []string{"-abspath", "-blank", "-asserts", "-ignore", "github.com/richardwilkes/errs:Append", "-ignore", "github.com/richardwilkes/toolbox/errs:Append", "-ignore", "io:CloseWithError", PKGS}, pkg: "github.com/kisielk/errcheck"},
		{cmd: "unconvert", args: []string{PKGS}, pkg: "github.com/mdempsky/unconvert"},
	}
)

type linter struct {
	cmd  string
	args []string
	pkg  string
}

func (lntr *linter) Name() string {
	if lntr.cmd == "go" && len(lntr.args) > 1 {
		return lntr.args[1]
	}
	return lntr.cmd
}

func (lntr *linter) Install(force bool) {
	if lntr.pkg != "" {
		if !force {
			if _, err := exec.LookPath(lntr.cmd); err == nil {
				// command already exists, so bail
				return
			}
		}
		cmd := exec.Command("go", "get", "-u", lntr.pkg)
		if data, err := cmd.CombinedOutput(); err != nil {
			fmt.Fprintln(os.Stderr, "Unable to install", lntr.Name())
			fmt.Fprintln(os.Stderr, string(data))
			atexit.Exit(1)
		}
		fmt.Println("Installed", lntr.Name())
	}
}

func selectLinters(fastOnly bool) []linter {
	var list []linter
	if fastOnly {
		list = FastLinters
	} else {
		list = make([]linter, len(FastLinters)+len(SlowLinters))
		copy(list, FastLinters)
		copy(list[len(FastLinters):], SlowLinters)
	}
	return list
}
