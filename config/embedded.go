package config

import (
	"bytes"
	"compress/zlib"
	"io"
	"os"

	"github.com/Velocidex/yaml/v2"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/utils"
)

func ExtractEmbeddedConfig(
	embedded_file string) (*config_proto.Config, error) {

	fd, err := os.Open(embedded_file)
	if err != nil {
		return nil, err
	}

	// Read a lot of the file into memory so we can extract the
	// configuration. This solution only loads the first 10mb into
	// memory which should be sufficient for most practical config
	// files. If there are embedded binaries they will not be read and
	// will be ignored at this stage (thay can be extracted with the
	// 'me' accessor).
	buf, err := utils.ReadAllWithLimit(fd, 10*1024*1024)
	if err != nil {
		return nil, err
	}

	// Find the embedded marker in the buffer.
	match := embedded_re.FindIndex(buf)
	if match == nil {
		return nil, noEmbeddedConfig
	}

	embedded_string := buf[match[0]:]
	return decode_embedded_config(embedded_string)
}

func read_embedded_config() (*config_proto.Config, error) {
	return decode_embedded_config(FileConfigDefaultYaml)
}

func decode_embedded_config(encoded_string []byte) (*config_proto.Config, error) {
	// Get the first line which is never disturbed
	idx := bytes.IndexByte(encoded_string, '\n')

	if len(encoded_string) < idx+10 {
		return nil, noEmbeddedConfig
	}

	// If the following line still starts with # then the file is not
	// repacked - the repacker will replace all further data with the
	// compressed string.
	if encoded_string[idx+1] == '#' {
		return nil, noEmbeddedConfig
	}

	// Decompress the rest of the data - note that zlib will ignore
	// any padding anyway because the zlib header already contains the
	// length of the compressed data so it is safe to just feed it the
	// whole string here.
	r, err := zlib.NewReader(bytes.NewReader(encoded_string[idx+1:]))
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
