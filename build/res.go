package build

import (
	"errors"
	"fmt"
	"io/ioutil"
	"strconv"
	"strings"

	"github.com/Masterminds/semver/v3"
	"github.com/magefile/mage/mg"
	"github.com/magefile/mage/sh"
	"www.velocidex.com/golang/velociraptor/constants"
)

func UpdateVersionInfo() error {
	read, err := ioutil.ReadFile("docs/winres/winres_template.json")
	if err != nil {
		return err
	}

	v, err := semver.NewVersion(constants.VERSION)
	if err != nil {
		return err
	}

	var prerelease int64
	prerelease_str := v.Prerelease()
	if prerelease_str != "" {
		if !strings.HasPrefix(prerelease_str, "rc") {
			return errors.New("Prerelease version should start with rc")
		}

		prerelease, err = strconv.ParseInt(prerelease_str[2:], 0, 0)
		if err != nil {
			return errors.New("Prerelease version should start with rc followed by numbers")
		}
	}

	version := fmt.Sprintf("%d.%d.%d.%d", v.Major(), v.Minor(),
		v.Patch(), prerelease)
	newContents := strings.Replace(string(read), "0.0.0.0", version, -1)
	err = ioutil.WriteFile("docs/winres/winres.json",
		[]byte(newContents), 0644)
	if err != nil {
		return err
	}

	command := []string{"make",
		"--in", "docs/winres/winres.json", "--out", "bin/rsrc"}

	err = sh.Run("go-winres", command...)
	if err != nil {
		err = sh.Run(mg.GoCmd(), "install", "github.com/tc-hib/go-winres@d743268d7ea168077ddd443c4240562d4f5e8c3e")
		if err != nil {
			return err
		}

		err = sh.Run("go-winres", command...)

		return err
	}

	return nil
}
