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

		switch runtime.GOOS {
		case "windows":
			tmpdir = config_obj.Client.TempdirWindows
		case "linux":
			tmpdir = config_obj.Client.TempdirLinux
		case "darwin":
			tmpdir = config_obj.Client.TempdirDarwin
		}

		if tmpdir == "" {
			return
		}

		// Expand the tmpdir if needed.
		tmpdir = utils.ExpandEnv(tmpdir)

		// Try to create a file in the directory to make sure
		// we have permissions and the directory exists.
		tmpfile, err := ioutil.TempFile(tmpdir, "tmp")
		if err != nil {
			// No we dont have permission there, fall back
			// to system default.
			return
		}
		defer os.Remove(tmpfile.Name())

		switch runtime.GOOS {
		case "windows":
			os.Setenv("TMP", tmpdir)
			os.Setenv("TEMP", tmpdir)
		case "linux", "darwin":
			os.Setenv("TMP", tmpdir)
			os.Setenv("TMPDIR", tmpdir)
		}

		logger := logging.GetLogger(config_obj, &logging.ClientComponent)
		logger.Info("Setting temp directory to <green>%v", tmpdir)
	}
}
