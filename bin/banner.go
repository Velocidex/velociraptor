package main

import (
	"os"
	"strings"

	"www.velocidex.com/golang/velociraptor/config"
	logging "www.velocidex.com/golang/velociraptor/logging"
)

var (
	nobanner_flag = app.Flag(
		"nobanner", "Suppress the Velociraptor banner").Bool()
)

var banner = `
<green> _    __     __           _                  __
<green>| |  / /__  / /___  _____(_)________ _____  / /_____  _____
<green>| | / / _ \/ / __ \/ ___/ / ___/ __ ` + "`" + `/ __ \/ __/ __ \/ ___/
<green>| |/ /  __/ / /_/ / /__/ / /  / /_/ / /_/ / /_/ /_/ / /
<green>|___/\___/_/\____/\___/_/_/   \__,_/ .___/\__/\____/_/
<green>                                  /_/
<red>Digging deeper!                  <cyan>https://www.velocidex.com
`

func doBanner() {
	if *nobanner_flag {
		return
	}
	for _, line := range strings.Split(banner, "\n") {
		if len(line) > 0 {
			logging.Prelog("%s", line)
		}
	}

	version := config.GetVersion()

	logging.Prelog("<yellow>This is Velociraptor %v built on %v (%v)", version.Version,
		version.BuildTime, version.Commit)

	// Record some important environment variables to the log.
	for _, e := range os.Environ() {
		pair := strings.SplitN(e, "=", 2)
		if len(pair) != 2 {
			continue
		}

		switch pair[0] {
		case // Ignore this one as it is the actual configuration
			"VELOCIRAPTOR_LITERAL_CONFIG",
			"VELOCIRAPTOR_SLOW_FILESYSTEM",
			"VELOCIRAPTOR_DISABLE_CSRF", "VELOCIRAPTOR_INJECT_API_SLEEP",
			"GOGC", "GOTRACEBACK", "GOMAXPROCS", "GODEBUG", "GOMEMLIMIT",
			"GORACE":
			logging.Prelog("<yellow>Environment Variable %v:</> %v",
				pair[0], pair[1])
		}
	}
}
