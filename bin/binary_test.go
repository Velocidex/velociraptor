// Tests for the binary.

package main

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
	"www.velocidex.com/golang/velociraptor/config"
	"www.velocidex.com/golang/velociraptor/constants"
)

type MainTestSuite struct {
	suite.Suite
	binary    string
	extension string
}

func (self *MainTestSuite) SetupTest() {
	if runtime.GOOS == "windows" {
		self.extension = ".exe"
	}

	// Search for a valid binary to run.
	binaries, err := filepath.Glob(
		"../output/velociraptor*" + constants.VERSION + "-" + runtime.GOOS +
			"-" + runtime.GOARCH + self.extension)
	assert.NoError(self.T(), err)

	if len(binaries) == 0 {
		binaries, _ = filepath.Glob("../output/velociraptor*" +
			self.extension)
	}

	self.binary = binaries[0]
	fmt.Printf("Found binary %v\n", self.binary)
}

func (self *MainTestSuite) TestHelp() {
	cmd := exec.Command(self.binary, "--help")
	out, err := cmd.CombinedOutput()
	require.NoError(self.T(), err)
	require.Contains(self.T(), string(out), "usage:")
}

const autoexec_file = `
autoexec:
  argv:
    - artifacts
    - list
  artifact_definitions:
  - name: MySpecialArtifact
    sources:
    - query: SELECT * FROM info()
  - name: Windows.Sys.Interfaces
    description: Override a built in artifact in the config.
    sources:
    - query: SELECT "MySpecialInterface" FROM scope()
`

func (self *MainTestSuite) TestAutoexec() {
	// Create a tempfile for the repacked binary.
	exe, err := ioutil.TempFile("", "exe*"+self.extension)
	assert.NoError(self.T(), err)

	defer os.Remove(exe.Name())
	exe.Close()

	// A temp file for the config.
	config_file, err := ioutil.TempFile("", "config")
	assert.NoError(self.T(), err)

	defer os.Remove(config_file.Name())
	config_file.Write([]byte(autoexec_file))
	config_file.Close()

	// Repack the config in the binary.
	cmd := exec.Command(self.binary,
		"config", "repack", config_file.Name(), exe.Name())
	out, err := cmd.CombinedOutput()
	require.NoError(self.T(), err, string(out))

	// Run the repacked binary with no args - it should run the
	// `artifacts list` command.
	cmd = exec.Command(exe.Name())
	out, err = cmd.CombinedOutput()
	require.NoError(self.T(), err, string(out))

	// The output should contain MySpecialArtifact as well as the
	// standard artifacts.
	require.Contains(self.T(), string(out), "MySpecialArtifact")
	require.Contains(self.T(), string(out), "Windows.Sys.Users")

	// If provided args it works normally.
	cmd = exec.Command(exe.Name(),
		"artifacts", "collect", "Windows.Sys.Interfaces", "--format", "jsonl")
	out, err = cmd.Output()
	require.NoError(self.T(), err)

	// Config artifacts override built in artifacts.
	require.Contains(self.T(), string(out), "MySpecialInterface")
}

func (self *MainTestSuite) TestBuildDeb() {
	// A temp file for the generated config.
	config_file, err := ioutil.TempFile("", "config")
	assert.NoError(self.T(), err)
	defer os.Remove(config_file.Name())

	cmd := exec.Command(
		self.binary, "config", "generate", "--merge",
		`{"Client": {"nonce": "Foo", "writeback_linux": "some_location"}}`)
	out, err := cmd.Output()
	require.NoError(self.T(), err)

	// Write the config to the tmp file
	config_file.Write(out)
	config_file.Close()

	// Create a tempfile for the binary executable.
	binary_file, err := ioutil.TempFile("", "binary")
	assert.NoError(self.T(), err)

	defer os.Remove(binary_file.Name())
	binary_file.Write([]byte("\x7f\x45\x4c\x46XXXXXXXXXX"))
	binary_file.Close()

	output_file, err := ioutil.TempFile("", "output*.deb")
	assert.NoError(self.T(), err)
	output_file.Close()
	defer os.Remove(output_file.Name())

	cmd = exec.Command(
		self.binary, "--config", config_file.Name(),
		"debian", "client", "--binary", binary_file.Name(),
		"--output", output_file.Name())
	_, err = cmd.Output()
	require.NoError(self.T(), err)

	// Make sure the file is written
	fd, err := os.Open(output_file.Name())
	assert.NoError(self.T(), err)

	stat, err := fd.Stat()
	assert.NoError(self.T(), err)

	assert.Greater(self.T(), stat.Size(), int64(0))

	// Now the server deb
	output_file, err = ioutil.TempFile("", "output*.deb")
	assert.NoError(self.T(), err)
	output_file.Close()
	defer os.Remove(output_file.Name())

	cmd = exec.Command(
		self.binary, "--config", config_file.Name(),
		"debian", "server", "--binary", binary_file.Name(),
		"--output", output_file.Name())
	_, err = cmd.Output()
	require.NoError(self.T(), err)

	// Make sure the file is written
	fd, err = os.Open(output_file.Name())
	assert.NoError(self.T(), err)

	stat, err = fd.Stat()
	assert.NoError(self.T(), err)

	assert.Greater(self.T(), stat.Size(), int64(0))
}

func (self *MainTestSuite) TestGenerateConfigWithMerge() {
	// A temp file for the generated config.
	config_file, err := ioutil.TempFile("", "config")
	assert.NoError(self.T(), err)

	defer os.Remove(config_file.Name())
	defer config_file.Close()

	cmd := exec.Command(
		self.binary, "config", "generate", "--merge",
		`{"Client": {"nonce": "Foo", "writeback_linux": "some_location"}}`)
	out, err := cmd.Output()
	require.NoError(self.T(), err)

	// Write the config to the tmp file
	config_file_content := out
	config_file.Write(out)
	config_file.Close()

	// Try to load it now.
	config_obj, err := new(config.Loader).WithFileLoader(config_file.Name()).
		WithRequiredFrontend().WithRequiredCA().WithRequiredClient().
		LoadAndValidate()
	require.NoError(self.T(), err)

	require.Equal(self.T(), config_obj.Client.Nonce, "Foo")
	require.Equal(self.T(), config_obj.Client.WritebackLinux, "some_location")

	// Try to show a config without a flag.
	cmd = exec.Command(self.binary, "config", "show")
	cmd.Env = append(os.Environ(),
		"VELOCIRAPTOR_CONFIG=",
	)
	out, err = cmd.Output()
	require.Error(self.T(), err)

	// Specify the config on the commandline
	cmd = exec.Command(self.binary, "config", "show", "--config", config_file.Name())
	cmd.Env = append(os.Environ(),
		"VELOCIRAPTOR_CONFIG=",
	)
	out, err = cmd.Output()
	require.NoError(self.T(), err)
	require.Contains(self.T(), string(out), "Foo")

	// Specify the config in the environment
	cmd = exec.Command(self.binary, "config", "show")
	cmd.Env = append(os.Environ(),
		"VELOCIRAPTOR_CONFIG="+config_file.Name(),
	)
	out, err = cmd.Output()
	require.NoError(self.T(), err)
	require.Contains(self.T(), string(out), "Foo")

	// Specifying invalid config in the flag is a hard stop - even
	// if there is a valid environ.
	cmd = exec.Command(self.binary, "config", "show", "--config", "XXXX")
	cmd.Env = append(os.Environ(),
		"VELOCIRAPTOR_CONFIG="+config_file.Name(),
	)
	out, err = cmd.Output()
	require.Error(self.T(), err)

	// Create a tempfile for the repacked binary.
	exe, err := ioutil.TempFile("", "exe*"+self.extension)
	assert.NoError(self.T(), err)

	defer os.Remove(exe.Name())
	exe.Close()

	// Repack the config in the binary.
	cmd = exec.Command(self.binary, "config", "repack", config_file.Name(), exe.Name())
	out, err = cmd.CombinedOutput()
	require.NoError(self.T(), err)

	// Run the repacked binary with invalid environ - config
	// should come from embedded.
	cmd = exec.Command(exe.Name(), "config", "show")
	cmd.Env = append(os.Environ(),
		"VELOCIRAPTOR_CONFIG=XXXX",
	)
	out, err = cmd.Output()
	require.NoError(self.T(), err)
	require.Contains(self.T(), string(out), "Foo")

	// Make second copy of config file and store modified version
	second_config_file, err := ioutil.TempFile("", "config")
	assert.NoError(self.T(), err)

	defer os.Remove(second_config_file.Name())

	// Second file has no Foo in it
	second_config_file_content := bytes.ReplaceAll(
		config_file_content, []byte(`Foo`), []byte(`Bar`))
	second_config_file.Write(second_config_file_content)
	second_config_file.Close()

	// Check that embedded binary with config will use its
	// embedded version, even if an env var is specified.
	cmd = exec.Command(exe.Name(), "config", "show")
	cmd.Env = append(os.Environ(),
		"VELOCIRAPTOR_CONFIG="+second_config_file.Name(),
	)
	out, err = cmd.Output()
	require.NoError(self.T(), err)
	require.Contains(self.T(), string(out), "Foo")

	// If a config is explicitly specified, it will override even the
	// embedded config.
	cmd = exec.Command(exe.Name(), "config", "show", "--config", second_config_file.Name())
	out, err = cmd.Output()
	require.NoError(self.T(), err)

	// loaded config contains Bar
	require.Contains(self.T(), string(out), "Bar")
}

func TestMainProgram(t *testing.T) {
	suite.Run(t, &MainTestSuite{})
}
