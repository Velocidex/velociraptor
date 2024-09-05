package main_test

import (
	"io/ioutil"
	"os"
	"os/exec"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"www.velocidex.com/golang/velociraptor/utils/tempfile"
)

func TestGenerateAPIConfig(t *testing.T) {
	binary, _ := SetupTest(t)

	// A temp file for the generated config.
	config_file, err := tempfile.TempFile("config")
	assert.NoError(t, err)
	defer os.Remove(config_file.Name())

	// Make a config file
	cmd := exec.Command(binary, "config", "generate")
	out, err := cmd.Output()
	require.NoError(t, err)

	// Write the config to the tmp file
	config_file.Write(out)
	config_file.Close()

	api_config_file, err := tempfile.TempFile("api_config")
	assert.NoError(t, err)
	defer os.Remove(api_config_file.Name())

	// Now generate an API config based on that.
	cmd = exec.Command(binary, "--config", config_file.Name(),
		"config", "api_client",
		"--name", "api_user", "--role", "administrator",
		api_config_file.Name())
	out, err = cmd.CombinedOutput()
	require.NoError(t, err, string(out))

	fd, err := os.Open(api_config_file.Name())
	assert.NoError(t, err)
	defer fd.Close()

	data, err := ioutil.ReadAll(fd)
	assert.NoError(t, err)

	assert.Regexp(t, "name: api_user", string(data))
}
