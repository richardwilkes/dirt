package main

import (
	"archive/zip"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/mitchellh/go-homedir"
	"github.com/richardwilkes/toolbox/cmdline"
	"github.com/richardwilkes/toolbox/xio"
	"github.com/richardwilkes/toolbox/xio/fs/safe"
)

// Archive creates an archive of the required linters.
func Archive(goos, goarch string) (err error) {
	var tmpDir string
	tmpDir, err = ioutil.TempDir(os.TempDir(), cmdline.AppCmdName)
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmpDir) // @allow
	if goos == "" {
		goos = runtime.GOOS
	}
	if goarch == "" {
		goarch = runtime.GOARCH
	}
	var f *safe.File
	f, err = safe.Create(fmt.Sprintf("%s-linters-%s-%s-%s.zip", cmdline.AppCmdName, runtime.Version(), goos, goarch))
	if err != nil {
		return err
	}
	defer func() {
		if cerr := f.Close(); cerr != nil && err == nil {
			err = cerr
		}
	}()
	zw := zip.NewWriter(f)
	gopath, setgopath := os.LookupEnv("GOPATH")
	for _, one := range selectLinters(false) {
		if one.pkg != "" {
			fmt.Printf("Building GOOS=%s GOARCH=%s %s...\n", goos, goarch, one.cmd)
			one.Install(false)
			path := filepath.Join(tmpDir, one.cmd)
			cmd := exec.Command("go", "build", "-o", path, one.pkg)
			cmd.Env = os.Environ()
			cmd.Env = append(cmd.Env, "GOOS="+goos, "GOARCH="+goarch)
			if setgopath {
				cmd.Env = append(cmd.Env, "GOPATH="+gopath)
			}
			var data []byte
			if data, err = cmd.CombinedOutput(); err != nil {
				err = fmt.Errorf("Unable to build %s\n%s", one.Name(), string(data))
				return err
			}
			if err = copyFileToZip(path, zw); err != nil {
				return err
			}
		}
	}
	if err = zw.Close(); err != nil {
		return err
	}
	err = f.Commit()
	return err
}

func copyFileToZip(fromPath string, to *zip.Writer) error {
	in, err := os.Open(fromPath)
	if err != nil {
		return err
	}
	defer xio.CloseIgnoringErrors(in)
	info, err := in.Stat()
	if err != nil {
		return err
	}
	header, err := zip.FileInfoHeader(info)
	if err != nil {
		return err
	}
	header.Method = zip.Deflate
	out, err := to.CreateHeader(header)
	if err != nil {
		return err
	}
	_, err = io.Copy(out, in)
	return err
}

// InstallFromArchive attempts to install the linters from an archive.
func InstallFromArchive(url string) error {
	path, err := ensureBinPath()
	if err != nil {
		return err
	}
	var archivePath string
	lower := strings.ToLower(url)
	if strings.HasPrefix(lower, "http:") || strings.HasPrefix(lower, "https:") {
		var f *os.File
		f, err = ioutil.TempFile(path, cmdline.AppCmdName)
		if err != nil {
			return err
		}
		archivePath = f.Name()
		defer os.Remove(archivePath) // @allow
		err = downloadArchive(url, f)
		if err != nil {
			return err
		}
		if err = f.Close(); err != nil {
			return err
		}
	} else {
		archivePath = url
	}
	linters := selectLinters(false)
	zr, err := zip.OpenReader(archivePath)
	if err != nil {
		return err
	}
	defer xio.CloseIgnoringErrors(zr)
	for _, f := range zr.File {
		for _, linter := range linters {
			if linter.pkg != "" && f.Name == linter.cmd {
				if err = copyFileFromZip(f, filepath.Join(path, linter.cmd)); err != nil {
					return err
				}
				break
			}
		}
	}
	return nil
}

func ensureBinPath() (string, error) {
	var path string
	paths := filepath.SplitList(os.Getenv("GOPATH"))
	if len(paths) == 0 {
		dir, err := homedir.Dir()
		if err != nil {
			return "", err
		}
		path = filepath.Join(dir, "go")
	} else {
		path = paths[0]
	}
	path = filepath.Join(path, "bin")
	if err := os.MkdirAll(path, 0755); err != nil {
		return "", err
	}
	return path, nil
}

func downloadArchive(url string, w io.Writer) error {
	resp, err := http.Get(url)
	if err != nil {
		return err
	}
	defer xio.CloseIgnoringErrors(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("Unable to retrieve archive from %s : %s", url, resp.Status)
	}
	_, err = io.Copy(w, resp.Body)
	return err
}

func copyFileFromZip(f *zip.File, dstPath string) error {
	r, err := f.Open()
	if err != nil {
		return err
	}
	defer xio.CloseIgnoringErrors(r)
	w, err := os.OpenFile(dstPath, os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0755)
	if err != nil {
		return err
	}
	defer func() {
		if cerr := w.Close(); cerr != nil && err == nil {
			err = cerr
		}
	}()
	_, err = io.Copy(w, r)
	return err
}
