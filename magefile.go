//+build mage

/*
   Velociraptor - Hunting Evil
   Copyright (C) 2019 Velocidex Innovations.

   This program is free software: you can redistribute it and/or modify
   it under the terms of the GNU Affero General Public License as published
   by the Free Software Foundation, either version 3 of the License, or
   (at your option) any later version.

   This program is distributed in the hope that it will be useful,
   but WITHOUT ANY WARRANTY; without even the implied warranty of
   MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
   GNU Affero General Public License for more details.

   You should have received a copy of the GNU Affero General Public License
   along with this program.  If not, see <https://www.gnu.org/licenses/>.
*/

package main

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/magefile/mage/mg"
	"github.com/magefile/mage/sh"
	"www.velocidex.com/golang/velociraptor/constants"
)

var (
	assets = map[string]string{
		"artifacts/b0x.yaml": "artifacts/assets/ab0x.go",
		"config/b0x.yaml":    "config/ab0x.go",
		"gui/b0x.yaml":       "gui/assets/ab0x.go",
	}

	// apt-get install gcc-mingw-w64-x86-64
	mingw_xcompiler = "x86_64-w64-mingw32-gcc"

	// apt-get install gcc-mingw-w64
	mingw_xcompiler_32 = "i686-w64-mingw32-gcc"
	name               = "velociraptor"
	version            = "v" + constants.VERSION
	base_tags          = " server_vql extras "
)

type Builder struct {
	goos        string
	arch        string
	extension   string
	extra_tags  string
	extra_flags []string

	disable_cgo bool

	// Set to override the output filename.
	filename string
}

func (self *Builder) Name() string {
	if self.filename != "" {
		return self.filename
	}

	if self.goos == "windows" {
		self.extension = ".exe"
	}

	return fmt.Sprintf("%s-%s-%s-%s%s",
		name, version,
		self.goos,
		self.arch,
		self.extension)
}

func (self *Builder) Env() map[string]string {
	env := make(map[string]string)

	env["GOOS"] = self.goos
	env["GOARCH"] = self.arch

	if self.disable_cgo {
		env["CGO_ENABLED"] = "0"
	} else {
		env["CGO_ENABLED"] = "1"
	}

	// If we are cross compiling, set the right compiler.
	if runtime.GOOS == "linux" && self.goos == "windows" {
		if self.arch == "amd64" {
			env["CC"] = mingw_xcompiler
		} else {
			env["CC"] = mingw_xcompiler_32
		}
	}

	return env
}

// Make sure the correct version of the syso file is present. If we
// are building for non windows platforms we need to remove it
// completely.
func (self Builder) ensureSyso() error {
	sh.Rm("bin/rsrc.syso")

	if self.goos == "windows" {
		switch self.arch {
		case "386":
			err := sh.Copy("bin/rsrc.syso", "docs/rsrc_386.syso")
			if err != nil {
				return err
			}
		case "amd64":
			err := sh.Copy("bin/rsrc.syso", "docs/rsrc_amd64.syso")
			if err != nil {
				return err
			}

		}
	}

	return nil
}

func (self Builder) Run() error {
	if err := os.Mkdir("output", 0700); err != nil && !os.IsExist(err) {
		return fmt.Errorf("failed to create output: %v", err)
	}

	self.ensureSyso()

	err := ensure_assets()
	if err != nil {
		return err
	}

	tags := base_tags + self.extra_tags
	args := []string{
		"build",
		"-o", filepath.Join("output", self.Name()),
		"-tags", tags,
		"-ldflags=-s -w " + flags(),
	}
	args = append(args, self.extra_flags...)
	args = append(args, "./bin/")

	return sh.RunWith(self.Env(), mg.GoCmd(), args...)
}

func Auto() error {
	return Builder{goos: runtime.GOOS,
		filename:   "velociraptor",
		extra_tags: " release ",
		arch:       runtime.GOARCH}.Run()
}

func AutoDev() error {
	return Builder{goos: runtime.GOOS,
		arch:        runtime.GOARCH,
		filename:    "velociraptor",
		extra_flags: []string{"-race"}}.Run()
}

// Build all the release versions. Darwin we build separately on a
// Mac.
func Release() error {
	err := Clean()
	if err != nil {
		return err
	}

	err = build_gui_files()
	if err != nil {
		return err
	}

	if runtime.GOOS == "linux" {
		err := Linux()
		if err != nil {
			return err
		}

		if mingwxcompiler_exists() {
			err := Windows()
			if err != nil {
				return err
			}
			return Windowsx86()
		}
	}

	if runtime.GOOS == "darwin" {
		return Darwin()
	}

	return Windows()
}

func Linux() error {
	return Builder{
		extra_tags: " release ",
		goos:       "linux",
		arch:       "amd64"}.Run()
}

func Aix() error {
	return Builder{
		extra_tags:  " release ",
		goos:        "aix",
		disable_cgo: true,
		arch:        "ppc64",
	}.Run()
}

// Builds a Development binary. This does not embed things like GUI
// resources to allow them to be loaded from the local directory.
func Dev() error {
	return Builder{goos: "linux", arch: "amd64",
		extra_flags: []string{"-race"}}.Run()
}

// Cross compile the windows binary using mingw. Note that this does
// not run the race detector because the ubuntu distribution of mingw
// does not include tsan.
func Windows() error {
	return Builder{
		extra_tags: " release ",
		goos:       "windows",
		arch:       "amd64"}.Run()
}

func WindowsDev() error {
	return Builder{
		goos:       "windows",
		extra_tags: " release ",
		filename:   "velociraptor.exe",
		arch:       "amd64"}.Run()
}

func Windowsx86() error {
	return Builder{
		extra_tags: " release ",
		goos:       "windows",
		arch:       "386"}.Run()
}

func Darwin() error {
	return Builder{goos: "darwin",
		extra_tags: " release ",
		arch:       "amd64"}.Run()
}

func DarwinBase() error {
	return Builder{goos: "darwin",
		extra_tags:  " release ",
		disable_cgo: true,
		arch:        "amd64"}.Run()
}

// Build step for Appveyor.
func Appveyor() error {
	err := build_gui_files()
	if err != nil {
		return err
	}

	err = Builder{
		goos:       "windows",
		arch:       "amd64",
		extra_tags: " release ",
		filename:   "velociraptor.exe"}.Run()

	if err != nil {
		return err
	}

	// Build a linux binary on Appveyor without cgo. This is
	// typically OK because it is mostly used for the server. It
	// will be missing yara etc.
	return Builder{
		goos:        "linux",
		arch:        "amd64",
		extra_tags:  " release ",
		disable_cgo: true,
		filename:    "velociraptor-linux.elf"}.Run()
}

func Clean() error {
	for _, target := range assets {
		err := sh.Rm(target)
		if err != nil {
			return err
		}
	}

	return nil
}

func build_gui_files() error {
	cwd, err := os.Getwd()
	if err != nil {
		return err
	}
	defer os.Chdir(cwd)

	err = os.Chdir("gui/static")
	if err != nil {
		return err
	}

	err = sh.RunV("npm", "install")
	if err != nil {
		return err
	}

	return sh.RunV("node", "node_modules/gulp/bin/gulp.js", "compile")
}

func flags() string {
	timestamp := time.Now().Format(time.RFC3339)
	return fmt.Sprintf(`-X "www.velocidex.com/golang/velociraptor/config.build_time=%s" -X "www.velocidex.com/golang/velociraptor/config.commit_hash=%s"`, timestamp, hash())
}

// hash returns the git hash for the current repo or "" if none.
func hash() string {
	hash, _ := sh.Output("git", "rev-parse", "--short", "HEAD")
	return hash
}

func fileb0x(asset string) error {
	err := sh.Run("fileb0x", asset)
	if err != nil {
		err = sh.Run(mg.GoCmd(), "get", "github.com/UnnoTed/fileb0x")
		if err != nil {
			return err
		}

		err = sh.Run("fileb0x", asset)
	}

	return err
}

func ensure_assets() error {
	for asset, target := range assets {
		before := timestamp_of(target)
		err := fileb0x(asset)
		if err != nil {
			return err
		}
		// Only do this if the file has changed.
		if before != timestamp_of(target) {
			err = replace_string_in_file(target, "func init()", "func Init()")
			if err != nil {
				return err
			}
		}
	}

	return nil
}

func mingwxcompiler_exists() bool {
	err := sh.Run(mingw_xcompiler, "--version")
	return err == nil
}

// Temporarily manipulate the generated file until
// https://github.com/UnnoTed/fileb0x/pull/46 goes in.
func replace_string_in_file(filename string, old string, new string) error {
	read, err := ioutil.ReadFile(filename)
	if err != nil {
		return err
	}
	newContents := strings.Replace(string(read), old, new, -1)
	return ioutil.WriteFile(filename, []byte(newContents), 0)
}

func timestamp_of(path string) int64 {
	stat, err := os.Stat(path)
	if os.IsNotExist(err) || err != nil {
		return 0
	}

	return stat.ModTime().UnixNano()
}
