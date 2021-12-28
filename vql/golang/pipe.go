package golang

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"

	"github.com/Velocidex/ordereddict"
	"www.velocidex.com/golang/velociraptor/glob"
	"www.velocidex.com/golang/velociraptor/utils"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/vfilter"
	"www.velocidex.com/golang/vfilter/arg_parser"
	"www.velocidex.com/golang/vfilter/types"
)

// A pipe is a VQL constract that emulates a file from a query.
//
// For example:
// LET MyPipe = Pipe(query={
//    SELECT _value FROM range(start=0, end=10, step=1)
// }, sep="\n")
// LET read_file(filename="MyPipe", accessor="pipe") AS Data FROM scope()
//
// Data = "1\n2\n3\n4\n

type Pipe struct {
	output_chan <-chan vfilter.Row
	ctx         context.Context
	sep         []byte
}

func (self *Pipe) Close() error { return nil }
func (self *Pipe) Seek(offset int64, whence int) (int64, error) {
	return 0, errors.New("Not Seekable")
}

func (self *Pipe) Stat() (os.FileInfo, error) {
	return nil, errors.New("Not implemented")
}

func (self *Pipe) Read(buff []byte) (int, error) {
	select {
	case <-self.ctx.Done():
		return 0, io.EOF

	case row, ok := <-self.output_chan:
		if !ok {
			return 0, io.EOF
		}

		switch t := row.(type) {
		case *ordereddict.Dict:
			keys := t.Keys()
			if len(keys) >= 1 {
				value, _ := t.Get(keys[0])

				switch t := value.(type) {
				case string:
					out := append([]byte(t), self.sep...)
					return utils.MemCpy(buff, out), nil

				case []byte:
					return utils.MemCpy(buff,
						append(t, self.sep...)), nil

				default:
					data := fmt.Sprintf("%v", value)
					out := append([]byte(data), self.sep...)
					return utils.MemCpy(buff, out), nil
				}
			}
		}

		data := fmt.Sprintf("%v", row)
		out := append([]byte(data), self.sep...)
		return utils.MemCpy(buff, out), nil
	}
}

type PipeFunctionArgs struct {
	Name  string            `vfilter:"optional,field=name,doc=Name to call the pipe"`
	Query types.StoredQuery `vfilter:"optional,field=query,doc=Run this query to generator data - the first column will be appended to pipe data."`
	Sep   string            `vfilter:"optional,field=sep,doc=The separator that will be used to split each read (default: no separator will be used)"`
}

type PipeFunction struct{}

func (self *PipeFunction) Call(ctx context.Context,
	scope vfilter.Scope,
	args *ordereddict.Dict) vfilter.Any {
	arg := &PipeFunctionArgs{}
	err := arg_parser.ExtractArgsWithContext(ctx, scope, args, arg)
	if err != nil {
		scope.Log("pipe: %s", err.Error())
		return false
	}

	if arg.Name == "" {
		arg.Name = types.ToString(arg.Query, scope)
	}

	key := "pipe:" + arg.Name
	cached_pipe_any := vql_subsystem.CacheGet(scope, key)
	cached_pipe, ok := cached_pipe_any.(*Pipe)

	defer vql_subsystem.CacheSet(scope, key, cached_pipe)

	if !ok {
		row_chan := arg.Query.Eval(ctx, scope)
		cached_pipe = &Pipe{
			output_chan: row_chan,
			ctx:         ctx,
			sep:         []byte(arg.Sep),
		}
	}

	return cached_pipe
}

func (self PipeFunction) Info(scope vfilter.Scope, type_map *vfilter.TypeMap) *vfilter.FunctionInfo {
	return &vfilter.FunctionInfo{
		Name:    "pipe",
		Doc:     "A pipe allows plugins that use files to read data from a vql query.",
		ArgType: type_map.AddType(scope, &PipeFunctionArgs{}),
	}
}

type PipeFilesystemAccessor struct {
	scope vfilter.Scope
}

func (self PipeFilesystemAccessor) New(scope vfilter.Scope) (glob.FileSystemAccessor, error) {
	return PipeFilesystemAccessor{scope}, nil
}

func (self PipeFilesystemAccessor) Lstat(variable string) (glob.FileInfo, error) {
	return utils.NewDataFileInfo(""), nil
}

func (self PipeFilesystemAccessor) ReadDir(path string) ([]glob.FileInfo, error) {
	return nil, errors.New("Not implemented")
}

// The path is the name of the scope variable that holds the pipe object
func (self PipeFilesystemAccessor) Open(variable string) (glob.ReadSeekCloser, error) {
	variable_data, pres := self.scope.Resolve(variable)
	if !pres {
		return nil, os.ErrNotExist
	}
	variable_data_lazy, ok := variable_data.(types.StoredExpression)
	if ok {
		ctx, cancel := context.WithCancel(context.Background())
		vql_subsystem.GetRootScope(self.scope).AddDestructor(cancel)
		variable_data = variable_data_lazy.Reduce(ctx, self.scope)
	}

	pipe, ok := variable_data.(*Pipe)
	if !ok {
		return nil, os.ErrNotExist
	}

	return pipe, nil
}

func (self PipeFilesystemAccessor) PathSplit(path string) []string {
	re := regexp.MustCompile("/")
	return re.Split(path, -1)
}

func (self PipeFilesystemAccessor) PathJoin(root, stem string) string {
	return filepath.Join(root, stem)
}

func (self PipeFilesystemAccessor) GetRoot(path string) (string, string, error) {
	return "/", path, nil
}

func init() {
	glob.Register("pipe", &PipeFilesystemAccessor{}, `Read from a VQL pipe.

A VQL pipe allows data to be generated from a VQL query, as the pipe is read, the query proceeds to feed more data to it.

Example:

  LET MyPipe = pipe(query={
        SELECT _value FROM range(start=0, end=10, step=1)
  }, sep="\n")

  SELECT read_file(filename="MyPipe", accessor="pipe")
  FROM scope()
`)
	vql_subsystem.RegisterFunction(&PipeFunction{})
}
