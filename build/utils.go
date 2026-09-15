package build

import (
	"archive/tar"
	"archive/zip"
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/ulikunitz/xz"
	"www.velocidex.com/golang/velociraptor/utils"
	"www.velocidex.com/golang/velociraptor/utils/tempfile"
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
	// Should be an absolute path for unpacking the toolchain
	basedir string

	// Enable for extract messages.
	verbose bool

	// The download URL for the package.
	Download string

	// A map of compilers keyed by architectures
	_GCC map[string]string
}

func NewToolchainDesc(basedir string, verbose bool) *ToolchainDesc {
	var res ToolchainDesc
	// When building on Windows use these tool chains.
	if runtime.GOOS == "windows" {
		res = package_url_windows_amd64
	} else {
		res = package_url_ubuntu_amd64
	}

	res.basedir = basedir
	res.verbose = verbose
	return &res
}

func (self *ToolchainDesc) extractZip(
	ctx context.Context, reader io.Reader) error {
	// Zip files need to be copied to a tempfile for unpacking.
	tempfile_fd, err := tempfile.TempFile("")
	if err != nil {
		return err
	}

	_, err = utils.Copy(ctx, tempfile_fd, reader)
	if err != nil {
		return err
	}
	defer tempfile_fd.Close()

	defer os.Remove(tempfile_fd.Name())

	stat, err := os.Lstat(tempfile_fd.Name())
	if err == nil && stat.Mode().IsDir() {
		return nil
	}

	z, err := zip.NewReader(tempfile_fd, stat.Size())
	if err != nil {
		return err
	}

	for _, member := range z.File {
		if member.FileInfo().IsDir() {
			continue
		}

		output_path := utils.Join(self.basedir, member.Name)
		if self.verbose {
			fmt.Printf("Creating %v (%v bytes)\n", output_path,
				member.FileInfo().Size())
		}
		err = os.MkdirAll(filepath.Dir(output_path), 0700)
		if err != nil {
			return err
		}
		out_fd, err := os.OpenFile(output_path,
			os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0700)
		if err != nil {
			return err
		}

		in_fd, err := member.Open()
		if err != nil {
			out_fd.Close()
			return err
		}

		_, err = utils.Copy(ctx, out_fd, in_fd)
		if err != nil {
			in_fd.Close()
			out_fd.Close()
			return err
		}
		in_fd.Close()
		out_fd.Close()
	}
	return nil
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

	fullpath := filepath.Join(self.basedir, filename)

	cmd := exec.Command(fullpath, "--version")
	_, err := cmd.CombinedOutput()
	return fullpath, err
}

func InstallToolChain(
	ctx context.Context,
	dst string, verbose bool) (*ToolchainDesc, error) {

	desc := NewToolchainDesc(dst, verbose)

	stat, err := os.Lstat(dst)
	// If the directory already exists, we assume the toolchain is
	// already downloaded.
	if err == nil && stat.Mode().IsDir() {
		return desc, nil
	}

	var reader io.Reader
	filename := filepath.Join(dst, "../packages",
		filepath.Base(desc.Download))
	_, err = os.Lstat(filename)
	if err == nil {
		fd, err := os.Open(filename)
		if err != nil {
			return nil, err
		}
		defer fd.Close()
		reader = fd
		fmt.Printf("Got a local package %v\n", filename)

	} else {
		req, err := http.NewRequestWithContext(
			ctx, "GET", desc.Download, nil)
		if err != nil {
			return nil, err
		}

		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			return nil, err
		}
		reader = resp.Body
		defer resp.Body.Close()
	}

	if strings.HasSuffix(filename, ".xz") {
		return desc, desc.extractXZ(reader)
	}

	if strings.HasSuffix(filename, ".zip") {
		return desc, desc.extractZip(ctx, reader)
	}

	return nil, fmt.Errorf("Unknown compression for %v", filename)
}
