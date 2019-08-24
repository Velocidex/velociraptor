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

	mingw_xcompiler = "x86_64-w64-mingw32-gcc"
	name            = "velociraptor"
	version         = "v" + constants.VERSION
)

func Xgo() error {
	err := build_gui_files()
	if err != nil {
		return err
	}

	return sh.RunV(
		"xgo", "-out", filepath.Join("output", "velociraptor-"+version), "-v",
		"--targets", "windows/*,darwin/amd64,linux/amd64",
		"-tags", "release server_vql cgo",
		"-go", "1.11",
		"-ldflags=-s -w "+flags(),
		"./bin/")
}

func WindowsRace() error {
	return sh.RunV(
		"xgo", "-out", filepath.Join("output", "velociraptor-"+version), "-v",
		"--targets", "windows/amd64",
		"-go", "1.11",
		"-tags", "release server_vql cgo", "-race",
		"-ldflags=-s -w "+flags(),
		"./bin/")
}

func Linux() error {
	if err := os.Mkdir("output", 0700); err != nil && !os.IsExist(err) {
		return fmt.Errorf("failed to create output: %v", err)
	}

	err := ensure_assets()
	if err != nil {
		return err
	}

	env := make(map[string]string)
	err = sh.RunWith(
		env,
		mg.GoCmd(), "build",
		"-o", filepath.Join("output", name),
		"-tags", "release server_vql ",
		"-ldflags=-s -w "+flags(),
		"./bin/")

	if err != nil {
		return err
	}

	return err
}

// Builds a Development binary. This does not embed things like GUI
// resources to allow them to be loaded from the local directory.
func Dev() error {
	if err := os.Mkdir("output", 0700); err != nil && !os.IsExist(err) {
		return fmt.Errorf("failed to create output: %v", err)
	}

	err := ensure_assets()
	if err != nil {
		return err
	}

	env := make(map[string]string)
	err = sh.RunWith(
		env,
		mg.GoCmd(), "build", "-race",
		"-o", filepath.Join("output", name),
		"-tags", "devel server_vql ",
		"-ldflags=-s -w "+flags(),
		"./bin/")

	if err != nil {
		return err
	}

	return err
}

// Build step for Appveyor.
func Appveyor() error {
	cwd, err := os.Getwd()
	if err != nil {
		return err
	}
	defer os.Chdir(cwd)

	err = os.Chdir("gui/static")
	if err != nil {
		return err
	}

	err = sh.RunV("npm", "install", "-g")
	if err != nil {
		return err
	}

	err = sh.RunV("npm", "install", "-g", "gulp", "gulp-cli")
	if err != nil {
		return err
	}

	err = sh.RunV("gulp", "compile")
	if err != nil {
		return err
	}

	os.Chdir(cwd)

	return Windows()
}

// Cross compile the windows binary using mingw. Note that this does
// not run the race detector because the ubuntu distribution of mingw
// does not include tsan. Use WindowsRace() to build with xgo.
func Windows() error {
	if err := os.Mkdir("output", 0700); err != nil && !os.IsExist(err) {
		return fmt.Errorf("failed to create output: %v", err)
	}

	err := ensure_assets()
	if err != nil {
		return err
	}

	env := make(map[string]string)
	if mingwxcompiler_exists() {
		env["CC"] = mingw_xcompiler
		env["CGO_ENABLED"] = "1"
	} else {
		fmt.Printf("Windows cross compiler not found. Disabling cgo modules.")
		env["CGO_ENABLED"] = "0"
	}

	env["GOOS"] = "windows"
	env["GOARCH"] = "amd64"

	err = sh.RunWith(
		env,
		mg.GoCmd(), "build",
		"-o", filepath.Join("output", name+".exe"),
		"-tags", "release server_vql ",
		"-ldflags=-s -w "+flags(),
		"./bin/")

	if err != nil {
		return err
	}

	return err
}

// We have to compile darwin executables with xgo otherwise many of
// the cgo plugins wont work.
func Darwin() error {
	return sh.RunV(
		"xgo", "-out", filepath.Join("output", "velociraptor-"+version), "-v",
		"--targets", "darwin/amd64",
		"-tags", "release server_vql ",
		"-ldflags=-s -w "+flags(),
		"./bin/")
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

	err = sh.RunV("gulp", "clean")
	if err != nil {
		return err
	}

	return sh.RunV("gulp", "compile")
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
