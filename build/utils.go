package build

import (
	"archive/tar"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/ulikunitz/xz"
	"www.velocidex.com/golang/velociraptor/utils"
)

var (
	// When building on ubuntu
	package_url_ubuntu_amd64 = ToolchainDesc{
		Download: "https://github.com/mstorsjo/llvm-mingw/releases/download/20260908/llvm-mingw-20260908-ucrt-ubuntu-22.04-x86_64.tar.xz",
		_GCC: map[string]string{
			"windows/armv7":   "llvm-mingw-20260908-ucrt-ubuntu-22.04-x86_64/bin/armv7-w64-mingw32-clang",
			"windows/386":     "llvm-mingw-20260908-ucrt-ubuntu-22.04-x86_64/bin/i686-w64-mingw32-clang",
			"windows/amd64":   "llvm-mingw-20260908-ucrt-ubuntu-22.04-x86_64/bin/x86_64-w64-mingw32-clang",
			"windows/aarch64": "llvm-mingw-20260908-ucrt-ubuntu-22.04-x86_64/bin/aarch64-w64-mingw32-clang",
		},
	}

	// When building on Windows
	package_url_windows_amd64 = ToolchainDesc{
		Download: "https://github.com/mstorsjo/llvm-mingw/releases/download/20260908/llvm-mingw-20260908-ucrt-i686.zip",
		_GCC: map[string]string{
			"windows/armv7":   "llvm-mingw-20260908-ucrt-i686/bin/armv7-w64-mingw32-clang",
			"windows/amd64":   "llvm-mingw-20260908-ucrt-i686/bin/i686-w64-mingw32-clang",
			"windows/aarch64": "llvm-mingw-20260908-ucrt-i686/bin/aarch64-w64-mingw32-clang",
		},
	}
)

type ToolchainDesc struct {
	basedir  string
	verbose  bool
	Download string
	_GCC     map[string]string
}

func NewToolchainDesc(basedir string, verbose bool) *ToolchainDesc {
	res := package_url_ubuntu_amd64
	res.basedir = basedir
	res.verbose = verbose
	return &res
}

func (self *ToolchainDesc) extractXZ(reader io.Reader) error {
	gzr, err := xz.NewReader(reader)
	if err != nil {
		return err
	}

	tr := tar.NewReader(gzr)
	for {
		header, err := tr.Next()

		switch {
		case err == io.EOF:
			return nil

		case err != nil:
			return err

		case header == nil:
			continue
		}

		target := filepath.Join(self.basedir, header.Name)

		// check the file type
		switch header.Typeflag {
		default:
			fmt.Printf("Do not know how to handle type %v: %#v\n",
				header.Typeflag, header)
			return utils.InvalidArgError

		case tar.TypeSymlink:
			target_link := header.Linkname
			err := os.Symlink(target_link, target)
			if self.verbose {
				fmt.Printf("Linking %v to %v\n", target, target_link)
			}
			if err != nil {
				return err
			}

		// if its a dir and it doesn't exist create it
		case tar.TypeDir:
			err := os.MkdirAll(target, 0755)
			if err != nil {
				return err
			}

		case tar.TypeReg:
			if self.verbose {
				fmt.Printf("Creating %v (%v bytes)\n", target, header.Size)
			}
			f, err := os.OpenFile(target, os.O_CREATE|os.O_RDWR, os.FileMode(header.Mode))
			if err != nil {
				return err
			}

			_, err = io.Copy(f, tr)
			if err != nil {
				return err
			}

			f.Close()

			err = os.Chmod(target, os.FileMode(header.Mode))
			if err != nil {
				return err
			}
		}
	}
}

func (self *ToolchainDesc) GCC(target string) (string, error) {
	// Check if the compiler is there
	filename, pres := self._GCC[target]
	if !pres {
		return "", fmt.Errorf("Unsupported target %v", target)
	}

	fullpath, err := filepath.Abs(filepath.Join(
		self.basedir, filename))
	if err != nil {
		return "", err
	}

	cmd := exec.Command(fullpath, "--version")
	_, err = cmd.CombinedOutput()
	return fullpath, err
}

func InstallToolChain(dst string, verbose bool) (*ToolchainDesc, error) {
	var err error

	desc := NewToolchainDesc(dst, verbose)

	stat, err := os.Lstat(dst)
	// If the directory already exists, we assume the toolchain is
	// already downloaded.
	if err == nil && stat.Mode().IsDir() {
		return desc, nil
	}

	resp, err := http.Get(package_url_ubuntu_amd64.Download)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	return desc, desc.extractXZ(resp.Body)
}
