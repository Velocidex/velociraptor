package main

import (
	"fmt"
	"log"
	"os"
	"path/filepath"

	"github.com/Velocidex/ordereddict"
	errors "github.com/go-errors/errors"
	"www.velocidex.com/golang/velociraptor/config"
	logging "www.velocidex.com/golang/velociraptor/logging"
	"www.velocidex.com/golang/velociraptor/services"
	"www.velocidex.com/golang/velociraptor/startup"
	"www.velocidex.com/golang/velociraptor/uploads"
	"www.velocidex.com/golang/velociraptor/vql/acl_managers"
)

var (
	// Command line interface for VQL commands.
	collector                   = app.Command("collector", "Build an offline collector")
	spec_file                   = collector.Arg("spec_file", "A Spec file to use.").String()
	collector_command_datastore = collector.Flag(
		"datastore", "Path to a datastore directory (defaults to temp)").
		ExistingDir()
)

const SampleSpec = `
# Can be Windows, Windows_x86, Linux, MacOS, MacOSArm, Generic
OS: Windows

# The list of artifacts and their args.
Artifacts:
 Windows.KapeFiles.Targets:
    EventLogs: Y
 Windows.Sysinternals.Autoruns:
    All: Y

# Can be ZIP, GCS, S3, Azure, SMBShare, SFTP
Target: ZIP

# Relevant args to the Target type above.
TargetArgs:
  bucket:
  GCSKey:

# Can be None, X509
# NOTE: You can unzip the encrypted zip using
# velociraptor --config server.config.yaml unzip --dump_dir output file.zip
EncryptionScheme: None

# Following can be Y or N
OptVerbose: Y
OptBanner: Y
OptPrompt: N
OptAdmin: Y

# A path to use for the temp file (Blank for system default)
OptTempdir:

# Compression level to use
OptLevel: 5
OptFilenameTemplate: "Collection-%FQDN%-%TIMESTAMP%"

# Can be jsonl or csv
OptFormat: jsonl
`

func doCollector() error {
	if *spec_file == "" {
		fmt.Printf(`
# This command builds an offline collector from the CLI

# You must provide a spec file. For example
# %s collector /path/to/spec_file.yaml

# An example spec file follows. You can redirect to a file and edit as needed.

`, os.Args[0])
		fmt.Printf("%s", SampleSpec)
		return errors.New("No Spec file provided")
	}

	// Start from a clean slate
	os.Setenv("VELOCIRAPTOR_CONFIG", "")

	datastore_directory := *collector_command_datastore
	if datastore_directory == "" {
		datastore_directory = filepath.Join(os.TempDir(), "gui_datastore")

		// Ensure the directory exists
		err := os.MkdirAll(datastore_directory, 0o777)
		if err != nil {
			return fmt.Errorf("Unable to create datastore directory: %w", err)
		}
	}

	datastore_directory, err := filepath.Abs(datastore_directory)
	if err != nil {
		return fmt.Errorf("Unable find path: %w", err)
	}

	server_config_path := filepath.Join(datastore_directory, "server.config.yaml")
	client_config_path := filepath.Join(datastore_directory, "client.config.yaml")

	// Try to open the config file from there
	config_obj, err := makeDefaultConfigLoader().
		WithVerbose(true).
		WithFileLoader(server_config_path).LoadAndValidate()
	if err != nil || config_obj.Frontend == nil {
		// Stop on hard errors but if the file does not exist we need
		// to create it below..
		hard_err, ok := err.(config.HardError)
		if ok && !errors.Is(hard_err.Err, os.ErrNotExist) {
			return err
		}

		// Need to generate a new config. This config is the same as
		// the `gui` command makes. You can keep this datastore around
		// for the next collector.
		logging.Prelog("No valid config found - " +
			"will generare a new one at <green>" + server_config_path)

		config_obj, err = generateGUIConfig(
			datastore_directory, server_config_path, client_config_path)
		if err != nil {
			return err
		}
	}

	if config_obj.Services == nil {
		config_obj.Services = services.GenericToolServices()
	}

	// Now start the frontend
	ctx, cancel := install_sig_handler()
	defer cancel()

	sm, err := startup.StartToolServices(ctx, config_obj)
	if err != nil {
		return fmt.Errorf("starting tool services: %w", err)
	}
	defer sm.Close()

	spec_filename, err := filepath.Abs(*spec_file)
	if err != nil {
		return err
	}

	_, err = os.Lstat(spec_filename)
	if err != nil {
		return err
	}

	logger := &StdoutLogWriter{}
	builder := services.ScopeBuilder{
		Config:     sm.Config,
		ACLManager: acl_managers.NewRoleACLManager(sm.Config, "administrator"),
		Logger:     log.New(logger, "", 0),
		Env: ordereddict.NewDict().
			Set("SPECFILE", spec_filename),
		Uploader: &uploads.FileBasedUploader{
			UploadDir: datastore_directory,
		},
	}

	// this is needed to ensure artifacts are fully loaded before we
	// start so their tools are fully registred.
	query := `
LET _ <= SELECT name FROM artifact_definitions()
LET Spec <= parse_yaml(filename=SPECFILE)
LET _K = SELECT _key FROM items(item=Spec.Artifacts)
SELECT * FROM Artifact.Server.Utils.CreateCollector(
   OS=Spec.OS,
   artifacts=serialize(item=_K._key),
   parameters=serialize(item=Spec.Artifacts),
   target=Spec.Target,
   target_args=Spec.TargetArgs,
   encryption_scheme=Spec.EncryptionScheme,
   opt_verbose=Spec.OptVerbose,
   opt_banner=Spec.OptBanner,
   opt_prompt=Spec.OptPrompt,
   opt_admin=Spec.OptAdmin,
   opt_tempdir=Spec.OptTempdir,
   opt_level=Spec.OptLevel,
   opt_filename_template=Spec.OptFilenameTemplate,
   opt_format=Spec.OptFormat
   )
`
	return runQueryWithEnv(query, builder, "json")
}

func init() {
	command_handlers = append(command_handlers, func(command string) bool {
		switch command {
		case collector.FullCommand():
			FatalIfError(collector, doCollector)

		default:
			return false
		}
		return true
	})
}
