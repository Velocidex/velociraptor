package flows

import (
	"errors"
	"fmt"
	"strings"

	"github.com/golang/protobuf/proto"
	"github.com/golang/protobuf/ptypes"
	actions_proto "www.velocidex.com/golang/velociraptor/actions/proto"
	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	flows_proto "www.velocidex.com/golang/velociraptor/flows/proto"
	utils "www.velocidex.com/golang/velociraptor/utils"
)

type FileFinder struct {
	*VQLCollector
}

func (self *FileFinder) New() Flow {
	return &FileFinder{&VQLCollector{}}
}

func (self *FileFinder) Start(
	config_obj *api_proto.Config,
	flow_obj *AFF4FlowObject,
	args proto.Message) error {
	file_finder_args, ok := args.(*flows_proto.FileFinderArgs)
	if !ok {
		return errors.New("Expected args of type VInterrogateArgs")
	}

	builder := file_finder_builder{args: file_finder_args}
	vql_request, err := builder.Build()
	if err != nil {
		return err
	}

	flow_obj.Log(fmt.Sprintf("Compiled VQL request: %s",
		proto.MarshalTextString(vql_request)))

	err = QueueMessageForClient(
		config_obj,
		flow_obj,
		"VQLClientAction",
		vql_request, processVQLResponses)
	if err != nil {
		return err
	}

	return nil
}

type file_finder_builder struct {
	args        *flows_proto.FileFinderArgs
	columns     []string
	glob_plugin string
	result      actions_proto.VQLCollectorArgs
}

// Returns a list of conditions which are expensive to execute (such as grep).
func (self *file_finder_builder) getMatchConditions() []*flows_proto.FileFinderCondition {
	result := []*flows_proto.FileFinderCondition{}
	for _, item := range self.args.Conditions {
		if item.GetContentsLiteralMatch() != nil {
			result = append(result, item)
		}
	}

	return result
}

// Returns a list of conditions which are fast to execute. In order to
// minimize execution cost we order the fast conditions first, then
// pass the results into a second query which runs the expensive
// conditions on that. NOTE: As the VFilter query optimizer improves
// this will not be needed.
func (self *file_finder_builder) getSimpleConditions() []*flows_proto.FileFinderCondition {
	result := []*flows_proto.FileFinderCondition{}
	for _, item := range self.args.Conditions {
		if item.GetContentsLiteralMatch() == nil {
			result = append(result, item)
		}
	}

	return result
}

func (self *file_finder_builder) Build() (*actions_proto.VQLCollectorArgs, error) {
	self.columns = []string{
		"FullPath",
		"Size",
		"Mode.String As Mode",
	}

	glob_plugin, err := self.compileGlobFunction()
	if err != nil {
		return nil, err
	}

	simple_conditions := self.getSimpleConditions()
	simple_query, err := self.compileSimpleQuery(glob_plugin, simple_conditions)
	if err != nil {
		return nil, err
	}
	self.result.Query = append(self.result.Query, &actions_proto.VQLRequest{
		VQL: "let _simple = " + simple_query,
	})

	grep_conditions := self.getMatchConditions()
	grep_query, err := self.compileGrepQuery("_simple", grep_conditions)
	if err != nil {
		return nil, err
	}

	self.result.Query = append(self.result.Query, &actions_proto.VQLRequest{
		VQL: "let files = " + grep_query,
	})

	// Now handle the uploads
	download_action := self.args.Action.GetDownload()
	if download_action != nil {
		query := fmt.Sprintf(
			"SELECT %s, upload(file=FullPath) as Upload FROM files",
			strings.Join(self.columns, " , "),
		)

		self.result.Query = append(self.result.Query, &actions_proto.VQLRequest{
			VQL:  query,
			Name: "File Finder Response",
		})
		return &self.result, nil
	}

	// Default is stat action.
	self.result.Query = append(self.result.Query, &actions_proto.VQLRequest{
		VQL:  "SELECT * from files",
		Name: "File Finder Response",
	})

	return &self.result, nil
}

func (self *file_finder_builder) compileGlobFunction() (string, error) {
	glob_vars := []string{}

	if len(self.args.Paths) == 0 {
		return "", errors.New("Invalid request: No globs specified.")
	}
	// Push the glob into the query environment to prevent
	// escaping issues.
	for idx, glob := range self.args.Paths {
		glob = utils.Normalize_windows_path(glob)
		glob_var := fmt.Sprintf("Path%02d", idx)
		self.result.Env = append(self.result.Env, &actions_proto.VQLEnv{
			Key:   glob_var,
			Value: glob,
		})
		glob_vars = append(glob_vars, glob_var)
	}

	return fmt.Sprintf(
		"glob(globs=[%s])", strings.Join(glob_vars, " , ")), nil
}

func (self *file_finder_builder) compileSimpleQuery(
	glob_plugin string,
	simple_conditions []*flows_proto.FileFinderCondition) (string, error) {
	var conditions []string

	for _, condition := range simple_conditions {
		condition_clause, err := processCondition(condition)
		if err != nil {
			return "", err
		}
		conditions = append(conditions, condition_clause...)
	}

	// Now construct the SQL statement
	where_clause := ""
	if len(conditions) > 0 {
		where_clause = fmt.Sprintf("WHERE %s", strings.Join(conditions, " AND "))
	}

	fields := []string{}
	fields = append(fields, self.columns...)
	fields = append(fields,
		"timestamp(epoch=Mtime.Sec) as mtime",
		"timestamp(epoch=Atime.Sec) as atime",
		"timestamp(epoch=Ctime.Sec) as ctime",
	)

	self.columns = append(self.columns, "mtime", "atime", "ctime")

	return fmt.Sprintf("SELECT %s from %s %s ",
		strings.Join(fields, " , "),
		glob_plugin, where_clause), nil
}

func (self *file_finder_builder) compileGrepQuery(
	source string,
	conditions []*flows_proto.FileFinderCondition) (string, error) {

	if len(conditions) == 0 {
		return fmt.Sprintf("SELECT * FROM %s", source), nil
	}

	kw_vars := []string{}

	// Push the glob into the query environment to prevent
	// escaping issues.
	for idx, condition := range conditions {
		grep_condition := condition.GetContentsLiteralMatch()
		if grep_condition == nil {
			continue
		}
		kw := grep_condition.Literal
		kw_var := fmt.Sprintf("KW%02d", idx)
		self.result.Env = append(self.result.Env, &actions_proto.VQLEnv{
			Key:   kw_var,
			Value: string(kw),
		})
		kw_vars = append(kw_vars, kw_var)
	}

	fields := []string{}
	fields = append(fields, self.columns...)
	fields = append(fields, fmt.Sprintf(
		"grep(keywords=[%s], path=FullPath) as GrepHit",
		strings.Join(kw_vars, " , ")))

	self.columns = append(self.columns, "GrepHit")

	return fmt.Sprintf("SELECT %s from %s WHERE GrepHit ",
		strings.Join(fields, " , "), source), nil
}

func processCondition(condition *flows_proto.FileFinderCondition) ([]string, error) {
	result := []string{}

	mod_time := condition.GetModificationTime()
	if mod_time != nil {
		if mod_time.MinLastModifiedTime > mod_time.MaxLastModifiedTime &&
			mod_time.MaxLastModifiedTime != 0 {
			return nil, errors.New(
				"Invalid modification time condition: min > max")
		}

		if mod_time.MinLastModifiedTime != 0 {
			result = append(result, fmt.Sprintf(
				" Mtime.Sec > %d ", mod_time.MinLastModifiedTime))
		}

		if mod_time.MaxLastModifiedTime != 0 {
			result = append(result, fmt.Sprintf(
				" Mtime.Sec < %d ", mod_time.MaxLastModifiedTime))
		}
	}

	access_time := condition.GetAccessTime()
	if access_time != nil {
		if access_time.MinLastAccessTime > access_time.MaxLastAccessTime &&
			access_time.MaxLastAccessTime != 0 {
			return nil, errors.New(
				"Invalid access time condition: min > max")
		}

		if access_time.MinLastAccessTime != 0 {
			result = append(result, fmt.Sprintf(
				" Atime.Sec > %d ", access_time.MinLastAccessTime))
		}

		if access_time.MaxLastAccessTime != 0 {
			result = append(result, fmt.Sprintf(
				" Atime.Sec < %d ", access_time.MaxLastAccessTime))
		}
	}

	inode_time := condition.GetInodeChangeTime()
	if inode_time != nil {
		if inode_time.MinLastInodeChangeTime > inode_time.MaxLastInodeChangeTime &&
			inode_time.MaxLastInodeChangeTime != 0 {
			return nil, errors.New(
				"Invalid inode change time condition: min > max")
		}

		if inode_time.MinLastInodeChangeTime != 0 {
			result = append(result, fmt.Sprintf(
				" Ctime.Sec > %d ", inode_time.MinLastInodeChangeTime))
		}

		if inode_time.MaxLastInodeChangeTime != 0 {
			result = append(result, fmt.Sprintf(
				" Ctime.Sec < %d ", inode_time.MaxLastInodeChangeTime))
		}
	}

	size := condition.GetSize()
	if size != nil {
		if size.MinFileSize > size.MaxFileSize && size.MaxFileSize != 0 {
			return nil, errors.New(
				"Invalid size condition: min > max")
		}
		if size.MinFileSize != 0 {
			result = append(result, fmt.Sprintf(
				" Size > %d ", size.MinFileSize))
		}
		if size.MaxFileSize != 0 {
			result = append(result, fmt.Sprintf(
				" Size < %d ", size.MaxFileSize))
		}
	}

	return result, nil
}

func init() {
	impl := FileFinder{}
	default_args, _ := ptypes.MarshalAny(&flows_proto.FileFinderArgs{})
	desc := &flows_proto.FlowDescriptor{
		Name:         "FileFinder",
		FriendlyName: "File Finder",
		Category:     "Collectors",
		Doc:          "Interactively build a VQL query to search for files.",
		ArgsType:     "FileFinderArgs",
		DefaultArgs:  default_args,
	}
	RegisterImplementation(desc, &impl)

}
