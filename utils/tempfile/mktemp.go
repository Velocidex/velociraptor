package tempfile

/*
  This is a workaround to the introduction of GetTempPath2 in
  Windows. The Golang standard library switched to this API in recent
  Go version https://github.com/golang/go/issues/56899

  The GetTempPath2 API ignores the TEMP environment variable and
  always returns C:\Windows\SystemTemp for system level programs. This
  causes the os.TempDir() API to ignore our configuration of the
  correct temp location.

  Since practically, we can not allow list the SystemTemp directory in
  e.g. EDR or other security products, this bug forces Velociraptor to
  write temp files in this common area ignoring the configuration
  file.

  To work around this issue we wrap all calls to temporary files and
  ignore the os.TempDir() API completely.
*/

import (
	"io/ioutil"
	"os"
	"runtime"
	"sync"

	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/logging"
	"www.velocidex.com/golang/velociraptor/utils"
)

var (
	mu sync.Mutex

	g_tempdir string = os.TempDir()
)

func GetTempDir() string {
	mu.Lock()
	defer mu.Unlock()

	return g_tempdir
}

// Wrap ioutil.TempFile to ensure that temp files are always written
// in the correct directory.
func TempFile(pattern string) (f *os.File, err error) {
	// Force the temporary file to be placed in the global tempdir
	return ioutil.TempFile(GetTempDir(), pattern)
}

// Wrap ioutil.TempDir to ensure that temp files are always written
// in the correct directory.
func TempDir(pattern string) (string, error) {
	// Force the temporary file to be placed in the global tempdir
	return os.MkdirTemp(GetTempDir(), pattern)
}

func CreateTemp(pattern string) (f *os.File, err error) {
	return os.CreateTemp(GetTempDir(), pattern)
}

func SetTempDir(path string) error {
	mu.Lock()
	defer mu.Unlock()

	// Try to create a file in the directory to make sure we have
	// permissions and the directory exists.
	tmpfile, err := ioutil.TempFile(path, "tmp")
	if err != nil {
		return err
	}
	tmpfile.Close()

	defer os.Remove(tmpfile.Name())

	g_tempdir = path

	return nil
}

// Calling SetTempfile() will change the default directory for all
// temporary files.
func SetTempfile(config_obj *config_proto.Config) {
	if config_obj.Client != nil {
		tmpdir := ""
		logger := logging.GetLogger(config_obj, &logging.ClientComponent)

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

		// Fall back to system default
		if tmpdir == "" {
			tmpdir = os.TempDir()
		}

		// Expand the tmpdir if needed.
		tmpdir = utils.ExpandEnv(tmpdir)

		// Try to create a file in the directory to make sure
		// we have permissions and the directory exists.
		tmpfile, err := ioutil.TempFile(tmpdir, "tmp")
		if err == nil {

			// Remove the file now - assume future tempfiles will
			// work.
			tmpfile.Close()
			os.Remove(tmpfile.Name())

		} else {
			logger.Error("Unable to write to configured temp dir %v - falling back to %v",
				tmpdir, os.TempDir())

			// No we dont have permission there, fall back to system
			// default, that is the best we can do - we hope we can
			// write there.
			tmpdir = os.TempDir()
		}

		// Set the env vars the same on all platforms to be consistent
		// across OSs
		os.Setenv("TMP", tmpdir)
		os.Setenv("TEMP", tmpdir)
		os.Setenv("TMPDIR", tmpdir)

		mu.Lock()
		g_tempdir = tmpdir
		mu.Unlock()

		logger.Info("Setting temp directory to <green>%v", tmpdir)
	}
}
