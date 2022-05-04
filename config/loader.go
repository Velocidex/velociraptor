package config

import (
	"bytes"
	"compress/zlib"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"os/user"
	"runtime"

	"github.com/Velocidex/yaml/v2"
	errors "github.com/pkg/errors"
	"google.golang.org/protobuf/proto"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/logging"
	"www.velocidex.com/golang/velociraptor/utils"
)

// A hard error causes the loader to stop immediately.
type HardError struct {
	Err error
}

func (self HardError) Error() string {
	return self.Err.Error()
}

type loader_func func(self *Loader) (*config_proto.Config, error)
type config_mutator_func func(self *config_proto.Config) error
type validator func(self *Loader, config_obj *config_proto.Config) error

type Loader struct {
	verbose, use_writeback, required_logging bool

	loaders         []loader_func
	config_mutators []config_mutator_func
	validators      []validator

	logger *logging.LogContext
}

func (self *Loader) WithTempdir(tmpdir string) *Loader {
	if tmpdir == "" {
		return self
	}

	self = self.Copy()
	self.validators = append(self.validators,
		func(self *Loader, config_obj *config_proto.Config) error {
			// Expand the tmpdir if needed.
			tmpdir = utils.ExpandEnv(tmpdir)

			// Try to create a file in the directory to make sure
			// we have permissions and the directory exists.
			tmpfile, err := ioutil.TempFile(tmpdir, "tmp")
			if err != nil {
				// No we dont have permission there, fall back
				// to system default.
				self.Log("Can not write in temp directory to <red>%v</red> - falling back to default %v",
					tmpdir, os.Getenv("TMP"))
				return nil
			}
			tmpfile.Close()

			defer os.Remove(tmpfile.Name())

			switch runtime.GOOS {
			case "windows":
				os.Setenv("TMP", tmpdir)
				os.Setenv("TEMP", tmpdir)
			case "linux", "darwin":
				os.Setenv("TMP", tmpdir)
				os.Setenv("TMPDIR", tmpdir)
			}
			self.Log("Setting temp directory to <green>%v", tmpdir)

			return nil
		})
	return self
}

func (self *Loader) WithLogFile(filename string) *Loader {
	if filename == "" {
		return self
	}

	self = self.Copy()
	self.validators = append(self.validators,
		func(self *Loader, config_obj *config_proto.Config) error {
			err := logging.AddLogFile(filename)
			if err != nil {
				return HardError{err}
			}
			return nil
		})
	return self
}

func (self *Loader) WithRequiredFrontend() *Loader {
	self = self.Copy()
	self.validators = append(self.validators,
		func(self *Loader, config_obj *config_proto.Config) error {
			if config_obj.Frontend == nil {
				return errors.New("Frontend config is required")
			}
			return nil
		})
	return self
}

func (self *Loader) WithRequiredClient() *Loader {
	self = self.Copy()
	self.validators = append(self.validators,
		func(self *Loader, config_obj *config_proto.Config) error {
			if config_obj.Client == nil {
				return errors.New("Client config is required")
			}
			return nil
		})
	return self
}

// Check that we are running as the correct user. This is critical
// when using the FileBaseDataStore because any files we accidentally
// create as the wrong user will not be readable by the frontend.
func (self *Loader) WithRequiredUser() *Loader {
	self = self.Copy()
	self.validators = append(self.validators,
		func(self *Loader, config_obj *config_proto.Config) error {
			if config_obj.Datastore == nil ||
				config_obj.Datastore.Implementation != "FileBaseDataStore" {
				return nil
			}

			if config_obj.Frontend == nil ||
				config_obj.Frontend.RunAsUser == "" {
				return nil
			}

			user, err := user.Current()
			if err != nil {
				return err
			}

			if user.Username != config_obj.Frontend.RunAsUser {
				return errors.New(fmt.Sprintf(
					"Velociraptor should be running as the '%s' user but you are '%s'. "+
						"Please change user with sudo first.",
					config_obj.Frontend.RunAsUser, user.Username))
			}
			return nil
		})
	return self
}

func (self *Loader) WithOverride(filename string) *Loader {
	if filename == "" {
		return self
	}

	self = self.Copy()
	self.config_mutators = append(self.config_mutators,
		func(config_obj *config_proto.Config) error {
			self.Log("Loading override config from file %v", filename)
			override, err := read_config_from_file(filename)
			if err != nil {
				return HardError{err}
			}

			// Merge the json blob with the config
			proto.Merge(config_obj, override)
			return nil
		})
	return self
}

func (self *Loader) WithRequiredCA() *Loader {
	self = self.Copy()
	self.validators = append(self.validators,
		func(self *Loader, config_obj *config_proto.Config) error {
			if config_obj.CA == nil || config_obj.CA.PrivateKey == "" {
				return errors.New("Config with valid CA is required")
			}
			return nil
		})
	return self
}

func (self *Loader) WithVerbose(verbose bool) *Loader {
	self = self.Copy()
	self.verbose = verbose
	return self
}

// If this is set we require logging to be properly
// initialized. Without this logging is directed to stderr only.
func (self *Loader) WithRequiredLogging() *Loader {
	self = self.Copy()
	self.required_logging = true
	return self
}

func (self *Loader) WithWriteback() *Loader {
	self = self.Copy()
	self.use_writeback = true
	return self
}

func (self *Loader) WithCustomLoader(loader func(self *Loader) (*config_proto.Config, error)) *Loader {
	self = self.Copy()
	self.loaders = append(self.loaders, loader)
	return self
}

func (self *Loader) WithConfigMutator(
	mutator config_mutator_func) *Loader {
	self = self.Copy()
	self.config_mutators = append(self.config_mutators, mutator)
	return self
}

func (self *Loader) WithCustomValidator(
	validator func(config_obj *config_proto.Config) error) *Loader {
	self = self.Copy()
	self.validators = append(self.validators,
		func(self *Loader, config_obj *config_proto.Config) error {
			return validator(config_obj)
		})
	return self
}

func (self *Loader) WithNullLoader() *Loader {
	self = self.Copy()
	self.loaders = append(self.loaders, func(self *Loader) (*config_proto.Config, error) {
		self.Log("Setting empty config")
		return &config_proto.Config{}, nil
	})
	return self
}

func (self *Loader) WithFileLoader(filename string) *Loader {
	if filename != "" {
		self = self.Copy()
		self.loaders = append(self.loaders, func(self *Loader) (*config_proto.Config, error) {
			self.Log("Loading config from file %v", filename)
			result, err := read_config_from_file(filename)
			if err != nil {
				// If a filename is specified but it
				// does not exist or invalid stop
				// searching immediately.
				return result, HardError{err}
			}
			return result, nil

		})
	}

	return self
}

func (self *Loader) WithEnvLoader(env_var string) *Loader {
	self = self.Copy()
	self.loaders = append(self.loaders, func(self *Loader) (*config_proto.Config, error) {
		env_config := os.Getenv(env_var)
		if env_config != "" {
			self.Log("Loading config from env %v (%v)", env_var, env_config)
			return read_config_from_file(env_config)
		}
		return nil, errors.New(fmt.Sprintf("Env var %v is not set", env_var))
	})

	return self
}

func (self *Loader) WithEnvLiteralLoader(env_var string) *Loader {
	self = self.Copy()
	self.loaders = append(self.loaders, func(self *Loader) (*config_proto.Config, error) {
		env_config := os.Getenv(env_var)
		if env_config != "" {
			self.Log("Loading literal config from env %v", env_var)
			result := &config_proto.Config{}
			err := yaml.UnmarshalStrict([]byte(env_config), result)
			if err != nil {
				return nil, errors.WithStack(err)
			}
			return result, nil
		}
		return nil, errors.New(fmt.Sprintf("Env var %v is not set", env_var))
	})

	return self
}

func (self *Loader) WithEmbedded() *Loader {
	self = self.Copy()
	self.loaders = append(self.loaders, func(self *Loader) (*config_proto.Config, error) {
		result, err := read_embedded_config()
		if err == nil {
			self.Log("Loaded embedded config")
		}
		return result, err
	})
	return self
}

func (self *Loader) WithApiLoader(filename string) *Loader {
	if filename == "" {
		return self
	}

	self = self.Copy()
	self.loaders = append(self.loaders, func(self *Loader) (*config_proto.Config, error) {
		result, err := read_api_config_from_file(filename)
		if err == nil {
			self.Log("Loaded api config from %v", filename)
		}
		return result, err
	})
	return self
}

func (self *Loader) WithEnvApiLoader(env_var string) *Loader {
	self = self.Copy()
	self.loaders = append(self.loaders, func(self *Loader) (*config_proto.Config, error) {
		env_config := os.Getenv(env_var)
		if env_config != "" {
			self.Log("Loading config from env %v (%v)", env_var, env_config)
			return read_api_config_from_file(env_config)
		}
		return nil, errors.New(fmt.Sprintf("Env var %v is not set", env_var))
	})
	return self
}

func (self *Loader) Copy() *Loader {
	return &Loader{
		verbose:         self.verbose,
		logger:          self.logger,
		loaders:         append([]loader_func{}, self.loaders...),
		validators:      append([]validator{}, self.validators...),
		config_mutators: append([]config_mutator_func{}, self.config_mutators...),
	}
}

func (self *Loader) Log(format string, v ...interface{}) {
	if self.logger == nil {
		logging.Prelog(format, v...)
	} else {
		self.logger.Info(format, v...)
	}
}

func (self *Loader) Validate(config_obj *config_proto.Config) error {
	var err error

	logging.Reset()
	logging.SuppressLogging = !self.verbose

	// Apply any configuration mutators
	for _, mutator := range self.config_mutators {
		err = mutator(config_obj)
		if err != nil {
			return err
		}
	}

	// Initialize the logging and dump early messages into the
	// correct log destination.
	if self.required_logging {
		err = logging.InitLogging(config_obj)
		if err != nil {
			return err
		}
	} else {
		// Logging is not required so if it fails we dont
		// care.
		_ = logging.InitLogging(&config_proto.Config{})
	}

	// Set the logger for the rest of the loading process.
	self.logger = logging.GetLogger(config_obj, &logging.ToolComponent)

	for _, validator := range self.validators {
		err = validator(self, config_obj)
		if err != nil {
			self.Log("%v", err)
			return err
		}
	}

	if config_obj.Autoexec != nil {
		err = ValidateAutoexecConfig(config_obj)
		if err != nil {
			return err
		}
	}

	if config_obj.Frontend != nil {
		err = ValidateFrontendConfig(config_obj)
		if err != nil {
			return err
		}
	}

	if config_obj.Datastore != nil {
		err = ValidateDatastoreConfig(config_obj)
		if err != nil {
			return err
		}
	}

	if config_obj.Client != nil {
		if self.use_writeback {
			err := self.loadWriteback(config_obj)
			if err != nil {
				return err
			}
		}
		err := ValidateClientConfig(config_obj)
		if err != nil {
			return err
		}
	}

	return nil
}

func (self *Loader) LoadAndValidate() (*config_proto.Config, error) {
	for _, loader := range self.loaders {
		result, err := loader(self)
		if err == nil {
			return result, self.Validate(result)
		}

		// Stop on hard errors.
		_, ok := err.(HardError)
		if ok {
			return nil, err
		}
		self.Log("%v", err)
	}
	return nil, errors.New("Unable to load config from any source.")
}

func read_embedded_config() (*config_proto.Config, error) {
	idx := bytes.IndexByte(FileConfigDefaultYaml, '\n')
	if FileConfigDefaultYaml[idx+1] == '#' {
		return nil, errors.New(
			"No embedded config - you can pack one with the `config repack` command")
	}

	r, err := zlib.NewReader(bytes.NewReader(FileConfigDefaultYaml[idx+1:]))
	if err != nil {
		return nil, err
	}

	b := &bytes.Buffer{}
	_, err = io.Copy(b, r)
	if err != nil {
		return nil, err
	}
	r.Close()

	result := &config_proto.Config{}
	err = yaml.Unmarshal(b.Bytes(), result)
	if err != nil {
		return nil, err
	}
	return result, nil
}

func read_config_from_file(filename string) (*config_proto.Config, error) {
	result := &config_proto.Config{}

	data, err := ioutil.ReadFile(filename)
	if err != nil {
		return nil, errors.WithStack(err)
	}

	err = yaml.UnmarshalStrict(data, result)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	return result, nil
}

func read_api_config_from_file(filename string) (*config_proto.Config, error) {
	result := &config_proto.Config{ApiConfig: &config_proto.ApiClientConfig{}}

	data, err := ioutil.ReadFile(filename)
	if err != nil {
		return nil, errors.WithStack(err)
	}

	err = yaml.UnmarshalStrict(data, result.ApiConfig)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	return result, nil
}
