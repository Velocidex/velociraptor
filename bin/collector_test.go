package main

import (
	"archive/zip"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/Velocidex/yaml/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
	"www.velocidex.com/golang/velociraptor/config"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/constants"
	"www.velocidex.com/golang/velociraptor/file_store"
	"www.velocidex.com/golang/velociraptor/paths"
)

type CollectorTestSuite struct {
	suite.Suite
	binary      string
	extension   string
	tmpdir      string
	config_file string
	config_obj  *config_proto.Config
	test_server *httptest.Server
}

func (self *CollectorTestSuite) SetupTest() {
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

	self.binary, _ = filepath.Abs(binaries[0])
	fmt.Printf("Found binary %v\n", self.binary)

	self.tmpdir, err = ioutil.TempDir("", "tmp")
	assert.NoError(self.T(), err)

	self.config_file = filepath.Join(self.tmpdir, "server.config.yaml")
	fd, err := os.OpenFile(
		self.config_file, os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0700)
	assert.NoError(self.T(), err)

	self.config_obj, err = new(config.Loader).
		WithFileLoader("../http_comms/test_data/server.config.yaml").
		LoadAndValidate()
	assert.NoError(self.T(), err)

	self.config_obj.Datastore.Implementation = "FileBaseDataStore"
	self.config_obj.Datastore.Location = self.tmpdir
	self.config_obj.Datastore.FilestoreDirectory = self.tmpdir
	self.config_obj.Frontend.DoNotCompressArtifacts = true

	// Start a web server that serves the filesystem
	self.test_server = httptest.NewServer(
		http.FileServer(http.Dir(filepath.Dir(self.binary))))

	// Set the server URL correctly.
	self.config_obj.Client.ServerUrls = []string{
		self.test_server.URL + "/",
	}

	serialized, err := yaml.Marshal(self.config_obj)
	assert.NoError(self.T(), err)

	fd.Write(serialized)
	fd.Close()

}

func (self *CollectorTestSuite) TearDownTest() {
	os.RemoveAll(self.tmpdir)
	self.test_server.Close()
}

func (self *CollectorTestSuite) TestCollector() {
	OS_TYPE := "Linux"
	if runtime.GOOS == "windows" {
		OS_TYPE = "Windows"
	} else if runtime.GOOS == "darwin" {
		OS_TYPE = "Darwin"
	}

	// Change into the tmpdir
	old_dir, _ := os.Getwd()
	defer os.Chdir(old_dir)

	os.Chdir(self.tmpdir)

	// Create a new artifact..
	file_store_factory := file_store.GetFileStore(self.config_obj)

	fd, err := file_store_factory.WriteFile(paths.GetArtifactDefintionPath(
		"Custom.TestArtifactDependent"))
	assert.NoError(self.T(), err)

	fd.Truncate()
	fd.Write([]byte(`name: Custom.TestArtifactDependent
tools:
  - name: MyTool

sources:
 - query: |
     LET binary <= SELECT FullPath, Name
         FROM Artifact.Generic.Utils.FetchBinary(
              ToolName="MyTool", SleepDuration='0')
     SELECT "Foobar", Stdout, binary[0].Name
     FROM execve(argv=[binary[0].FullPath, "artifacts", "list"])
`))
	fd.Close()

	fd, err = file_store_factory.WriteFile(
		paths.GetArtifactDefintionPath("Custom.TestArtifact"))
	assert.NoError(self.T(), err)

	fd.Truncate()
	fd.Write([]byte(`name: Custom.TestArtifact
parameters:
- name: MyParameter
  default: DefaultMyParameter
- name: MyDefaultParameter
  default: DefaultMyDefaultParameter
sources:
 - query: |
     SELECT *, MyParameter, MyDefaultParameter
     FROM Artifact.Custom.TestArtifactDependent()

reports:
 - type: HTML
   template: |
     <html><body><h1>This is the html report template</h1> {{ .main \
            }} </body></html>

 - type: CLIENT
   template: |
     # This is the report.

     {{ Query "SELECT * FROM source()" | Table }}

     {{ $foundit := Query "SELECT * FROM source()" | Expand }}

     {{ if $foundit }}

     ## Found a Scheduled Task

     {{ else }}

     ## Did not find a Scheduled Task!

     {{ end }}
`))
	fd.Close()
	cmd := exec.Command(self.binary, "--config", self.config_file,
		"artifacts", "show", "Custom.TestArtifact")
	out, err := cmd.CombinedOutput()
	fmt.Println(string(out))
	require.NoError(self.T(), err)

	var os_name string
	for _, os_name = range []string{"Windows", "Windows_x86", "Linux", "Darwin"} {
		cmd = exec.Command(self.binary, "--config", self.config_file,
			"tools", "upload", "--name", "Velociraptor"+os_name,
			self.config_file,
			"--serve_remote")
		out, err = cmd.CombinedOutput()
		fmt.Println(string(out))
		require.NoError(self.T(), err)
	}

	switch runtime.GOOS {
	case "windows":
		os_name = "Windows"
	case "linux":
		os_name = "Linux"
	case "darwin":
		os_name = "Darwin"
	}

	cmd = exec.Command(self.binary, "--config", self.config_file,
		"tools", "upload", "--name", "Velociraptor"+os_name,
		self.test_server.URL+"/"+filepath.Base(self.binary),
		"--serve_remote")
	out, err = cmd.CombinedOutput()
	fmt.Println(string(out))
	require.NoError(self.T(), err)

	// Make sure the binary is proprly added.
	assert.Regexp(self.T(), "name: Velociraptor", string(out))

	// Not served locally - download on demand should have no hash
	// and serve_locally should be false.
	assert.NotRegexp(self.T(), "serve_locally", string(out))
	assert.NotRegexp(self.T(), "hash: .+", string(out))

	cmd = exec.Command(self.binary, "--config", self.config_file,
		"tools", "upload", "--name", "MyTool",
		self.test_server.URL+"/"+filepath.Base(self.binary),
		"--serve_remote")
	out, err = cmd.CombinedOutput()
	fmt.Println(string(out))
	require.NoError(self.T(), err)

	output_zip := filepath.Join(self.tmpdir, "output.zip")

	// Now we want to create a stand alone collector. We do this
	// by collecting the Server.Utils.CreateCollector artifact
	cmdline := []string{"--config", self.config_file, "-v",
		"artifacts", "collect", "Server.Utils.CreateCollector",
		"--args", "OS=" + OS_TYPE,
		"--args", "artifacts=[\"Custom.TestArtifact\"]",
		"--args", "parameters={\"Custom.TestArtifact\":{\"MyParameter\": \"MyValue\"}}",
		"--args", "target=ZIP",
		"--args", "opt_admin=N",
		"--args", "opt_prompt=N",
		"--args", "template=Custom.TestArtifact",
		"--output", output_zip,
	}

	cmd = exec.Command(self.binary, cmdline...)
	out, err = cmd.CombinedOutput()
	fmt.Println(string(out))
	require.NoError(self.T(), err)

	r, err := zip.OpenReader(output_zip)
	assert.NoError(self.T(), err)

	defer r.Close()

	// Iterate through the files in the archive,
	// printing some of their contents.
	output_executable := filepath.Join(self.tmpdir, "collector"+self.extension)
	for _, f := range r.File {
		fmt.Printf("Contents of collector:  %s (%v bytes)\n",
			f.Name, f.UncompressedSize)
		if strings.HasPrefix(f.Name, "Collector") {
			fmt.Printf("Extracting %v to %v\n", f.Name, output_executable)

			rc, err := f.Open()
			assert.NoError(self.T(), err)

			out_fd, err := os.OpenFile(
				output_executable, os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0700)
			assert.NoError(self.T(), err)

			n, err := io.Copy(out_fd, rc)
			assert.NoError(self.T(), err)
			rc.Close()
			out_fd.Close()

			fmt.Printf("Copied %v bytes\n", n)
		}
	}

	// Now just run the executable.
	fmt.Printf("Config show\n")
	cmd = exec.Command(output_executable, "config", "show")
	out, err = cmd.CombinedOutput()
	fmt.Println(string(out))
	require.NoError(self.T(), err)

	// Now just run the executable.
	cmd = exec.Command(output_executable)
	out, err = cmd.CombinedOutput()
	fmt.Println(string(out))
	require.NoError(self.T(), err)

	// There should be a collection now.
	zip_files, err := filepath.Glob("Collection-*.zip")
	assert.NoError(self.T(), err)
	assert.Equal(self.T(), 1, len(zip_files))

	// Inspect the collection zip file - there should be a single
	// artifact output from our custom artifact, and the data it
	// produces should have the string Foobar in it.
	r, err = zip.OpenReader(zip_files[0])
	assert.NoError(self.T(), err)

	assert.True(self.T(), len(r.File) > 0)

	for _, f := range r.File {
		fmt.Printf("Contents of %s:\n", f.Name)
		assert.Equal(self.T(), f.Name, "Custom.TestArtifact.json")

		rc, err := f.Open()
		assert.NoError(self.T(), err)

		data, err := ioutil.ReadAll(rc)
		assert.NoError(self.T(), err)
		assert.Contains(self.T(), string(data), "Foobar")
	}

	// Inspect the produced HTML report.
	html_files, err := filepath.Glob("Collection-*.html")
	assert.NoError(self.T(), err)
	assert.Equal(self.T(), 1, len(html_files))

	html_fd, err := os.Open(html_files[0])
	assert.NoError(self.T(), err)

	data, err := ioutil.ReadAll(html_fd)
	assert.NoError(self.T(), err)

	// Ensure the report contains the data that was passed.
	assert.Contains(self.T(), string(data), "MyValue")

	// And the default parameter is still there.
	assert.Contains(self.T(), string(data), "DefaultMyDefaultParameter")

	assert.Contains(self.T(), string(data), "Foobar")
	assert.Contains(self.T(), string(data), "This is the report")

	// Make sure we found the artifact in the report
	assert.Contains(self.T(), string(data), "Windows.System.TaskScheduler")
	assert.Contains(self.T(), string(data), "Found a Scheduled Task")

	// Check that we used the default template from the
	// Reporting.Default artifact:
	assert.Contains(self.T(), string(data), "<html>")
	assert.Contains(self.T(), string(data), "This is the html report template")

	// fmt.Println(string(data))
}

func TestCollector(t *testing.T) {
	suite.Run(t, &CollectorTestSuite{})
}
