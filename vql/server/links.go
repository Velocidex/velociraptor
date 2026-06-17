package server

import (
	"context"
	"fmt"
	"net/url"
	"path"
	"strings"

	"github.com/Velocidex/ordereddict"
	"www.velocidex.com/golang/velociraptor/json"
	"www.velocidex.com/golang/velociraptor/services"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/vfilter"
	"www.velocidex.com/golang/vfilter/arg_parser"
)

type LinkToFunctionArgs struct {
	Type     string            `vfilter:"optional,field=type,doc=The type of link. Currently one of collection, hunt, artifact, event, debug"`
	ClientId string            `vfilter:"optional,field=client_id"`
	FlowId   string            `vfilter:"optional,field=flow_id,doc=Link to this flow. If this value is 'new', the link will create a new collection with the specified artifact."`
	Upload   *ordereddict.Dict `vfilter:"optional,field=upload,doc=Upload object for the file to upload (upload object is returned by the upload() function)"`
	Tab      string            `vfilter:"optional,field=tab,doc=The tab to focus - can be overview, request, results, logs, notebook"`
	Text     string            `vfilter:"optional,field=text,doc=If specified we emit a markdown style URL with a text"`

	HuntId     string `vfilter:"optional,field=hunt_id,doc=The hunt id to read. You can specify 'new' to create a new hunt based on the template in the 'artifact' parameter. "`
	NotebookId string `vfilter:"optional,field=notebook_id,doc=The notebook id to read. You can specify 'new' to create a new notebook based on the template in the 'artifact' parameter. "`

	Artifact   string            `vfilter:"optional,field=artifact,doc=The artifact to retrieve"`
	Parameters *ordereddict.Dict `vfilter:"optional,field=parameters,doc=artifact parameters to use when creating a new flow/hunt/notebook."`
	RawLink    bool              `vfilter:"optional,field=raw,doc=When specified we emit a raw URL (without autodetected text)"`
	OrgId      string            `vfilter:"optional,field=org,doc=If set the link accesses a different org. Otherwise we accesses the current org."`
}

type LinkToFunction struct{}

func (self *LinkToFunction) Call(ctx context.Context,
	scope vfilter.Scope,
	args *ordereddict.Dict) vfilter.Any {

	arg := &LinkToFunctionArgs{}
	err := arg_parser.ExtractArgsWithContext(ctx, scope, args, arg)
	if err != nil {
		scope.Log("link_to: %s", err.Error())
		return vfilter.Null{}
	}

	if arg.RawLink && arg.Text != "" {
		scope.Log("link_to: 'raw' was specified together with 'title' - ignoring 'title'")
		arg.Text = ""
	}

	config_obj, ok := vql_subsystem.GetServerConfig(scope)
	if !ok || config_obj.GUI == nil {
		scope.Log("link_to: Command can only run on the server")
		return vfilter.Null{}
	}

	frontend_service, err := services.GetFrontendManager(config_obj)
	if err != nil {
		return vfilter.Null{}
	}

	url, err := frontend_service.GetPublicUrl(config_obj)
	if err != nil {
		scope.Log("link_to: %v", err)
		return vfilter.Null{}
	}

	org := arg.OrgId
	if org == "" {
		org = config_obj.OrgId
	}

	query := url.Query()

	// Autodetect the type based on the args
	if arg.Type == "" {
		if arg.ClientId != "" && arg.FlowId != "" {
			arg.Type = "collection"
		} else if arg.Artifact != "" && arg.ClientId != "" {
			arg.Type = "event"
		} else if arg.ClientId != "" {
			arg.Type = "client"
		} else if arg.HuntId != "" {
			arg.Type = "hunt"
		} else if arg.NotebookId != "" {
			arg.Type = "notebook"
		} else if arg.Artifact != "" {
			arg.Type = "artifact"
		} else if arg.Upload != nil {
			arg.Type = "upload"
		} else {
			scope.Log("link_to: Supported link types must be one of client, collection, hunt, artifact, event")
			return vfilter.Null{}
		}
	}

	switch strings.ToLower(arg.Type) {
	case "debug":
		frontend_service, err := services.GetFrontendManager(config_obj)
		if err != nil {
			return vfilter.Null{}
		}

		// The link is an API call to VFSDownloadInfo
		url, err := frontend_service.GetBaseURL(config_obj)
		if err != nil {
			scope.Log("link_to: %v", err)
			return vfilter.Null{}
		}

		url.Path = path.Join(url.Path, "/debug/")
		return formatURL(arg.Text, url, query, org)

	case "upload":
		components, pres := arg.Upload.GetStrings("Components")
		if !pres {
			scope.Log("link_to: upload link does not have a components field")
			return vfilter.Null{}
		}

		vfs_name, pres := arg.Upload.GetString("StoredName")
		if !arg.RawLink && pres && arg.Text == "" {
			arg.Text = vfs_name
		}

		frontend_service, err := services.GetFrontendManager(config_obj)
		if err != nil {
			return vfilter.Null{}
		}

		// The link is an API call to VFSDownloadInfo
		url, err = frontend_service.GetBaseURL(config_obj)
		if err != nil {
			scope.Log("link_to: %v", err)
			return vfilter.Null{}
		}

		url.Path = path.Join(url.Path, "/api/v1/DownloadVFSFile")
		query.Add("vfs_path", vfs_name)
		for _, c := range components {
			query.Add("fs_components", c)
		}
		return formatURL(arg.Text, url, query, org)

	case "client":
		if arg.ClientId == "" {
			scope.Log("link_to: For client link client_id must be set")
			return vfilter.Null{}
		}

		// Default label includes the hostname
		if !arg.RawLink && arg.Text == "" {
			arg.Text = fmt.Sprintf("%v (%v)", arg.ClientId, services.GetHostname(
				ctx, config_obj, arg.ClientId))
		}

		url.RawFragment = makeFragment("host", arg.ClientId)
		return formatURL(arg.Text, url, query, org)

	case "collection":
		if arg.ClientId == "" || arg.FlowId == "" {
			scope.Log("link_to: For collection link both client_id and flow_id must be set")
			return vfilter.Null{}
		}

		tab := arg.Tab
		if tab == "" {
			tab = "overview"
		}

		if !arg.RawLink && arg.Text == "" {
			arg.Text = arg.FlowId
		}

		if arg.FlowId == "new" {
			if arg.Artifact == "" {
				scope.Log("link_to: For new collection links, artifact must be specified.")
				return vfilter.Null{}
			}

			// No parameters specified
			if arg.Parameters != nil {
				serialized, err := json.MarshalString(ordereddict.NewDict().
					Set("specs", ordereddict.NewDict().
						Set(arg.Artifact, arg.Parameters)))
				if err == nil {
					url.RawFragment = makeFragment(
						"collected", arg.ClientId,
						"new", arg.Artifact, serialized)
					return formatURL(arg.Text, url, query, org)
				}
			}

			url.RawFragment = makeFragment("collected", arg.ClientId,
				"new", arg.Artifact)
			return formatURL(arg.Text, url, query, org)
		}

		url.RawFragment = makeFragment("collected",
			arg.ClientId, arg.FlowId, tab)
		return formatURL(arg.Text, url, query, org)

	case "hunt":
		if arg.HuntId == "" {
			scope.Log("link_to: For hunt links hunt_id must be set")
			return vfilter.Null{}
		}

		if arg.HuntId == "new" {
			if arg.Artifact == "" {
				scope.Log("link_to: For new hunt links, artifact must be specified.")
				return vfilter.Null{}
			}

			// No parameters specified
			if arg.Parameters != nil {
				serialized, err := json.MarshalString(ordereddict.NewDict().
					Set("specs", ordereddict.NewDict().
						Set(arg.Artifact, arg.Parameters)))
				if err == nil {
					url.RawFragment = makeFragment(
						"hunts", "new", arg.Artifact, serialized)
					return formatURL(arg.Text, url, query, org)
				}
			}

			url.RawFragment = makeFragment("hunts", "new", arg.Artifact)
			return formatURL(arg.Text, url, query, org)
		}

		tab := arg.Tab
		if tab == "" {
			tab = "overview"
		}

		if !arg.RawLink && arg.Text == "" {
			arg.Text = arg.HuntId
		}

		url.RawFragment = makeFragment("hunts", arg.HuntId, tab)
		return formatURL(arg.Text, url, query, org)

	case "notebook":
		if arg.NotebookId == "" {
			scope.Log("link_to: For notebook links notebook_id must be set")
			return vfilter.Null{}
		}

		if arg.NotebookId == "new" {
			if arg.Artifact == "" {
				scope.Log("link_to: For new notebook links, artifact must be specified.")
				return vfilter.Null{}
			}

			// No parameters specified
			if arg.Parameters != nil {
				name, _ := arg.Parameters.GetString("name")
				description, _ := arg.Parameters.GetString("description")
				arg.Parameters.Delete("name")
				arg.Parameters.Delete("description")

				serialized, err := json.MarshalString(ordereddict.NewDict().
					Set("name", name).
					Set("description", description).
					Set("specs", ordereddict.NewDict().
						Set(arg.Artifact, arg.Parameters)))
				if err == nil {
					url.RawFragment = makeFragment(
						"notebooks", "new", arg.Artifact, serialized)
					return formatURL(arg.Text, url, query, org)
				}
			}

			url.RawFragment = makeFragment("notebooks", "new", arg.Artifact)
			return formatURL(arg.Text, url, query, org)
		}

		if !arg.RawLink && arg.Text == "" {
			arg.Text = arg.NotebookId
		}

		url.RawFragment = makeFragment("notebooks", arg.NotebookId)
		return formatURL(arg.Text, url, query, org)

	case "artifact":
		if arg.Artifact == "" {
			scope.Log("link_to: For artifact links artifact parameter must be set")
			return vfilter.Null{}
		}

		if !arg.RawLink && arg.Text == "" {
			arg.Text = arg.Artifact
		}

		url.RawFragment = makeFragment("artifacts", arg.Artifact)
		return formatURL(arg.Text, url, query, org)

	case "event":
		if arg.Artifact == "" || arg.ClientId == "" {
			scope.Log("link_to: For event links both artifact and client_id parameters must be set")
			return vfilter.Null{}
		}

		if !arg.RawLink && arg.Text == "" {
			arg.Text = arg.Artifact
		}

		url.RawFragment = makeFragment("events", arg.ClientId, arg.Artifact)
		return formatURL(arg.Text, url, query, org)

	default:
		scope.Log("link_to: Supported link types must be one of client, collection, hunt, artifact, event")
		return vfilter.Null{}
	}
}

func formatURL(text string, url *url.URL, query url.Values, org string) string {
	// By convention we need to add the org_id at the end.
	if org == "" {
		org = "root"
	}

	if !query.Has("org_id") {
		query.Set("org_id", org)
	}

	url.RawQuery = query.Encode()
	url_string := url.String()

	// Raw fragment means we take care of the encoding ourselves.
	if url.RawFragment != "" {
		url_string += "#" + url.RawFragment
	}
	if text == "" {
		return url_string
	}
	return fmt.Sprintf("[%s](%v)", text, url_string)
}

func (self *LinkToFunction) Info(
	scope vfilter.Scope, type_map *vfilter.TypeMap) *vfilter.FunctionInfo {
	return &vfilter.FunctionInfo{
		Name:     "link_to",
		Doc:      "Create a url linking to a particular part in the Velociraptor GUI.",
		ArgType:  type_map.AddType(scope, &LinkToFunctionArgs{}),
		Metadata: vql_subsystem.VQLMetadata().Build(),
		Version:  3,
	}
}

// Escape each part of the fragment separately. This allows us to
// escape / separators which occur within the fragment parts safely.
func makeFragment(parts ...string) string {
	var res []string
	for _, p := range parts {
		escaped := url.QueryEscape(p)
		// QueryEscape converts spaces to + but javascript's
		// decodeURIComponent does not convert them back so we always
		// convert spaces to %20
		escaped = strings.ReplaceAll(escaped, "+", "%20")
		res = append(res, escaped)
	}

	return "/" + strings.Join(res, "/")
}

func init() {
	vql_subsystem.RegisterFunction(&LinkToFunction{})
}
