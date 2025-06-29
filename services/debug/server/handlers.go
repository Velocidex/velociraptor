package server

import (
	"encoding/json"
	"net/http"
	"net/http/pprof"
	"net/url"
	"path"
	"regexp"
	"strings"

	"github.com/Velocidex/ordereddict"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"www.velocidex.com/golang/velociraptor/api/authenticators"
	api_utils "www.velocidex.com/golang/velociraptor/api/utils"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/services"
	"www.velocidex.com/golang/velociraptor/utils"
	"www.velocidex.com/golang/velociraptor/vql/acl_managers"
	"www.velocidex.com/golang/velociraptor/vql/golang"
	"www.velocidex.com/golang/vfilter"
)

func (self *debugMux) HandleProfile(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")

	profile_type, _ := url.QueryUnescape(path.Base(r.URL.Path))

	format := "jsonl"
	if profile_type == "json" {
		format = "json"
		profile_type, _ = url.QueryUnescape(path.Base(path.Dir(r.URL.Path)))
	}

	if profile_type == "profile" || profile_type == "all" {
		profile_type = ".+"
	} else {
		// Take the profile type as a literal string not a regex
		profile_type = regexp.QuoteMeta(profile_type)
	}

	builder := services.ScopeBuilder{
		Config:     self.config_obj,
		ACLManager: acl_managers.NullACLManager{},
		Env:        ordereddict.NewDict(),
	}

	manager, err := services.GetRepositoryManager(self.config_obj)
	if err != nil {
		return
	}
	scope := manager.BuildScope(builder)
	defer scope.Close()

	plugin := &golang.ProfilePlugin{}
	rows := []vfilter.Row{}
	for row := range plugin.Call(
		r.Context(), scope, ordereddict.NewDict().
			Set("type", profile_type)) {
		if format == "jsonl" {
			serialized, _ := json.Marshal(row)
			serialized = append(serialized, '\n')
			_, _ = w.Write(serialized)

		} else {
			rows = append(rows, row)
		}
	}

	if format == "json" {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		serialized, _ := json.Marshal(rows)
		_, _ = w.Write(serialized)
	}
}

type debugMux struct {
	config_obj       *config_proto.Config
	base             string
	require_root_org bool
}

func (self *debugMux) RequireRootOrg() *debugMux {
	self.require_root_org = true
	return self
}

func (self *debugMux) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if self.require_root_org {
		org_id := authenticators.GetOrgIdFromRequest(r)
		if !utils.IsRootOrg(org_id) {
			// Switch to the root org
			url := r.URL
			url.Path = api_utils.GetBasePath(self.config_obj, url.Path)
			url.RawQuery = "org_id=root"
			http.Redirect(w, r, url.String(), http.StatusTemporaryRedirect)
			return
		}
	}

	url_path := r.URL.Path
	parts := strings.Split(strings.TrimPrefix(url_path, "/"), "/")
	if len(parts) <= 1 {
		self.HandleIndex(w, r)
		return
	}

	// If the URL ends with html we render a html page that allows us
	// to access the raw data.
	if parts[len(parts)-1] == "html" {
		self.renderHTML(w, r)
		return
	}

	switch parts[1] {
	case "profile":
		self.HandleProfile(w, r)

	case "pprof":
		if len(parts) == 2 {
			pprof.Index(w, r)
			return
		}

		switch parts[2] {
		case "profile":
			pprof.Profile(w, r)
		case "symbol":
			pprof.Symbol(w, r)
		case "trace":
			pprof.Trace(w, r)
		default:
			pprof.Index(w, r)
		}

	case "metrics":
		promhttp.Handler().ServeHTTP(w, r)

	default:
		self.HandleIndex(w, r)
	}
}

func DebugMux(config_obj *config_proto.Config, base string) *debugMux {
	return &debugMux{
		config_obj: config_obj,
		base:       base,
	}
}
