package main

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/richardwilkes/errs"
	"github.com/richardwilkes/fileutil"
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
	FastLinters = [][]string{
		{"gofmt", "-l", "-s", FILES},
		{"goimports", "-l", FILES},
		{"golint", PKGS},
		{"ineffassign", REPO},
		{"misspell", "-locale", "US", FILES},
		{"go", "tool", "vet", "-all", "-shadow", DIRS},
	}
	// SlowLinters holds the linters that are known to execute quickly.
	SlowLinters = [][]string{
		{"megacheck", PKGS},
		{"errchk", "-abspath", "-blank", "-asserts", "-ignore", "github.com/richardwilkes/errs:Append", PKGS},
		{"interfacer", PKGS},
		{"unconvert", PKGS},
		{"structcheck", PKGS},
	}
)

type lint struct {
	origPath  string
	goSrcPath string
	repoPath  string
	pkgs      []string
	dirs      []string
	files     []string
	linters   [][]string
	status    int32
	lineChan  chan problem
	doneChan  chan bool
}

type problem struct {
	prefix string
	output string
}

func linters(fastOnly bool) [][]string {
	var list [][]string
	if fastOnly {
		list = FastLinters
	} else {
		list = make([][]string, len(FastLinters)+len(SlowLinters))
		copy(list, FastLinters)
		copy(list[len(FastLinters):], SlowLinters)
	}
	return list
}

func linterName(args []string) string {
	if args[0] == "go" {
		return args[2]
	}
	return args[0]
}

func newLint(basePath string, lintersToRun [][]string) (*lint, error) {
	l := &lint{
		linters:  lintersToRun,
		lineChan: make(chan problem, 16),
		doneChan: make(chan bool),
	}
	if err := l.setupPaths(basePath); err != nil {
		return nil, err
	}
	if err := l.collectPackagesAndFiles(); err != nil {
		return nil, err
	}
	return l, nil
}

func (l *lint) run(timeout time.Duration) int {
	go l.parseLines()
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	for _, args := range l.linters {
		l.execLinter(ctx, args)
		if ctx.Err() == context.DeadlineExceeded {
			break
		}
	}
	close(l.lineChan)
	<-l.doneChan
	if l.status != 0 && ctx.Err() == context.DeadlineExceeded {
		fmt.Println("*** Timeout exceeded ***")
	}
	return int(l.status)
}

func (l *lint) setupPaths(basePath string) error {
	var err error
	if l.origPath, err = filepath.Abs(basePath); err != nil {
		return errs.Wrap(err)
	}

	l.goSrcPath = os.Getenv("GOPATH")
	if l.goSrcPath == "" {
		var usr *user.User
		if usr, err = user.Current(); err != nil {
			return errs.Wrap(err)
		}
		l.goSrcPath = filepath.Join(usr.HomeDir, "go")
	} else {
		l.goSrcPath = filepath.SplitList(l.goSrcPath)[0]
	}
	if l.goSrcPath, err = filepath.Abs(filepath.Join(l.goSrcPath, "src")); err != nil {
		return errs.Wrap(err)
	}
	if !fileutil.IsDir(l.goSrcPath) {
		return fmt.Errorf("Invalid Go src path: %s", l.goSrcPath)
	}
	if _, err = filepath.Rel(l.goSrcPath, l.origPath); err != nil {
		return errs.Wrap(err)
	}

	l.repoPath = l.origPath
	for {
		if l.repoPath == l.goSrcPath {
			l.repoPath = l.origPath
			break
		}
		if fileutil.IsDir(filepath.Join(l.repoPath, ".git")) {
			break
		}
		l.repoPath = filepath.Dir(l.repoPath)
	}
	return nil
}

func (l *lint) collectPackagesAndFiles() error {
	pkgMap := make(map[string]bool)
	if walkErr := filepath.Walk(l.repoPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		name := info.Name()
		if strings.HasPrefix(name, "_") || strings.HasPrefix(name, ".") {
			if info.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		name = strings.ToLower(name)
		if name == "vendor" || name == "testdata" {
			if info.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if strings.HasSuffix(name, ".go") {
			pkg, relErr := filepath.Rel(l.goSrcPath, filepath.Dir(path))
			if relErr != nil {
				return nil
			}
			pkgMap[pkg] = true
			l.files = append(l.files, path)
		}
		return nil
	}); walkErr != nil {
		return errs.Wrap(walkErr)
	}
	l.pkgs = make([]string, 0, len(pkgMap))
	l.dirs = make([]string, 0, len(pkgMap))
	for pkg := range pkgMap {
		l.pkgs = append(l.pkgs, pkg)
		l.dirs = append(l.dirs, filepath.Join(l.goSrcPath, pkg))
	}
	return nil
}

func (l *lint) argSubstitution(args []string) []string {
	result := make([]string, 0, len(args))
	for _, arg := range args {
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

func (l *lint) execLinter(ctx context.Context, args []string) {
	args = l.argSubstitution(args)
	prefix := linterName(args)
	cc := exec.CommandContext(ctx, args[0], args[1:]...)

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

	if err = cc.Start(); err != nil {
		l.lineChan <- problem{prefix: prefix, output: err.Error()}
		return
	}
	if err = cc.Wait(); err != nil {
		atomic.StoreInt32(&l.status, 1)
		return
	}
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
	if strings.Contains(output, "(SA3000)") {
		return
	}
	if strings.Contains(output, ".pb.go") {
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
	fmt.Printf("%s [%s]\n", output, line.prefix)
	atomic.StoreInt32(&l.status, 1)
}
