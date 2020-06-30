package main

import (
	"archive/zip"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/Velocidex/yaml/v2"
	"github.com/alecthomas/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
	"www.velocidex.com/golang/velociraptor/config"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/constants"
	"www.velocidex.com/golang/velociraptor/file_store"
)

type CollectorTestSuite struct {
	suite.Suite
	binary      string
	extension   string
	tmpdir      string
	config_file string
	config_obj  *config_proto.Config
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

	serialized, err := yaml.Marshal(self.config_obj)
	assert.NoError(self.T(), err)

	fd.Write(serialized)
	fd.Close()

	// Start a web server that serves the filesystem
	go func() {
		http.Handle("/", http.FileServer(http.Dir(filepath.Dir(self.binary))))
		err = http.ListenAndServe("localhost:8000", nil)
		assert.NoError(self.T(), err)
	}()
}

func (self *CollectorTestSuite) TearDownTest() {
	os.RemoveAll(self.tmpdir)
}

func (self *CollectorTestSuite) TestCollector() {
	OS_TYPE := "Linux"
	if runtime.GOOS == "windows" {
		OS_TYPE = "Windows"
	}

	// Change into the tmpdir
	old_dir, _ := os.Getwd()
	defer os.Chdir(old_dir)

	os.Chdir(self.tmpdir)

	// Create a new artifact..
	file_store_factory := file_store.GetFileStore(self.config_obj)
	fd, err := file_store_factory.WriteFile("/artifact_definitions/Custom/TestArtifact.yaml")
	assert.NoError(self.T(), err)

	fd.Truncate()
	fd.Write([]byte(`name: Custom.TestArtifact
sources:
 - query: SELECT "Foobar" FROM scope()

reports:
 - type: CLIENT
   template: |
     # This is the report.

     {{ Query "SELECT * FROM source()" | Table }}
`))
	fd.Close()

	cmd := exec.Command(self.binary, "--config", self.config_file,
		"artifacts", "show", "Custom.TestArtifact")
	out, err := cmd.CombinedOutput()
	fmt.Println(string(out))
	require.NoError(self.T(), err)

	cmd = exec.Command(self.binary, "--config", self.config_file,
		"third_party", "upload", "--name", "Velociraptor"+OS_TYPE,
		"http://localhost:8000/"+filepath.Base(self.binary),
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

	output_zip := filepath.Join(self.tmpdir, "output.zip")

	// Now we want to create a stand alone collector. We do this
	// by collecting the Server.Utils.CreateCollector artifact
	cmdline := []string{"--config", self.config_file, "-v",
		"artifacts", "collect", "Server.Utils.CreateCollector",
		"--args", "OS=" + OS_TYPE,
		"--args", "artifacts=[\"Custom.TestArtifact\"]",
		"--args", "target=ZIP",
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
		fmt.Printf("Contents of %s:\n", f.Name)
		fmt.Println()
		if strings.HasPrefix(f.Name, "Collector") {
			fmt.Printf("Copying %v\n", f.Name)

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
	assert.Contains(self.T(), string(data), "Foobar")
	assert.Contains(self.T(), string(data), "This is the report")
}

func TestCollector(t *testing.T) {
	suite.Run(t, &CollectorTestSuite{})
}
