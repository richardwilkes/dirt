package main

import (
	"bufio"
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/richardwilkes/toolbox/errs"
	"github.com/richardwilkes/toolbox/xio/fs"
)

var vendor = fmt.Sprintf("%[1]cvendor%[1]c", os.PathSeparator)

func listPackages() ([]string, error) {
	return golist()
}

func listDirs() ([]string, error) {
	return golist("-f", "{{.Dir}}")
}

func listFiles() ([]string, error) {
	return golist("-f", fmt.Sprintf("{{$d := .Dir}}{{range .GoFiles}}{{$d}}%c{{.}}\n{{end}}", os.PathSeparator))
}

func golist(extra ...string) ([]string, error) {
	args := make([]string, 0, 2+len(extra))
	args = append(args, "list")
	args = append(args, extra...)
	args = append(args, "./...")
	cmd := exec.Command("go", args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return nil, errs.NewfWithCause(err, "go %s", strings.Join(args, " "))
	}
	var result []string
	scanner := bufio.NewScanner(bytes.NewReader(out))
	for scanner.Scan() {
		one := strings.TrimSpace(scanner.Text())
		if one != "" {
			if !strings.Contains(one, "/vendor/") && !strings.Contains(one, vendor) {
				result = append(result, one)
			}
		}
	}
	return result, nil
}

func findRoot(in string) string {
	pathSeparator := string([]rune{os.PathSeparator})
	orig, err := filepath.Abs(in)
	if err != nil {
		return in
	}
	path := orig
	for {
		if fs.IsDir(filepath.Join(path, ".git")) {
			return path
		}
		path = filepath.Dir(path)
		if strings.HasSuffix(path, pathSeparator) {
			return orig
		}
	}
}
