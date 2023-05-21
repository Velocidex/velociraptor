//+build mage

/*
   Velociraptor - Dig Deeper
   Copyright (C) 2019-2022 Rapid7 Inc.

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
	"bytes"
	"fmt"
	"io/ioutil"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/magefile/mage/mg"
	"github.com/magefile/mage/sh"
	"www.velocidex.com/golang/velociraptor/constants"
	"www.velocidex.com/golang/velociraptor/json"
)

var (
	assets = map[string]string{
		"artifacts/b0x.yaml":        "artifacts/assets/ab0x.go",
		"config/b0x.yaml":           "config/ab0x.go",
		"gui/velociraptor/b0x.yaml": "gui/velociraptor/ab0x.go",
		"crypto/b0x.yaml":           "crypto/ab0x.go",
	}

	index_template = "gui/velociraptor/build/index.html"

	// apt-get install gcc-mingw-w64-x86-64
	mingw_xcompiler = "x86_64-w64-mingw32-gcc"

	// apt-get install gcc-mingw-w64
	mingw_xcompiler_32 = "i686-w64-mingw32-gcc"
	musl_xcompiler     = "musl-gcc"
	name               = "velociraptor"
	version            = "v" + constants.VERSION
	base_tags          = " server_vql extras "
)

type Builder struct {
	goos          string
	arch          string
	extension     string
	extra_tags    string
	extra_flags   []string
	extra_ldflags string
	cc            string

	disable_cgo bool

	// Set to override the output filename.
	filename string

	extra_name string
}

func (self *Builder) Name() string {
	if self.filename != "" {
		return self.filename
	}

	if self.goos == "windows" {
		self.extension = ".exe"
	}

	name := fmt.Sprintf("%s-%s-%s-%s%s",
		name, version,
		self.goos,
		self.arch,
		self.extension)

	if self.disable_cgo {
		name += "-nocgo"
	}

	name += self.extra_name

	return name
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
	if (runtime.GOOS == "linux" || runtime.GOOS == "darwin") &&
		self.goos == "windows" {

		if self.arch == "amd64" {
			if mingwxcompiler_exists() {
				env["CC"] = mingw_xcompiler
			}
		} else {
			if mingwxcompiler32_exists() {
				env["CC"] = mingw_xcompiler_32
			}
		}
	}

	if self.cc != "" {
		env["CC"] = self.cc
	}
	fmt.Printf("Build Environment: %v\n", json.MustMarshalString(env))
	return env
}

func (self Builder) Run() error {
	if err := os.Mkdir("output", 0700); err != nil && !os.IsExist(err) {
		return fmt.Errorf("failed to create output: %v", err)
	}

	err := ensure_assets()
	if err != nil {
		return err
	}

	tags := base_tags + self.extra_tags
	args := []string{
		"build",
		"-o", filepath.Join("output", self.Name()),
		"-tags", tags,
		"-ldflags=-s -w " + self.extra_ldflags + flags(),
	}
	args = append(args, self.extra_flags...)
	args = append(args, "./bin/")

	return sh.RunWith(self.Env(), mg.GoCmd(), args...)
}

func Auto() error {
	return Builder{goos: runtime.GOOS,
		filename:   "velociraptor",
		extra_tags: " release yara ",
		arch:       runtime.GOARCH}.Run()
}

func AutoDev() error {
	return Builder{goos: runtime.GOOS,
		arch:        runtime.GOARCH,
		extra_tags:  " yara ",
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

	err = UpdateDependentTools()
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
		extra_tags: " release yara ",
		goos:       "linux",
		arch:       "amd64"}.Run()
}

func LinuxMusl() error {
	return Builder{
		extra_tags:    " release yara ",
		goos:          "linux",
		cc:            "musl-gcc",
		extra_name:    "-musl",
		extra_ldflags: "-linkmode external -extldflags \"-static\"",
		arch:          "amd64"}.Run()
}

func LinuxMusl386() error {
	return Builder{
		extra_tags:    " release yara disable_gui ",
		goos:          "linux",
		cc:            "musl-gcc",
		extra_name:    "-musl",
		disable_cgo:   true,
		extra_ldflags: "-linkmode external -extldflags \"-static\"",
		arch:          "386"}.Run()
}

// A Linux binary without the GUI
func LinuxBare() error {
	return Builder{
		extra_tags: " release yara disable_gui ",
		goos:       "linux",
		arch:       "amd64"}.Run()
}

func Freebsd() error {
	return Builder{
		extra_tags: " release yara ",
		goos:       "freebsd",

		// When building on actual freebsd we should have c
		// compilers, otherwise disable cgo.
		disable_cgo: runtime.GOOS != "freebsd",
		arch:        "amd64"}.Run()
}

func Aix() error {
	return Builder{
		extra_tags:  " release yara ",
		goos:        "aix",
		disable_cgo: true,
		arch:        "ppc64",
	}.Run()
}

func PPCLinux() error {
	return Builder{
		extra_tags:  " release yara ",
		goos:        "linux",
		disable_cgo: true,
		arch:        "ppc64le",
	}.Run()
}

func Version() error {
	fmt.Println(constants.VERSION)
	return nil
}

func Arm() error {
	return Builder{
		extra_tags:  " release yara ",
		goos:        "linux",
		disable_cgo: true,
		arch:        "arm",
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
		extra_tags: " release yara ",
		goos:       "windows",
		arch:       "amd64"}.Run()
}

// Windows client without a gui.
func WindowsBare() error {
	return Builder{
		extra_tags: " release yara disable_gui ",
		goos:       "windows",
		arch:       "amd64"}.Run()
}

func WindowsDev() error {
	return Builder{
		goos:       "windows",
		extra_tags: " release yara ",
		filename:   "velociraptor.exe",
		arch:       "amd64"}.Run()
}

// Windows binary with race detection. This requires building on
// windows (ie not cross compiling). You will need to install gcc
// first using https://jmeubank.github.io/tdm-gcc/ as well as the Go
// windows distribution and optionally the windows node distribution
// (for the GUI).
func WindowsTest() error {
	return Builder{
		goos:        "windows",
		extra_tags:  " release yara ",
		filename:    "velociraptor.exe",
		arch:        "amd64",
		extra_flags: []string{"-race"}}.Run()
}

func Windowsx86() error {
	return Builder{
		extra_tags: " release yara ",
		goos:       "windows",
		arch:       "386"}.Run()
}

func Darwin() error {
	return Builder{goos: "darwin",
		extra_tags: " release yara ",
		arch:       "amd64"}.Run()
}

func DarwinM1() error {
	return Builder{goos: "darwin",
		extra_tags: " release yara ",
		arch:       "arm64"}.Run()
}

func LinuxM1() error {
	return Builder{goos: "linux",
		extra_tags:  " release yara ",
		disable_cgo: true,
		arch:        "arm64"}.Run()
}
func DarwinBase() error {
	return Builder{goos: "darwin",
		extra_tags:  " release ",
		disable_cgo: true,
		arch:        "amd64"}.Run()
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

// Only build the assets without building the actual code.
func Assets() error {
	err := build_gui_files()
	if err != nil {
		return err
	}

	return ensure_assets()
}

func build_gui_files() error {
	cwd, err := os.Getwd()
	if err != nil {
		return err
	}
	defer os.Chdir(cwd)

	err = os.Chdir("gui/velociraptor")
	if err != nil {
		return err
	}

	err = sh.RunV("npm", "ci")
	if err != nil {
		return err
	}

	err = sh.RunV("npm", "run", "build")
	if err != nil {
		return err
	}

	// Recreate the keep files since sometimes they get removed.
	for _, keep_path := range []string{
		"build/.keep",
	} {
		os.MkdirAll(path.Dir(keep_path), 0600)

		fd, err := os.OpenFile(keep_path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0600)
		if err == nil {
			fd.Close()
		}
	}
	return nil
}

func flags() string {
	timestamp := time.Now().Format(time.RFC3339)
	flags := fmt.Sprintf(` -X "www.velocidex.com/golang/velociraptor/config.build_time=%s"`, timestamp)

	flags += fmt.Sprintf(` -X "www.velocidex.com/golang/velociraptor/config.commit_hash=%s"`, hash())

	// If we are running on the CI pipeline we need to know the run
	// number and URL so we can report them.
	if os.Getenv("GITHUB_SERVER_URL") != "" {
		flags += fmt.Sprintf(` -X "www.velocidex.com/golang/velociraptor/config.ci_run_url=%s"`,
			os.ExpandEnv("$GITHUB_SERVER_URL/$GITHUB_REPOSITORY/actions/runs/$GITHUB_RUN_ID"))
	}

	return flags
}

// hash returns the git hash for the current repo or "" if none.
func hash() string {
	hash, _ := sh.Output("git", "rev-parse", "--short", "HEAD")
	return hash
}

func fileb0x(asset string) error {
	err := sh.Run("fileb0x", asset)
	if err != nil {
		err = sh.Run(mg.GoCmd(), "install", "github.com/Velocidex/fileb0x@d54f4040016051dd9657ce04d0ae6f31eab99bc6")
		if err != nil {
			return err
		}

		err = sh.Run("fileb0x", asset)
	}

	return err
}

func ensure_assets() error {
	// Fixup the vite build - this is a hack but i cant figure vite
	// now.
	replace_string_in_file(
		index_template, `="/app/assets/index`,
		`="{{.BasePath}}/app/assets/index`)

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

func musl_exists() bool {
	err := sh.Run(musl_xcompiler, "--version")
	return err == nil
}

func mingwxcompiler32_exists() bool {
	err := sh.Run(mingw_xcompiler_32, "--version")
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

func UpdateDependentTools() error {
	template := "artifacts/definitions/Server/Internal/ToolDependencies.tmpl"
	fd, err := os.Open(template)
	if err != nil {
		return err
	}
	defer fd.Close()

	data, err := ioutil.ReadAll(fd)
	if err != nil {
		return err
	}

	outfile := strings.ReplaceAll(template, "tmpl", "yaml")
	outfd, err := os.OpenFile(outfile,
		os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0600)
	if err != nil {
		return err
	}
	defer outfd.Close()

	_, err = outfd.Write(
		bytes.ReplaceAll(data, []byte("<VERSION>"), []byte(constants.VERSION)))
	return err
}
