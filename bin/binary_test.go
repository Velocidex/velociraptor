// Tests for the binary.

package main_test

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"sync"
	"testing"
	"time"

	"github.com/Velocidex/yaml/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"www.velocidex.com/golang/velociraptor/config"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/constants"
	"www.velocidex.com/golang/velociraptor/json"
	"www.velocidex.com/golang/velociraptor/utils/tempfile"
)

var (
	mu        sync.Mutex
	binary    string
	extension string
	cwd       string
)

func SetupTest(t *testing.T) (string, string) {
	t.Parallel()

	if testing.Short() {
		t.Skip("skipping test in short mode.")
	}

	mu.Lock()
	defer mu.Unlock()

	if binary != "" {
		return binary, extension
	}

	if runtime.GOOS == "windows" {
		extension = ".exe"
	}

	// Search for a valid binary to run.
	binaries, err := filepath.Glob(
		filepath.Join(cwd, "..", "output",
			"velociraptor*"+constants.VERSION+"-"+runtime.GOOS+
				"-"+runtime.GOARCH+extension))
	assert.NoError(t, err)

	if len(binaries) == 0 {
		binaries, _ = filepath.Glob(
			filepath.Join(cwd, "..", "output", "velociraptor*"+extension))
	}

	binary = binaries[0]
	fmt.Printf("Found binary %v\n", binary)

	return binary, extension
}

func TestHelp(t *testing.T) {
	binary, _ := SetupTest(t)

	cmd := exec.Command(binary, "--help")
	out, err := cmd.CombinedOutput()
	require.NoError(t, err)
	require.Contains(t, string(out), "usage:")
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

func TestAutoexec(t *testing.T) {
	binary, extension := SetupTest(t)

	// Create a tempfile for the repacked binary.
	exe, err := tempfile.TempFile("exe*" + extension)
	assert.NoError(t, err)

	defer os.Remove(exe.Name())
	exe.Close()

	// A temp file for the config.
	config_file, err := tempfile.TempFile("config")
	assert.NoError(t, err)

	defer os.Remove(config_file.Name())
	config_file.Write([]byte(autoexec_file))
	config_file.Close()

	// Repack the config in the binary.
	cmd := exec.Command(binary,
		"config", "repack", config_file.Name(), exe.Name(), "-v")
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, string(out))

	os.Chmod(exe.Name(), 0755)

	// Run the repacked binary with no args - it should run the
	// `artifacts list` command.
	cmd = exec.Command(exe.Name())
	out, err = cmd.CombinedOutput()
	require.NoError(t, err, string(out))

	// The output should contain MySpecialArtifact as well as the
	// standard artifacts.
	require.Contains(t, string(out), "MySpecialArtifact")
	require.Contains(t, string(out), "Windows.Sys.Users")

	// If provided args it works normally.
	cmd = exec.Command(exe.Name(),
		"artifacts", "collect", "Windows.Sys.Interfaces", "--format", "jsonl")
	out, err = cmd.CombinedOutput()
	require.NoError(t, err, string(out))

	// Config artifacts override built in artifacts.
	require.Contains(t, string(out), "MySpecialInterface")
}

const sleepDefinitions = `
autoexec:
  artifact_definitions:
  - name: Sleep
    sources:
    - query: SELECT sleep(time=100) FROM scope()
`

func TestTimeout(t *testing.T) {
	binary, _ := SetupTest(t)

	// A temp file for the config.
	config_file, err := tempfile.TempFile("config")
	assert.NoError(t, err)

	defer os.Remove(config_file.Name())
	config_file.Write([]byte(sleepDefinitions))
	config_file.Close()

	// Repack the config in the binary.
	cmd := exec.Command(binary,
		"--config", config_file.Name(),
		"artifacts", "collect", "Sleep", "-v", "--timeout", "0.1")
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, string(out))
	assert.Regexp(t, "Starting collection of Sleep", string(out))

	// Make sure the collection timed out.
	assert.Regexp(t, "Timeout Error: Collection timed out after", string(out))
}

func TestProgressTimeout(t *testing.T) {
	binary, _ := SetupTest(t)

	// A temp file for the config.
	config_file, err := tempfile.TempFile("config")
	assert.NoError(t, err)

	defer os.Remove(config_file.Name())
	config_file.Write([]byte(sleepDefinitions))
	config_file.Close()

	// Repack the config in the binary.
	start := time.Now()
	cmd := exec.Command(binary,
		"--config", config_file.Name(),
		"artifacts", "collect", "Sleep", "-v", "--progress_timeout", "0.1")
	out, err := cmd.CombinedOutput()
	require.Error(t, err, string(out))
	assert.Regexp(t, "Starting collection of Sleep", string(out))

	// Make sure the collection timed out and dumped the goroutines.
	assert.Regexp(t, "Goroutine dump: goroutine profile", string(out))
	assert.Regexp(t, "Mutex dump:", string(out))

	// Make sure the query was cancelled quickly without running the
	// full length of the sleep.
	fmt.Printf("Now is %v started at %v\n", time.Now(), start)
	assert.Less(t, int(time.Now().Unix()-start.Unix()), int(20))
}

const cpulimitDefinitions = `
autoexec:
  artifact_definitions:
  - name: GoHard
    sources:
    - query: |
        SELECT * FROM foreach(
          row={ SELECT * FROM range(start=0, end=100000, step=1) },
          query={
              SELECT * FROM range(start=0, end=100000, step=1)
              WHERE FALSE
          }, workers=20)
`

func TestCPULimit(t *testing.T) {
	binary, _ := SetupTest(t)

	// A temp file for the config.
	config_file, err := tempfile.TempFile("config")
	assert.NoError(t, err)

	defer os.Remove(config_file.Name())
	config_file.Write([]byte(cpulimitDefinitions))
	config_file.Close()

	// Run the test with cpu limiting. For this test we timeout
	// immediately but you can increase the timeout manually to
	// observe the cpu limiting in action.
	cmd := exec.Command(binary,
		"--config", config_file.Name(),
		"artifacts", "collect", "GoHard",
		"-v", "--cpu_limit", "5", "--timeout", "0.1")
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, string(out))
	assert.Regexp(t, "Starting collection of GoHard", string(out))

	// Make sure the collection timed out and dumped the goroutines.
	assert.Regexp(t, "Will throttle query to 5 percent of", string(out))
}

var (
	client_deb_regex = regexp.MustCompile("client.+\\.deb$")
	server_deb_regex = regexp.MustCompile("server.+\\.deb$")
)

func TestBuildDeb(t *testing.T) {
	binary, _ := SetupTest(t)

	tempdir, err := tempfile.TempDir("TestBuildDeb")
	assert.NoError(t, err)

	defer os.RemoveAll(tempdir)

	// A temp file for the generated config.
	config_file, err := os.OpenFile(filepath.Join(tempdir, "config"),
		os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0600)

	assert.NoError(t, err)
	defer os.Remove(config_file.Name())

	cmd := exec.Command(
		binary, "config", "generate", "--merge",
		`{"Client": {"nonce": "Foo", "writeback_linux": "some_location"}}`)
	out, err := cmd.Output()
	require.NoError(t, err)

	// Write the config to the tmp file
	config_file.Write(out)
	config_file.Close()

	binary_file, _ := filepath.Abs("../artifacts/testdata/files/test.elf")

	cmd = exec.Command(
		binary, "--config", config_file.Name(),
		"debian", "client", "--binary", binary_file,
		"--output", tempdir)
	out, err = cmd.CombinedOutput()
	require.NoError(t, err, string(out))

	output_file, err := tempfile.FindFile(tempdir, client_deb_regex)
	assert.NoError(t, err)

	// Make sure the file is written
	fd, err := os.Open(output_file)
	assert.NoError(t, err)

	stat, err := fd.Stat()
	assert.NoError(t, err)

	assert.Greater(t, stat.Size(), int64(0))

	// Now the server deb
	cmd = exec.Command(
		binary, "--config", config_file.Name(),
		"debian", "server", "--binary", binary_file,
		"--output", tempdir)
	out, err = cmd.Output()
	require.NoError(t, err, string(out))

	output_file, err = tempfile.FindFile(tempdir, server_deb_regex)
	assert.NoError(t, err)

	// Make sure the file is written
	fd, err = os.Open(output_file)
	assert.NoError(t, err)

	stat, err = fd.Stat()
	assert.NoError(t, err)

	assert.Greater(t, stat.Size(), int64(0))
}

func TestGenerateConfigWithMerge(t *testing.T) {
	binary, extension := SetupTest(t)

	// A temp file for the generated config.
	config_file, err := tempfile.TempFile("config")
	assert.NoError(t, err)
	defer os.Remove(config_file.Name())

	cmd := exec.Command(
		binary, "config", "generate", "--merge",
		`{"Client": {"nonce": "Foo", "writeback_linux": "some_location"}}`)
	out, err := cmd.Output()
	require.NoError(t, err, string(out))

	// Write the config to the tmp file
	config_file_content := out
	config_file.Write(out)
	config_file.Close()

	// Try to load it now.
	config_obj, err := new(config.Loader).WithFileLoader(config_file.Name()).
		WithRequiredFrontend().WithRequiredCA().WithRequiredClient().
		LoadAndValidate()
	require.NoError(t, err)

	require.Equal(t, config_obj.Client.Nonce, "Foo")
	require.Equal(t, config_obj.Client.WritebackLinux, "some_location")

	// Try to show a config without a flag - should result in an error
	cmd = exec.Command(binary, "config", "show")
	cmd.Env = append(os.Environ(),
		"VELOCIRAPTOR_CONFIG=",
	)
	out, err = cmd.Output()
	require.Error(t, err, string(out))

	// Specify the config on the commandline - should load correctly
	cmd = exec.Command(binary, "config", "show", "--config", config_file.Name())
	cmd.Env = append(os.Environ(),
		"VELOCIRAPTOR_CONFIG=",
	)
	out, err = cmd.Output()
	require.NoError(t, err, string(out))
	require.Contains(t, string(out), "Foo")

	// Specify the config in the environment
	cmd = exec.Command(binary, "config", "show")
	cmd.Env = append(os.Environ(),
		"VELOCIRAPTOR_CONFIG="+config_file.Name(),
	)
	out, err = cmd.Output()
	require.NoError(t, err)
	require.Contains(t, string(out), "Foo")

	// Specify the literal config in the environment
	cmd = exec.Command(binary, "config", "show")
	cmd.Env = append(os.Environ(),
		"VELOCIRAPTOR_LITERAL_CONFIG="+string(config_file_content),
		"VELOCIRAPTOR_CONFIG=",
	)
	out, err = cmd.Output()
	require.NoError(t, err)
	require.Contains(t, string(out), "Foo")

	// Specifying invalid config in the flag is a hard stop - even
	// if there is a valid environ.
	cmd = exec.Command(binary, "config", "show", "--config", "XXXX")
	cmd.Env = append(os.Environ(),
		"VELOCIRAPTOR_CONFIG="+config_file.Name(),
	)
	out, err = cmd.Output()
	require.Error(t, err)

	// Create a tempfile for the repacked binary.
	exe, err := tempfile.TempFile("exe*" + extension)
	assert.NoError(t, err)

	defer os.Remove(exe.Name())
	exe.Close()

	// Repack the config in the binary.
	cmd = exec.Command(binary, "config", "repack", config_file.Name(), exe.Name())
	out, err = cmd.CombinedOutput()
	require.NoError(t, err, string(out))

	os.Chmod(exe.Name(), 0755)

	// Run the repacked binary with invalid environ - config
	// should come from embedded.
	cmd = exec.Command(exe.Name(), "config", "show")
	cmd.Env = append(os.Environ(),
		"VELOCIRAPTOR_CONFIG=XXXX",
	)
	out, err = cmd.Output()
	require.NoError(t, err)
	require.Contains(t, string(out), "Foo")

	// Make second copy of config file and store modified version
	second_config_file, err := tempfile.TempFile("config")
	assert.NoError(t, err)

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
	require.NoError(t, err)
	require.Contains(t, string(out), "Foo")

	// If a config is explicitly specified, it will override even the
	// embedded config.
	cmd = exec.Command(exe.Name(), "config", "show",
		"--config", second_config_file.Name())
	out, err = cmd.Output()
	require.NoError(t, err)

	// loaded config contains Bar
	require.Contains(t, string(out), "Bar")
}

func TestShowConfigWithMergePatch(t *testing.T) {
	binary, _ := SetupTest(t)

	// A temp file for the generated config.
	config_file, err := tempfile.TempFile("config")
	assert.NoError(t, err)

	defer os.Remove(config_file.Name())

	// Write the config to the tmp file
	config_obj := config.GetDefaultConfig()
	config_obj.GUI = nil
	config_obj.Frontend = nil
	config_obj.Client.CaCertificate = "A"
	config_obj.Client.Nonce = "A"
	config_obj.Client.ServerUrls = []string{"https://FirstServer/"}

	serialized, err := json.Marshal(config_obj)
	assert.NoError(t, err)

	config_file.Write(serialized)
	config_file.Close()

	// This merge removes the FirstServer from the ServerUrls and
	// replaces the Nonce With Foo, then adds another server to the
	// urls: Merges are done first, then patches.
	cmd := exec.Command(
		binary, "config", "show", "--config", config_file.Name(), "-v",
		"--merge",
		`{"Client": {"nonce": "Foo", "server_urls": ["https://192.168.1.11:8000/"]}}`,
		"--patch",
		`[{"op": "add", "path": "/Client/server_urls/0", "value": "https://SomeServer/"}]`,
	)
	out, err := cmd.Output()
	if err != nil {
		fmt.Println(string(err.(*exec.ExitError).Stderr))
	}
	require.NoError(t, err, string(out))

	// Try to load it now.
	new_config := &config_proto.Config{}
	err = yaml.Unmarshal(out, new_config)
	assert.NoError(t, err)

	// Nonce should be replace by json merge
	require.Equal(t, new_config.Client.Nonce, "Foo")
	require.Equal(t, 2, len(new_config.Client.ServerUrls))
	require.Equal(t, "https://SomeServer/", new_config.Client.ServerUrls[0])
	require.Equal(t, "https://192.168.1.11:8000/", new_config.Client.ServerUrls[1])
}

func init() {
	cwd, _ = os.Getwd()
}
