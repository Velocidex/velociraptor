package csv

import (
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/vfilter"
)

func SetCSVOptions(
	config_obj *config_proto.Config,
	scope vfilter.Scope, writer *Writer) {
	if config_obj != nil &&
		config_obj.Defaults != nil &&
		config_obj.Defaults.CsvDelimiter != "" {
		writer.Comma = []rune(config_obj.Defaults.CsvDelimiter)[0]
	}
}
