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

	"github.com/Velocidex/yaml"
	errors "github.com/pkg/errors"
	kingpin "gopkg.in/alecthomas/kingpin.v2"
	artifacts "www.velocidex.com/golang/velociraptor/artifacts"
	"www.velocidex.com/golang/velociraptor/config"
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

	embedded_re = regexp.MustCompile(`#{3}<Begin Embedded Config>\n`)
)

func validate_config(config_data []byte) error {
	// Validate the string by parsing it as a config proto.
	test_config := config.GetDefaultConfig()
	err := yaml.UnmarshalStrict(config_data, test_config)
	if err != nil {
		return err
	}

	if test_config.Autoexec != nil {
		repository := artifacts.NewRepository()

		for _, definition := range test_config.Autoexec.ArtifactDefinitions {
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

func doRepack() {
	config_fd, err := os.Open(*repack_command_config)
	kingpin.FatalIfError(err, "Unable to open config file")

	config_data, err := ioutil.ReadAll(config_fd)
	kingpin.FatalIfError(err, "Unable to read config file")

	// Validate the string by parsing it as a config proto.
	err = validate_config(config_data)
	kingpin.FatalIfError(err, "Config file invalid")

	// Compress the string.
	var b bytes.Buffer
	w := zlib.NewWriter(&b)
	w.Write(config_data)
	w.Close()

	if b.Len() > len(config.FileConfigDefaultYaml)-40 {
		kingpin.FatalIfError(err, "config file is too large to embed.")
	}

	config_data = b.Bytes()

	// Now pad to the end of the config.
	for i := 0; i < len(config.FileConfigDefaultYaml)-40-len(config_data); i++ {
		config_data = append(config_data, '#')
	}

	input := *repack_command_exe
	if input == "" {
		input, err = os.Executable()
		kingpin.FatalIfError(err, "Unable to open executable")
	}

	fd, err := os.Open(input)
	kingpin.FatalIfError(err, "Unable to open executable")
	defer fd.Close()

	outfd, err := os.OpenFile(*repack_command_output,
		os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0755)
	kingpin.FatalIfError(err, "Unable to create output file")
	defer outfd.Close()

	data, err := ioutil.ReadAll(fd)
	kingpin.FatalIfError(err, "Unable to read executable")

	if *repack_command_append != nil {
		// A PE file - adjust the size of the .rsrc section to
		// cover the entire binary.
		if string(data[0:2]) == "MZ" {
			stat, err := (*repack_command_append).Stat()
			kingpin.FatalIfError(err, "Unable to read appended file")

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
		kingpin.FatalIfError(err, "Unable to read appended file")

		data = append(data, appended...)
	}

	match := embedded_re.FindIndex(data)
	if match == nil {
		kingpin.Fatalf("I can not seem to locate the embedding config????")
	}

	end := match[1]

	_, err = outfd.Write(data[:end])
	kingpin.FatalIfError(err, "Writing")

	_, err = outfd.Write(config_data)
	kingpin.FatalIfError(err, "Writing")

	_, err = outfd.Write(data[end+len(config_data):])
	kingpin.FatalIfError(err, "Writing")
}

func init() {
	command_handlers = append(command_handlers, func(command string) bool {
		if command == "config repack" {
			doRepack()
			return true
		}

		return false
	})
}
