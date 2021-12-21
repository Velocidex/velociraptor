// +build !aix

/*
   Velociraptor - Hunting Evil
   Copyright (C) 2019 Velocidex Innovations.

   This program is free software: you can redistribute it and/or modify
   it under the terms of the GNU Affero General Public License as published
   by the Free Software Foundation, either version 3 of the License, or
   (at your option) any later version.

   This program is distributed in the hope that it will be useful,
   but WITHOUT ANY WARRANTY; without even the implied warranty of
   MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
   GNU Affero General Public License for more details.

   You should have received a copy of the GNU Affero General Public License
   along with this program.  If not, see <https://www.gnu.org/licenses/>.
*/
package main

import (
	"bytes"
	"compress/zlib"
	"encoding/binary"
	"fmt"
	"io/ioutil"
	"os"
	"regexp"

	"github.com/Velocidex/yaml/v2"
	errors "github.com/pkg/errors"
	"www.velocidex.com/golang/velociraptor/config"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	logging "www.velocidex.com/golang/velociraptor/logging"
	"www.velocidex.com/golang/velociraptor/services"
)

var (
	repack_command = config_command.Command(
		"repack", "Embed a configuration file inside the binary.")

	repack_command_exe = repack_command.Flag(
		"exe", "Use an alternative exe.").String()

	repack_command_config = repack_command.Arg(
		"config_file", "The filename to write into the binary.").
		Required().String()

	repack_command_append = repack_command.Flag(
		"append", "If provided we append the file to the output binary.").
		File()

	repack_command_output = repack_command.Arg(
		"output", "The filename to write the repacked binary.").
		Required().String()

	embedded_re = regexp.MustCompile(`#{3}<Begin Embedded Config>\r?\n`)
)

// Validate any embedded artifacts to make sure they compile properly.
func validate_config(config_obj *config_proto.Config) error {
	if config_obj.Autoexec != nil {
		manager, err := services.GetRepositoryManager()
		if err != nil {
			return err
		}
		repository := manager.NewRepository()

		for _, definition := range config_obj.Autoexec.ArtifactDefinitions {
			serialized, err := yaml.Marshal(definition)
			if err != nil {
				return err
			}

			_, err = repository.LoadYaml(string(serialized), true /* validate */)
			if err != nil {
				return errors.New(
					fmt.Sprintf("While parsing artifact: %s\n%s",
						err, string(serialized)))
			}
		}
	}

	return nil
}

func doRepack() error {
	config_obj, err := new(config.Loader).
		WithFileLoader(*repack_command_config).
		LoadAndValidate()
	if err != nil {
		return fmt.Errorf("Unable to load config file: %w", err)
	}

	sm, err := startEssentialServices(config_obj)
	if err != nil {
		return fmt.Errorf("Starting services: %w", err)
	}
	defer sm.Close()

	err = validate_config(config_obj)
	if err != nil {
		return fmt.Errorf("Validating config: %w", err)
	}

	logger := logging.GetLogger(config_obj, &logging.ToolComponent)

	config_fd, err := os.Open(*repack_command_config)
	if err != nil {
		return fmt.Errorf("Unable to open config file: %w", err)
	}

	config_data, err := ioutil.ReadAll(config_fd)
	if err != nil {
		return fmt.Errorf("Unable to open config file: %w", err)
	}

	// Compress the string.
	var b bytes.Buffer
	w := zlib.NewWriter(&b)
	_, err = w.Write(config_data)
	if err != nil {
		return fmt.Errorf("Unable to write: %w", err)
	}
	w.Close()

	if b.Len() > len(config.FileConfigDefaultYaml)-40 {
		return fmt.Errorf("config file is too large to embed.")
	}

	config_data = b.Bytes()

	// Now pad to the end of the config.
	for i := 0; i < len(config.FileConfigDefaultYaml)-40-len(config_data); i++ {
		config_data = append(config_data, '#')
	}

	input := *repack_command_exe
	if input == "" {
		input, err = os.Executable()
		if err != nil {
			return fmt.Errorf("Unable to open executable: %w", err)
		}
	}

	fd, err := os.Open(input)
	if err != nil {
		return fmt.Errorf("Unable to open executable: %w", err)
	}
	defer fd.Close()

	outfd, err := os.OpenFile(*repack_command_output,
		os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0755)
	if err != nil {
		return fmt.Errorf("Unable to create output file: %w", err)
	}

	data, err := ioutil.ReadAll(fd)
	if err != nil {
		return fmt.Errorf("Unable to read executable: %w", err)
	}
	logger.Info("Read complete binary at %v bytes\n", len(data))

	if *repack_command_append != nil {
		// A PE file - adjust the size of the .rsrc section to
		// cover the entire binary.
		if string(data[0:2]) == "MZ" {
			stat, err := (*repack_command_append).Stat()
			if err != nil {
				return fmt.Errorf("Unable to read appended file: %w", err)
			}

			end_of_file := int64(len(data)) + stat.Size()

			// This is the IMAGE_SECTION_HEADER.Name which
			// is also the start of IMAGE_SECTION_HEADER.
			offset_to_rsrc := bytes.Index(data, []byte(".rsrc"))

			// Found it.
			if offset_to_rsrc > 0 {
				// IMAGE_SECTION_HEADER.PointerToRawData is a 32 bit int.
				start_of_rsrc_section := binary.LittleEndian.Uint32(
					data[offset_to_rsrc+20:])
				size_of_raw_data := uint32(end_of_file) - start_of_rsrc_section
				binary.LittleEndian.PutUint32(
					data[offset_to_rsrc+16:], size_of_raw_data)
			}
		}

		appended, err := ioutil.ReadAll(*repack_command_append)
		if err != nil {
			return fmt.Errorf("Unable to read appended file: %w", err)
		}

		data = append(data, appended...)
	}

	match := embedded_re.FindIndex(data)
	if match == nil {
		return fmt.Errorf("I can not seem to locate the embedded config????")
	}

	end := match[1]

	logger.Info("Write %v\n", len(data[:end]))
	_, err = outfd.Write(data[:end])
	if err != nil {
		return err
	}

	logger.Info("Write %v\n", len(config_data))
	_, err = outfd.Write(config_data)
	if err != nil {
		return err
	}

	logger.Info("Write %v\n", len(data[end+len(config_data):]))
	_, err = outfd.Write(data[end+len(config_data):])
	if err != nil {
		return err
	}

	err = outfd.Close()
	if err != nil {
		return err
	}

	return os.Chmod(outfd.Name(), 0777)
}

func init() {
	command_handlers = append(command_handlers, func(command string) bool {
		if command == repack_command.FullCommand() {
			FatalIfError(repack_command, doRepack)
			return true
		}

		return false
	})
}
