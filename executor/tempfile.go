package executor

import (
	"io/ioutil"
	"os"
	"runtime"

	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/logging"
	"www.velocidex.com/golang/velociraptor/utils"
)

func SetTempfile(config_obj *config_proto.Config) {
	if config_obj.Client != nil {
		tmpdir := ""

		// We must set some temp directory that is reasonable even if
		// the config does not specify.
		switch runtime.GOOS {
		case "windows":
			tmpdir = config_obj.Client.TempdirWindows

		case "linux":
			tmpdir = config_obj.Client.TempdirLinux

		case "darwin":
			tmpdir = config_obj.Client.TempdirDarwin

		}

		if tmpdir == "" {
			tmpdir = os.TempDir()
		}

		// Expand the tmpdir if needed.
		tmpdir = utils.ExpandEnv(tmpdir)

		// Try to create a file in the directory to make sure
		// we have permissions and the directory exists.
		tmpfile, err := ioutil.TempFile(tmpdir, "tmp")
		if err == nil {
			defer os.Remove(tmpfile.Name())

		} else {
			// No we dont have permission there, fall back to system
			// default, that is the best we can do we hope we can
			// write there.
			tmpdir = os.TempDir()
		}

		// Set the env vars the same on all platforms to be consistent
		// across OSs
		os.Setenv("TMP", tmpdir)
		os.Setenv("TEMP", tmpdir)
		os.Setenv("TMPDIR", tmpdir)

		logger := logging.GetLogger(config_obj, &logging.ClientComponent)
		logger.Info("Setting temp directory to <green>%v", tmpdir)
	}
}
