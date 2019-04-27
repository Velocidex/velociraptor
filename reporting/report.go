package reporting

import (
	"errors"
	"fmt"
	"strings"

	//	html_template "html/template"

	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	"www.velocidex.com/golang/velociraptor/artifacts"
	artifacts_proto "www.velocidex.com/golang/velociraptor/artifacts/proto"
	"www.velocidex.com/golang/velociraptor/logging"
	"www.velocidex.com/golang/velociraptor/utils"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/vfilter"
)

var (
	valid_report_types = []string{
		"MONITORING_DAILY", "CLIENT",
	}
)

// An expander is presented to the go templates to implement template
// operations.
type TemplateEngine interface {
	Execute(template_string string) (string, error)
	SetEnv(key, value string)
	GetArtifact() *artifacts_proto.Artifact
	Close()
}

// Everything needed to evaluate a template
type BaseTemplateEngine struct {
	Artifact *artifacts_proto.Artifact
	Env      *vfilter.Dict
	Scope    *vfilter.Scope
	logger   *logging.LogContext
}

func (self *BaseTemplateEngine) GetArtifact() *artifacts_proto.Artifact {
	return self.Artifact
}

func (self *BaseTemplateEngine) SetEnv(key, value string) {
	self.Env.Set(key, value)
}

func (self *BaseTemplateEngine) Close() {
	self.Scope.Close()
}

func (self *BaseTemplateEngine) getFunction(a interface{}, b string) interface{} {
	res := a
	var pres bool
	for _, component := range strings.Split(b, ".") {
		res, pres = self.Scope.Associative(res, component)
		if !pres {
			return ""
		}
	}
	return res
}

func (self *BaseTemplateEngine) GetScope(item string) interface{} {
	value, pres := self.Scope.Resolve(item)
	if pres {
		return value
	}

	return "<?>"
}

// GenerateMonitoringDailyReport Generates a report for daily
// monitoring reports.

// Daily monitoring reports are intended to operate on one of more
// daily logs.

// Parameters:
// dayName: The name of the required day as needed by VQL plugins like
//   the monitoring() plugin. This allows the report to query
//   monitoring logs of one or more artifacts.

func GenerateMonitoringDailyReport(template_engine TemplateEngine,
	client_id, day_name string) (string, error) {

	template_engine.SetEnv("dayName", day_name)
	template_engine.SetEnv("ClientId", client_id)

	result := ""
	for _, report := range template_engine.GetArtifact().Reports {
		type_name := strings.ToLower(report.Type)
		if type_name != "monitoring_daily" {
			continue
		}

		value, err := template_engine.Execute(report.Template)
		if err != nil {
			return "", err
		}
		result += value
	}

	return result, nil
}

func GenerateClientReport(template_engine TemplateEngine,
	client_id, flow_id string) (string, error) {

	template_engine.SetEnv("FlowId", flow_id)
	template_engine.SetEnv("ClientId", client_id)
	template_engine.SetEnv("ArtifactName", template_engine.GetArtifact().Name)

	result := ""
	for _, report := range template_engine.GetArtifact().Reports {
		utils.Debug(report)
		type_name := strings.ToLower(report.Type)
		if type_name != "client" {
			continue
		}

		value, err := template_engine.Execute(report.Template)
		if err != nil {
			return "", err
		}
		result += value
	}

	return result, nil
}

func newBaseTemplateEngine(
	config_obj *api_proto.Config,
	artifact_name string,
	parameters map[string]string) (*BaseTemplateEngine, error) {
	repository, err := artifacts.GetGlobalRepository(config_obj)
	if err != nil {
		return nil, err
	}

	artifact, pres := repository.Get(artifact_name)
	if !pres {
		return nil, errors.New(
			fmt.Sprintf("Artifact %v not known.", artifact_name))
	}

	env := vfilter.NewDict().
		Set("config", config_obj.Client).
		Set("server_config", config_obj).
		Set(vql_subsystem.CACHE_VAR, vql_subsystem.NewScopeCache())

	if parameters != nil {
		for k, v := range parameters {
			env.Set(k, v)
		}
	}

	scope := artifacts.MakeScope(repository).AppendVars(env)
	return &BaseTemplateEngine{
		Artifact: artifact,
		Scope:    scope,
		Env:      env,
		logger:   logging.GetLogger(config_obj, &logging.FrontendComponent),
	}, nil
}
