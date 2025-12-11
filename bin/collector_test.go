package main

import (
	"context"
	"fmt"
	"io"
	"io/ioutil"
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
	"www.velocidex.com/golang/velociraptor/third_party/zip"
	"www.velocidex.com/golang/velociraptor/utils"
	"www.velocidex.com/golang/velociraptor/utils/tempfile"
)

var (
	cwd string
)

func init() {
	cwd, _ = os.Getwd()
}

type CollectorTestSuite struct {
	suite.Suite

	binary      string
	extension   string
	tmpdir      string
	config_file string
	config_obj  *config_proto.Config

	OS_TYPE string
}

func (self *CollectorTestSuite) SetupSuite() {
	self.findAndPrepareBinary()
	self.addArtifactDefinitions()
	self.uploadToolDefinitions()
}

func (self *CollectorTestSuite) findAndPrepareBinary() {
	t := self.T()

	if runtime.GOOS == "windows" {
		self.extension = ".exe"
	}

	// Search for a valid binary to run.
	binaries, err := filepath.Glob(
		filepath.Join(cwd,
			"..", "output", "velociraptor*"+constants.VERSION+"-"+runtime.GOOS+
				"-"+runtime.GOARCH+self.extension))
	assert.NoError(t, err)

	if len(binaries) == 0 {
		binaries, _ = filepath.Glob(
			filepath.Join(cwd, "..", "output", "velociraptor*"+self.extension))
	}

	self.binary, _ = filepath.Abs(binaries[0])
	fmt.Printf("Found binary %v\n", self.binary)

	self.tmpdir, err = tempfile.TempDir("tmp")
	assert.NoError(t, err)

	// Copy the binary into the tmpdir
	dest_file := filepath.Join(self.tmpdir, filepath.Base(self.binary))
	err = utils.CopyFile(context.Background(), self.binary, dest_file, 0755)
	assert.NoError(t, err)

	self.binary = dest_file

	self.config_file = filepath.Join(self.tmpdir, "server.config.yaml")
	fd, err := os.OpenFile(
		self.config_file, os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0700)
	assert.NoError(t, err)

	self.config_obj, err = new(config.Loader).
		WithFileLoader("../http_comms/test_data/server.config.yaml").
		LoadAndValidate()
	assert.NoError(t, err)

	self.config_obj.Datastore.Implementation = "FileBaseDataStore"
	self.config_obj.Datastore.Location = self.tmpdir
	self.config_obj.Datastore.FilestoreDirectory = self.tmpdir
	self.config_obj.Frontend.DoNotCompressArtifacts = true

	serialized, err := yaml.Marshal(self.config_obj)
	assert.NoError(t, err)

	fd.Write(serialized)
	fd.Close()

	// Record the OS Type we are running on.
	self.OS_TYPE = "Linux"
	if runtime.GOOS == "windows" {
		self.OS_TYPE = "Windows"
	} else if runtime.GOOS == "darwin" {
		self.OS_TYPE = "Darwin"
	}
}

func (self *CollectorTestSuite) addArtifactDefinitions() {
	t := self.T()

	// Create new artifacts and just save them on the filesystem - we
	// dont need a real repository manager.
	file_store_factory := file_store.GetFileStore(self.config_obj)

	fd, err := file_store_factory.WriteFile(paths.GetArtifactDefintionPath(
		"Custom.TestArtifactDependent"))
	assert.NoError(t, err)

	fd.Truncate()
	fd.Write([]byte(`name: Custom.TestArtifactDependent
tools:
  - name: MyTool
    version: 1
  - name: MyDataFile

sources:
 - query: |
     LET binary <= SELECT OSPath, Name
         FROM Artifact.Generic.Utils.FetchBinary(
              ToolName="MyTool", SleepDuration='0')

     LET data_file <= SELECT OSPath, Name
         FROM Artifact.Generic.Utils.FetchBinary(
              ToolName="MyDataFile", SleepDuration='0',
              IsExecutable=FALSE)

     LET _ <= sleep(time=1)

     SELECT "Foobar", Stdout, binary[0].Name,
            data_file[0].OSPath AS DataFilePath,
            data_file[0].OSPath =~ ".yar$" AS HasYarExtension,
            read_file(filename=data_file[0].OSPath) AS Data
     FROM execve(argv=[binary[0].OSPath, "artifacts", "list"])
`))
	fd.Close()

	fd, err = file_store_factory.WriteFile(
		paths.GetArtifactDefintionPath("Custom.TestArtifact"))
	assert.NoError(t, err)

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
`))
	fd.Close()

	fd, err = file_store_factory.WriteFile(
		paths.GetArtifactDefintionPath("Custom.TestHello"))
	assert.NoError(t, err)

	fd.Truncate()
	fd.Write([]byte(`name: Custom.TestHello
sources:
 - query: SELECT "Hello" AS Hi FROM scope()
`))
	fd.Close()
}

func (self *CollectorTestSuite) uploadToolDefinitions() {
	t := self.T()

	// Upload the real binary for the architecture we are running on.
	cmd := exec.Command(self.binary, "--config", self.config_file,
		"tools", "upload", "--name", "Velociraptor"+self.OS_TYPE,
		"--tool_version", constants.VERSION,
		self.binary)
	out, err := cmd.CombinedOutput()
	fmt.Println(string(out))
	require.NoError(t, err)

	// Make sure the binary is proprly added.
	assert.Regexp(t, "name: Velociraptor", string(out))

	// Should have a hash
	assert.Regexp(t, "hash: .+", string(out))

	// Add ourselves again as a tool called MyTool - the artifact will
	// call it.
	cmd = exec.Command(self.binary, "--config", self.config_file,
		"tools", "upload", "--name", "MyTool",
		"--tool_version", "1", self.binary)
	out, err = cmd.CombinedOutput()
	fmt.Println(string(out))
	require.NoError(t, err)
}

func (self *CollectorTestSuite) TearDownSuite() {
	os.RemoveAll(self.tmpdir)
}

func (self *CollectorTestSuite) TestCollectorPlain() {
	t := self.T()

	// Change into the tmpdir
	old_dir, _ := os.Getwd()
	defer os.Chdir(old_dir)

	os.Chdir(self.tmpdir)

	// Create an embedded data file
	data_file_name := filepath.Join(self.tmpdir, "test.yar")
	{
		fd, err := os.OpenFile(data_file_name,
			os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0600)
		assert.NoError(t, err)
		fd.Write([]byte("Hello world"))
		fd.Close()
	}

	// Add it as a tool
	cmd := exec.Command(self.binary, "--config", self.config_file,
		"tools", "upload", "--name", "MyDataFile", data_file_name)
	out, err := cmd.CombinedOutput()
	fmt.Println(string(out))
	require.NoError(t, err)

	// Where we will write the collection.
	output_zip := filepath.Join(self.tmpdir, "output.zip")

	// Now we want to create a stand alone collector. We do this
	// by collecting the Server.Utils.CreateCollector artifact
	cmdline := []string{"--config", self.config_file, "-v",
		"artifacts", "collect", "Server.Utils.CreateCollector",
		"--args", "OS=" + self.OS_TYPE,
		"--args", "artifacts=[\"Custom.TestArtifact\"]",
		"--args", "parameters={\"Custom.TestArtifact\":{\"MyParameter\": \"MyValue\"}}",
		"--args", "target=ZIP",
		"--args", "opt_admin=N",
		"--args", "opt_prompt=N",
		"--output", output_zip,
	}

	cmd = exec.Command(self.binary, cmdline...)
	out, err = cmd.CombinedOutput()
	fmt.Println(string(out))
	require.NoError(t, err)

	// Inspect the resulting binary - it should have a zip appended.
	r, err := zip.OpenReader(output_zip)
	assert.NoError(t, err)

	defer r.Close()

	// Iterate through the files in the archive,
	// printing some of their contents.
	output_executable := filepath.Join(self.tmpdir, "collector"+self.extension)
	for _, f := range r.File {
		fmt.Printf("Contents of collector:  %s (%v bytes)\n",
			f.Name, f.UncompressedSize)
		if strings.HasPrefix(f.Name, "uploads/scope/Collector") {
			fmt.Printf("Extracting %v to %v\n", f.Name, output_executable)

			rc, err := f.Open()
			assert.NoError(t, err)

			out_fd, err := os.OpenFile(
				output_executable, os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0700)
			assert.NoError(t, err)

			n, err := io.Copy(out_fd, rc)
			assert.NoError(t, err)
			rc.Close()
			out_fd.Close()

			fmt.Printf("Copied %v bytes\n", n)
		}
	}

	// Now just run the executable to check the config.
	fmt.Printf("Config show\n")
	cmd = exec.Command(output_executable, "config", "show")
	out, err = cmd.CombinedOutput()
	fmt.Println(string(out))
	require.NoError(t, err)

	// Run the executable and see what it collects.
	cmd = exec.Command(output_executable)
	out, err = cmd.CombinedOutput()
	fmt.Println(string(out))
	require.NoError(t, err)

	// There should be a collection now.
	zip_files, err := filepath.Glob("Collection-*.zip")
	assert.NoError(t, err)
	assert.Equal(t, 1, len(zip_files))

	// Clean it up after we are done.
	defer func() {
		err := os.Remove(zip_files[0])
		assert.NoError(t, err)
	}()

	// Inspect the collection zip file - there should be a single
	// artifact output from our custom artifact, and the data it
	// produces should have the string Foobar in it.
	r, err = zip.OpenReader(zip_files[0])
	assert.NoError(t, err)

	defer r.Close()
	assert.True(t, len(r.File) > 0)

	checked := false

	for _, f := range r.File {
		if f.Name != "results/Custom.TestArtifact.json" {
			continue
		}

		checked = true

		fmt.Printf("Contents of %s:\n", f.Name)
		assert.Equal(t, f.Name, "results/Custom.TestArtifact.json")

		rc, err := f.Open()
		assert.NoError(t, err)

		data, err := ioutil.ReadAll(rc)
		assert.NoError(t, err)

		// Make sure the data from the artifact contains the following
		// strings:
		// Foobar column:
		assert.Contains(t, string(data), "Foobar")

		// Content of packed data file
		assert.Contains(t, string(data), `"Data":"Hello world"`)

		// Make sure the data file has the .yar extension
		assert.Contains(t, string(data), `"HasYarExtension":true`)
	}

	assert.True(t, checked)
}

// Check that we can properly generated encrypted containers.
func (self *CollectorTestSuite) TestCollectorEncrypted() {
	t := self.T()

	// Change into the tmpdir
	old_dir, _ := os.Getwd()
	defer os.Chdir(old_dir)

	os.Chdir(self.tmpdir)

	output_zip := filepath.Join(self.tmpdir, "output_enc.zip")

	// Now we want to create a stand alone collector. We do this
	// by collecting the Server.Utils.CreateCollector artifact
	cmdline := []string{"--config", self.config_file, "-v",
		"artifacts", "collect", "Server.Utils.CreateCollector",
		"--args", "OS=" + self.OS_TYPE,
		"--args", "artifacts=[\"Custom.TestHello\"]",
		"--args", "parameters={\"Custom.TestHello\":{\"MyParameter\": \"MyValue\"}}",
		"--args", "target=ZIP",
		"--args", "opt_admin=N",
		"--args", "opt_prompt=N",
		"--args", "encryption_scheme=X509",
		"--args", `encryption_args={"public_key":"","password":""}`,
		"--output", output_zip,
	}

	cmd := exec.Command(self.binary, cmdline...)
	out, err := cmd.CombinedOutput()
	fmt.Println(string(out))
	require.NoError(t, err)

	r, err := zip.OpenReader(output_zip)
	assert.NoError(t, err)

	defer r.Close()

	output_executable := filepath.Join(self.tmpdir, "collector"+self.extension)
	for _, f := range r.File {
		fmt.Printf("Contents of collector:  %s (%v bytes)\n",
			f.Name, f.UncompressedSize)
		if strings.HasPrefix(f.Name, "uploads/scope/Collector") {
			fmt.Printf("Extracting %v to %v\n", f.Name, output_executable)

			rc, err := f.Open()
			assert.NoError(t, err)

			out_fd, err := os.OpenFile(
				output_executable, os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0700)
			assert.NoError(t, err)

			n, err := io.Copy(out_fd, rc)
			assert.NoError(t, err)
			rc.Close()
			out_fd.Close()

			fmt.Printf("Copied %v bytes\n", n)
		}
	}

	// Now just run the executable.
	cmd = exec.Command(output_executable)
	out, err = cmd.CombinedOutput()
	fmt.Println(string(out))
	require.NoError(t, err)

	// There should be a collection now.
	zip_files, err := filepath.Glob("Collection-*.zip")
	assert.NoError(t, err)
	assert.Equal(t, 1, len(zip_files))

	// Clean up after we are done.
	defer func() {
		err := os.Remove(zip_files[0])
		assert.NoError(t, err)
	}()

	// Inspect the collection zip file - The zip file is encrypted
	// therefore contains only a single metadata file (with the
	// encrypted session key) and an opaque data.zip member which is
	// password encrypted.
	r, err = zip.OpenReader(zip_files[0])
	assert.NoError(t, err)

	defer r.Close()

	assert.True(t, len(r.File) > 0)

	names := []string{}
	for _, f := range r.File {
		fmt.Printf("Contents of %s:\n", f.Name)
		names = append(names, f.Name)

		switch f.Name {
		case "metadata.json":
			rc, err := f.Open()
			assert.NoError(t, err)

			data, err := ioutil.ReadAll(rc)
			assert.NoError(t, err)

			// The metadata should contain information required to unpack
			// the zip.
			assert.Contains(t, string(data), "EncryptedPass")

			// Encryption scheme.
			assert.Contains(t, string(data), `"Scheme": "X509"`)
		}
	}

	assert.Equal(t, []string{"metadata.json", "data.zip"}, names)
}

func TestCollector(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping test in short mode.")
	}

	suite.Run(t, &CollectorTestSuite{})
}
