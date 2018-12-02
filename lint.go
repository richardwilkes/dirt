package main

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/richardwilkes/toolbox/errs"
	"github.com/richardwilkes/toolbox/taskqueue"
)

type lint struct {
	origPath            string
	repoPath            string
	pkgs                []string
	dirs                []string
	files               []string
	linters             []linter
	disallowedImports   []string
	disallowedFunctions []string
	status              int32
	lineChan            chan problem
	doneChan            chan bool
	parallel            bool
}

type problem struct {
	prefix string
	output string
}

func newLint(lintersToRun []linter, disallowedImports, disallowedFunctions []string, parallel bool) (*lint, error) {
	l := &lint{
		linters:             lintersToRun,
		disallowedImports:   disallowedImports,
		disallowedFunctions: disallowedFunctions,
		lineChan:            make(chan problem, 16),
		doneChan:            make(chan bool),
		parallel:            parallel,
	}
	var err error
	if l.origPath, err = filepath.Abs("."); err != nil {
		return nil, errs.Wrap(err)
	}
	l.repoPath = findRoot(".")
	if err = os.Chdir(l.repoPath); err != nil {
		return nil, errs.Wrap(err)
	}
	if err = l.collectPackagesAndFiles(); err != nil {
		return nil, err
	}
	return l, nil
}

func (l *lint) run(timeout time.Duration) int {
	go l.parseLines()
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	if len(l.disallowedImports) > 0 || len(l.disallowedFunctions) > 0 {
		l.checkDisallowed()
	}
	if l.parallel {
		queue := taskqueue.New(taskqueue.Workers(runtime.NumCPU()), taskqueue.Log(l.logger))
		for _, one := range l.linters {
			queue.Submit(l.workFunc(ctx, one))
		}
		queue.Shutdown()
	} else {
		for _, one := range l.linters {
			l.execLinter(ctx, one)
			if ctx.Err() == context.DeadlineExceeded {
				break
			}
		}
	}
	close(l.lineChan)
	<-l.doneChan
	if l.status != 0 && ctx.Err() == context.DeadlineExceeded {
		fmt.Fprintln(os.Stderr, "*** Timeout exceeded ***")
	}
	return int(l.status)
}

func (l *lint) logger(v ...interface{}) {
	fmt.Fprintln(os.Stderr, v...)
}

func (l *lint) workFunc(ctx context.Context, lntr linter) func() {
	return func() {
		l.execLinter(ctx, lntr)
	}
}

func (l *lint) collectPackagesAndFiles() error {
	var err error
	l.pkgs, err = listPackages()
	if err != nil {
		return err
	}
	l.dirs, err = listDirs()
	if err != nil {
		return err
	}
	l.files, err = listFiles()
	if err != nil {
		return err
	}
	if len(l.files) == 0 {
		return fmt.Errorf("No files to process")
	}
	return nil
}

func (l *lint) argSubstitution(lntr linter) []string {
	result := make([]string, 0, len(lntr.args))
	for _, arg := range lntr.args {
		switch arg {
		case REPO:
			result = append(result, l.repoPath)
		case FILES:
			result = append(result, l.files...)
		case DIRS:
			result = append(result, l.dirs...)
		case PKGS:
			result = append(result, l.pkgs...)
		default:
			result = append(result, arg)
		}
	}
	return result
}

func (l *lint) parseLines() {
	for line := range l.lineChan {
		l.processLine(line)
	}
	l.doneChan <- true
}

func (l *lint) execLinter(ctx context.Context, lntr linter) {
	prefix := lntr.Name()
	cc := exec.CommandContext(ctx, lntr.cmd, l.argSubstitution(lntr)...)

	stdout, err := cc.StdoutPipe()
	if err != nil {
		l.lineChan <- problem{prefix: prefix, output: err.Error()}
		return
	}
	stderr, err := cc.StderrPipe()
	if err != nil {
		l.lineChan <- problem{prefix: prefix, output: err.Error()}
		return
	}

	var wg sync.WaitGroup
	wg.Add(1)
	go l.scanOutput(prefix, stdout, &wg)
	wg.Add(1)
	go l.scanOutput(prefix, stderr, &wg)

	addRunningCmdChan <- cc
	defer func() {
		removeRunningCmdChan <- cc
	}()
	if err = cc.Start(); err != nil {
		l.lineChan <- problem{prefix: prefix, output: err.Error()}
		return
	}
	cc.Wait() // @allow
	wg.Wait()
}

func (l *lint) scanOutput(prefix string, r io.Reader, wg *sync.WaitGroup) {
	defer wg.Done()
	scanner := bufio.NewScanner(r)
	for scanner.Scan() {
		l.lineChan <- problem{prefix: prefix, output: scanner.Text()}
	}
}

func (l *lint) processLine(line problem) {
	output := strings.TrimSpace(line.output)
	if output == "" {
		return
	}
	if strings.HasPrefix(output, "vendor") {
		return
	}
	if strings.Contains(output, "@allow") {
		return
	}
	if strings.Contains(output, "(SA3000)") {
		return
	}
	if strings.Contains(output, ".pb.go") {
		return
	}
	if strings.Contains(output, "mock_grpc") {
		return
	}
	if strings.Contains(output, "couldn't load packages due to errors:") {
		return
	}
	if strings.HasPrefix(output, line.prefix+": ") {
		output = output[len(line.prefix)+2:]
	}
	if filepath.IsAbs(output) {
		if replacement, err := filepath.Rel(l.origPath, output); err == nil {
			output = replacement
		}
	}
	if !strings.Contains(output, ":") {
		output += ":1:1:"
	}
	fmt.Fprintf(os.Stderr, "%s [%s]\n", output, line.prefix)
	l.markError()
}

func (l *lint) markError() {
	atomic.StoreInt32(&l.status, 1)
}
