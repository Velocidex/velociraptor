package filesystem

import (
	"context"
	"github.com/Velocidex/ahocorasick"
	"io"
	"www.velocidex.com/golang/velociraptor/glob"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/vfilter"
)

type GrepFunctionArgs struct {
	Path     string   `vfilter:"required,field=path"`
	Keywords []string `vfilter:"required,field=keywords"`
	Context  int      `vfilter:"optional,field=context"`
}

type GrepFunction struct{}

// The Grep VQL function searches for a literal or regex match inside the file
func (self *GrepFunction) Call(ctx context.Context,
	scope *vfilter.Scope,
	args *vfilter.Dict) vfilter.Any {
	arg := &GrepFunctionArgs{}
	err := vfilter.ExtractArgs(scope, args, arg)
	if err != nil {
		scope.Log("%s: %s", self.Name(), err.Error())
		return false
	}

	if arg.Context == 0 {
		arg.Context = 10
	}

	var keywords [][]byte
	for _, item := range arg.Keywords {
		// TODO: Add extra encodings like UTF16
		keywords = append(keywords, []byte(item))
	}

	ah_matcher := ahocorasick.NewMatcher(keywords)
	offset := 0

	buf := make([]byte, 4*1024*1024) // 4Mb chunks
	fs := glob.OSFileSystemAccessor{}
	file, err := fs.Open(arg.Path)
	if err != nil {
		scope.Log(err.Error())
		return false
	}
	defer file.Close()

	hits := []*vfilter.Dict{}

	for {
		select {
		case <-ctx.Done():
			return vfilter.Null{}

		default:
			n, err := file.Read(buf)
			if err == io.EOF {
				return hits

			} else if err != nil {
				scope.Log(err.Error())
				return false
			}

			for _, hit := range ah_matcher.Match(buf[:n]) {
				min_bound := offset + hit - arg.Context
				if min_bound < 0 {
					min_bound = 0
				}

				max_bound := offset + hit + arg.Context
				if max_bound > n {
					max_bound = n
				}

				hits = append(hits, vfilter.NewDict().
					Set("type", "GrepHit").
					Set("offset", offset+hit).
					Set("min_bound", min_bound).
					Set("max_bound", max_bound).
					Set("context", string(
						buf[min_bound:max_bound])))
			}

			offset += n
		}
	}
}

func (self GrepFunction) Name() string {
	return "grep"
}

func init() {
	vql_subsystem.RegisterFunction(&GrepFunction{})
}
