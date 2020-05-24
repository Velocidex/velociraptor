package config

import (
	"bytes"
	"compress/zlib"
	"fmt"
	"io"
	"io/ioutil"
	"os"

	"github.com/Velocidex/yaml/v2"
	errors "github.com/pkg/errors"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
)

type loader_func func() (*config_proto.Config, error)
type validator func(config_obj *config_proto.Config) error

type Loader struct {
	verbose, use_writeback bool

	loaders    []loader_func
	validators []validator
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

func (self *Loader) WithDefaultLoader() *Loader {
	self = self.Copy()
	self.loaders = append(self.loaders, func() (*config_proto.Config, error) {
		return GetDefaultConfig(), nil
	})
	return self
}

func (self *Loader) WithFileLoader(filename string) *Loader {
	if filename != "" {
		self = self.Copy()
		self.loaders = append(self.loaders, func() (*config_proto.Config, error) {
			self.Log(fmt.Sprintf("Trying to load from file %v", filename))
			return read_config_from_file(filename)
		})
	}

	return self
}

func (self *Loader) WithEnvLoader(env_var string) *Loader {
	self = self.Copy()
	self.loaders = append(self.loaders, func() (*config_proto.Config, error) {
		env_config := os.Getenv(env_var)
		if env_config != "" {
			self.Log(fmt.Sprintf("Trying to load from env %v (%v)",
				env_var, env_config))
			return read_config_from_file(env_config)
		}
		return nil, errors.New(fmt.Sprintf("Env var %v is not set", env_var))
	})

	return self
}

func (self *Loader) WithEmbedded() *Loader {
	self = self.Copy()
	self.loaders = append(self.loaders, read_embedded_config)
	return self
}

func (self *Loader) WithApiLoader(filename string) *Loader {
	return self
}

func (self *Loader) WithEnvApiLoader(env_var string) *Loader {
	return self
}

func (self *Loader) Copy() *Loader {
	return &Loader{
		verbose:    self.verbose,
		loaders:    append([]loader_func{}, self.loaders...),
		validators: append([]validator{}, self.validators...),
	}
}

func (self *Loader) Log(message string) {
	fmt.Println(message)
}

func (self *Loader) Validate(config_obj *config_proto.Config) error {

	for _, validator := range self.validators {
		err := validator(config_obj)
		if err != nil {
			return err
		}
	}

	if config_obj.Autoexec != nil {
		err := ValidateAutoexecConfig(config_obj)
		if err != nil {
			return err
		}
	}

	if config_obj.Frontend != nil {
		err := ValidateFrontendConfig(config_obj)
		if err != nil {
			return err
		}
	}

	if config_obj.Client != nil {
		self.loadWriteback(config_obj)
		err := ValidateClientConfig(config_obj)
		if err != nil {
			return err
		}
	}

	return nil
}

func (self *Loader) loadWriteback(config_obj *config_proto.Config) {
	existing_writeback := &config_proto.Writeback{}
	data, err := ioutil.ReadFile(WritebackLocation(config_obj))

	// Failing to read the file is not an error - the file may not
	// exist yet.
	if err == nil {
		err = yaml.Unmarshal(data, existing_writeback)
		// writeback file is invalid... Log an error and reset
		// it otherwise the client will fail to start and
		// break.
		if err != nil {
			self.Log(fmt.Sprintf(
				"Writeback file is corrupt - resetting: %v", err))
		}
	}

	// Merge the writeback with the config.
	config_obj.Writeback = existing_writeback
}

func (self *Loader) LoadAndValidate() (*config_proto.Config, error) {
	for _, loader := range self.loaders {
		result, err := loader()
		if err == nil {
			return result, self.Validate(result)
		}
		self.Log(fmt.Sprintf("%v", err))
	}
	return nil, errors.New("Unable to load config from any source.")
}

func read_embedded_config() (*config_proto.Config, error) {
	idx := bytes.IndexByte(FileConfigDefaultYaml, '\n')
	if FileConfigDefaultYaml[idx+1] == '#' {
		return nil, errors.New(
			"No embedded config - try to pack one with the pack command or " +
				"provide the --config flag.")
	}

	r, err := zlib.NewReader(bytes.NewReader(FileConfigDefaultYaml[idx+1:]))
	if err != nil {
		return nil, err
	}

	b := &bytes.Buffer{}
	io.Copy(b, r)
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
