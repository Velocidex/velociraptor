package vql

import (
	"www.velocidex.com/golang/vfilter"
)

// The upload plugin is a passthrough plugin which uploads the files
// to the server.

// Args:
//   - hits: A series of rows to upload. These are typically
//      subselects. The rows will be passed directly to the output of
//      the plugin.

// Example:
//   SELECT * from upload(hits= { SELECT FullPath FROM glob(globs=['/tmp/*.txt']) })

func MakeUploaderPlugin() vfilter.GenericListPlugin {
	plugin := vfilter.GenericListPlugin{
		PluginName: "upload",
		RowType:    nil,
	}

	plugin.Function = func(args *vfilter.Dict) []vfilter.Row {
		var result []vfilter.Row
		// Extract the glob from the args.
		hits, ok := args.Get("hits")
		if ok {
			switch t := hits.(type) {
			case []vfilter.Any:
				for _, item := range t {
					plugin.RowType = item
					result = append(result, item)
				}
			default:
				return result
			}
		}
		return result
	}

	return plugin
}
