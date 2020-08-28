package config

import (
	"bytes"
	"compress/zlib"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"os/user"
	"path/filepath"

	"github.com/Velocidex/yaml/v2"
	errors "github.com/pkg/errors"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/logging"
)

// A hard error causes the loader to stop immediately.
type HardError struct {
	err error
}

func (self HardError) Error() string {
	return self.err.Error()
}

type loader_func func() (*config_proto.Config, error)
type validator func(config_obj *config_proto.Config) error

type Loader struct {
	verbose, use_writeback, required_logging bool

	write_back_path string

	loaders    []loader_func
	validators []validator

	logger *logging.LogContext
}

func (self *Loader) WithLogFile(filename string) *Loader {
	if filename == "" {
		return self
	}

	self = self.Copy()
	self.validators = append(self.validators, func(config_obj *config_proto.Config) error {
		return logging.AddLogFile(filename)
	})
	return self
}

func (self *Loader) WithRequiredFrontend() *Loader {
	self = self.Copy()
	self.validators = append(self.validators, func(config_obj *config_proto.Config) error {
		if config_obj.Frontend == nil {
			return errors.New("Frontend config is required")
		}
		return nil
	})
	return self
}

func (self *Loader) WithRequiredClient() *Loader {
	self = self.Copy()
	self.validators = append(self.validators, func(config_obj *config_proto.Config) error {
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
	self.validators = append(self.validators, func(config_obj *config_proto.Config) error {
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

func (self *Loader) WithRequiredCA() *Loader {
	self = self.Copy()
	self.validators = append(self.validators, func(config_obj *config_proto.Config) error {
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

func (self *Loader) WithCustomLoader(loader func() (*config_proto.Config, error)) *Loader {
	self = self.Copy()
	self.loaders = append(self.loaders, loader)
	return self
}

func (self *Loader) WithCustomValidator(validator func(config_obj *config_proto.Config) error) *Loader {
	self = self.Copy()
	self.validators = append(self.validators, validator)
	return self
}

func (self *Loader) WithNullLoader() *Loader {
	self = self.Copy()
	self.loaders = append(self.loaders, func() (*config_proto.Config, error) {
		self.Log("Setting empty config")
		return &config_proto.Config{}, nil
	})
	return self
}

func (self *Loader) WithFileLoader(filename string) *Loader {
	if filename != "" {
		self = self.Copy()
		self.loaders = append(self.loaders, func() (*config_proto.Config, error) {
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
	self.loaders = append(self.loaders, func() (*config_proto.Config, error) {
		env_config := os.Getenv(env_var)
		if env_config != "" {
			self.Log("Loading config from env %v (%v)", env_var, env_config)
			return read_config_from_file(env_config)
		}
		return nil, errors.New(fmt.Sprintf("Env var %v is not set", env_var))
	})

	return self
}

func (self *Loader) WithEmbedded() *Loader {
	self = self.Copy()
	self.loaders = append(self.loaders, func() (*config_proto.Config, error) {
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
	self.loaders = append(self.loaders, func() (*config_proto.Config, error) {
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
	self.loaders = append(self.loaders, func() (*config_proto.Config, error) {
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
		write_back_path: self.write_back_path,
		loaders:         append([]loader_func{}, self.loaders...),
		validators:      append([]validator{}, self.validators...),
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

	for _, validator := range self.validators {
		err = validator(config_obj)
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

func (self *Loader) loadWriteback(config_obj *config_proto.Config) error {
	existing_writeback := &config_proto.Writeback{}

	filename, err := WritebackLocation(config_obj)
	if err != nil {
		return err
	}
	if !filepath.IsAbs(filename) && self.write_back_path != "" {
		filename = filepath.Join(self.write_back_path, filename)
	}

	self.Log("Loading writeback from %v", filename)
	data, err := ioutil.ReadFile(filename)

	// Failing to read the file is not an error - the file may not
	// exist yet.
	if err == nil {
		err = yaml.Unmarshal(data, existing_writeback)
		// writeback file is invalid... Log an error and reset
		// it otherwise the client will fail to start and
		// break.
		if err != nil {
			self.Log("Writeback file is corrupt - resetting: %v", err)
		}
	}

	// Merge the writeback with the config.
	config_obj.Writeback = existing_writeback
	return nil
}

func (self *Loader) LoadAndValidate() (*config_proto.Config, error) {
	for _, loader := range self.loaders {
		result, err := loader()
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
