//+build mage

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
)

var (
	assets = map[string]string{
		"artifacts/b0x.yaml": "artifacts/assets/ab0x.go",
		"config/b0x.yaml":    "config/ab0x.go",
		"gui/b0x.yaml":       "gui/assets/ab0x.go",
	}

	mingw_xcompiler = "x86_64-w64-mingw32-gcc"
	name            = "velociraptor"
	version         = "v0.2.5"
)

func Xgo() error {
	return sh.RunV(
		"xgo", "-out", filepath.Join("output", "velociraptor-"+version), "-v",
		"--targets", "windows/*,darwin/*,linux/*",
		"-tags", "release",
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
		"-tags", "release",
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
		mg.GoCmd(), "build",
		"-o", filepath.Join("output", name),
		"-tags", "devel",
		"-ldflags=-s -w "+flags(),
		"./bin/")

	if err != nil {
		return err
	}

	return err
}

// Cross compile the windows binary using mingw
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
		"-tags", "release",
		"-ldflags=-s -w "+flags(),
		"./bin/")

	if err != nil {
		return err
	}

	return err
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
