package server

import (
	"context"
	"fmt"
	"net/url"
	"path"
	"strings"

	"github.com/Velocidex/ordereddict"
	"www.velocidex.com/golang/velociraptor/services"
	"www.velocidex.com/golang/velociraptor/vql"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/vfilter"
	"www.velocidex.com/golang/vfilter/arg_parser"
)

type LinkToFunctionArgs struct {
	Type     string            `vfilter:"optional,field=type,doc=The type of link. Currently one of collection, hunt, artifact, event, debug"`
	ClientId string            `vfilter:"optional,field=client_id"`
	FlowId   string            `vfilter:"optional,field=flow_id"`
	Upload   *ordereddict.Dict `vfilter:"optional,field=upload,doc=Upload object for the file to upload (upload object is returned by the upload() function)"`
	Tab      string            `vfilter:"optional,field=tab,doc=The tab to focus - can be overview, request, results, logs, notebook"`
	Text     string            `vfilter:"optional,field=text,doc=If specified we emit a markdown style URL with a text"`

	HuntId   string `vfilter:"optional,field=hunt_id,doc=The hunt id to read."`
	Artifact string `vfilter:"optional,field=artifact,doc=The artifact to retrieve"`

	RawLink bool   `vfilter:"optional,field=raw,doc=When specified we emit a raw URL (without autodetected text)"`
	OrgId   string `vfilter:"optional,field=org,doc=If set the link accesses a different org. Otherwise we accesses the current org."`
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
		} else if arg.ClientId != "" {
			arg.Type = "client"
		} else if arg.HuntId != "" {
			arg.Type = "hunt"
		} else if arg.Artifact != "" && arg.ClientId != "" {
			arg.Type = "event"
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

		url.Fragment = fmt.Sprintf("/host/%v", arg.ClientId)
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

		url.Fragment = fmt.Sprintf("/collected/%v/%v/%v",
			arg.ClientId, arg.FlowId, tab)
		return formatURL(arg.Text, url, query, org)

	case "hunt":
		if arg.HuntId == "" {
			scope.Log("link_to: For hunt links hunt_id must be set")
			return vfilter.Null{}
		}

		tab := arg.Tab
		if tab == "" {
			tab = "overview"
		}

		if !arg.RawLink && arg.Text == "" {
			arg.Text = arg.HuntId
		}

		url.Fragment = fmt.Sprintf("/hunts/%v/%v", arg.HuntId, tab)
		return formatURL(arg.Text, url, query, org)

	case "artifact":
		if arg.Artifact == "" {
			scope.Log("link_to: For artifact links artifact parameter must be set")
			return vfilter.Null{}
		}

		if !arg.RawLink && arg.Text == "" {
			arg.Text = arg.Artifact
		}

		url.Fragment = fmt.Sprintf("/artifacts/%v", arg.Artifact)
		return formatURL(arg.Text, url, query, org)

	case "event":
		if arg.Artifact == "" || arg.ClientId == "" {
			scope.Log("link_to: For event links both artifact and client_id parameters must be set")
			return vfilter.Null{}
		}

		if !arg.RawLink && arg.Text == "" {
			arg.Text = arg.Artifact
		}

		url.Fragment = fmt.Sprintf("/events/%v/%v", arg.ClientId, arg.Artifact)
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
	if text == "" {
		return url.String()
	}
	return fmt.Sprintf("[%s](%v)", text, url.String())
}

func (self *LinkToFunction) Info(
	scope vfilter.Scope, type_map *vfilter.TypeMap) *vfilter.FunctionInfo {
	return &vfilter.FunctionInfo{
		Name:     "link_to",
		Doc:      "Create a url linking to a particular part in the Velociraptor GUI.",
		ArgType:  type_map.AddType(scope, &LinkToFunctionArgs{}),
		Metadata: vql.VQLMetadata().Build(),
	}
}

func init() {
	vql_subsystem.RegisterFunction(&LinkToFunction{})
}
