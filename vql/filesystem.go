package vql

import (
	"context"
	"www.velocidex.com/golang/velociraptor/glob"
	"www.velocidex.com/golang/vfilter"
)

type GlobPlugin struct{}

func (self GlobPlugin) Call(
	ctx context.Context,
	scope *vfilter.Scope,
	args vfilter.Dict) <-chan vfilter.Row {
	globber := make(glob.Globber)
	output_chan := make(chan vfilter.Row)

	// Extract the glob from the args.
	globs, ok := scope.Associative(args, "globs")
	if ok {
		switch t := globs.(type) {
		case string:
			globber.Add(t, "/")
		case []vfilter.Any:
			for _, item := range t {
				switch item_t := item.(type) {
				case string:
					globber.Add(item_t, "/")
				default:
					vfilter.Debug("Unsupported arg type")
				}
			}
		default:
			vfilter.Debug("Unsupported args")
		}
	} else {
		// If not args specified just glob *
		globber.Add("*", "/")
	}

	go func() {
		defer close(output_chan)
		file_chan := globber.ExpandWithContext(
			ctx, "/", glob.OSFileSystemAccessor{})
		for {
			select {
			case <-ctx.Done():
				return

			case f, ok := <-file_chan:
				if !ok {
					return
				}

				output_chan <- f
			}
		}
	}()

	return output_chan
}

func (self GlobPlugin) Name() string {
	return "glob"
}

func (self GlobPlugin) Info(type_map *vfilter.TypeMap) *vfilter.PluginInfo {
	return &vfilter.PluginInfo{
		Name:    "glob",
		Doc:     "Retrieve files based on a list of glob expressions",
		RowType: type_map.AddType(glob.OSFileInfo{}),
	}
}
