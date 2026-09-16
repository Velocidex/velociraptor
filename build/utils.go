package build

import (
	"archive/tar"
	"archive/zip"
	"bufio"
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

type ToolChainOptions struct {
	BaseDir         string
	PackageCacheDir string
	Verbose         bool
}

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
	ctx context.Context, fd *os.File) error {

	stat, err := os.Lstat(fd.Name())
	if err == nil && stat.Mode().IsDir() {
		return nil
	}

	z, err := zip.NewReader(fd, stat.Size())
	if err != nil {
		return err
	}

	buffer := make([]byte, 10*1024*1024)

	for _, member := range z.File {
		if utils.IsCtxDone(ctx) {
			return utils.CancelledError
		}

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

		_, err = utils.CopyWithBuffer(ctx, out_fd, in_fd, buffer)
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

func (self *ToolchainDesc) extractXZ(
	ctx context.Context, fd *os.File) error {
	gzr, err := xz.NewReader(bufio.NewReader(fd))
	if err != nil {
		return err
	}

	buffer := make([]byte, 10*1024*1024)

	tr := tar.NewReader(gzr)
	for {
		if utils.IsCtxDone(ctx) {
			return utils.CancelledError
		}

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

			_, err = utils.CopyWithBuffer(ctx, f, tr, buffer)
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

func fetchFile(
	ctx context.Context, url, output string) error {
	// Download the package locally
	out_fd, err := os.OpenFile(output,
		os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0700)
	if err != nil {
		return err
	}
	defer out_fd.Close()

	req, err := http.NewRequestWithContext(
		ctx, "GET", url, nil)
	if err != nil {
		return err
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	_, err = utils.Copy(ctx, out_fd, resp.Body)
	return err
}

func InstallToolChain(
	ctx context.Context,
	options ToolChainOptions) (*ToolchainDesc, error) {

	if options.BaseDir == "" || options.PackageCacheDir == "" {
		return nil, fmt.Errorf(
			"Both BaseDir and PackageCacheDir must be provided")
	}

	desc := NewToolchainDesc(
		options.BaseDir, options.Verbose)

	stat, err := os.Lstat(options.BaseDir)
	// If the directory already exists, we assume the toolchain is
	// already downloaded.
	if err == nil && stat.Mode().IsDir() {
		return desc, nil
	}

	package_filename := filepath.Join(
		options.PackageCacheDir, filepath.Base(desc.Download))

	_, err = os.Lstat(package_filename)
	if err == nil {
		fmt.Printf("Reusing cached package %v\n", package_filename)

	} else {
		if os.IsNotExist(err) {
			fmt.Printf("Downloading package %v into %v\n",
				desc.Download, package_filename)

			err := fetchFile(ctx, desc.Download, package_filename)
			if err != nil {
				return nil, err
			}
		} else {
			return nil, err
		}
	}

	fd, err := os.Open(package_filename)
	if err != nil {
		return nil, err
	}
	defer fd.Close()

	if strings.HasSuffix(package_filename, ".xz") {
		fmt.Printf("Unpacking %v to %v\n", package_filename,
			options.BaseDir)
		defer fmt.Printf("Done!\n")
		return desc, desc.extractXZ(ctx, fd)
	}

	if strings.HasSuffix(package_filename, ".zip") {
		fmt.Printf("Unpacking %v to %v\n", package_filename,
			options.BaseDir)
		defer fmt.Printf("Done!\n")
		return desc, desc.extractZip(ctx, fd)
	}

	return nil, fmt.Errorf("Unknown compression for %v",
		package_filename)
}
