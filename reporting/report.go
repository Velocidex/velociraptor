package reporting

import (
	"errors"
	"fmt"
	"strings"

	"www.velocidex.com/golang/velociraptor/artifacts"
	artifacts_proto "www.velocidex.com/golang/velociraptor/artifacts/proto"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/logging"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/vfilter"
)

var (
	valid_report_types = []string{
		"MONITORING_DAILY", "CLIENT",
		"CLIENT_EVENT", "SERVER_EVENT", "HUNT",
	}
)

// An expander is presented to the go templates to implement template
// operations.
type TemplateEngine interface {
	Execute(template_string string) (string, error)
	SetEnv(key string, value interface{})
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

func (self *BaseTemplateEngine) SetEnv(key string, value interface{}) {
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

// Daily monitoring reports are intended to operate on one or more
// daily logs. The template automatically provides a number of
// parameters through the scope, which may be accessed by the
// template. However, normally the template will simply use the
// source() VQL plugin. This plugin will be able to transparently use
// these parameters so the report template author does not need to
// worry about the parameters too much.

// Parameters:
// StartTime: When the report should start reporting from.
// EndTime: When the report should end reporting.
func GenerateMonitoringDailyReport(template_engine TemplateEngine,
	client_id string, start uint64, end uint64) (string, error) {
	artifact := template_engine.GetArtifact()

	template_engine.SetEnv("ReportMode", "MONITORING_DAILY")
	template_engine.SetEnv("StartTime", int64(start))
	template_engine.SetEnv("EndTime", int64(end))
	template_engine.SetEnv("ClientId", client_id)
	template_engine.SetEnv("ArtifactName", artifact.Name)

	result := ""
	for _, report := range getArtifactReports(
		artifact, []string{
			"client_event",
			"monitoring_daily",
		}) {
		value, err := template_engine.Execute(report.Template)
		if err != nil {
			return "", err
		}
		result += value
	}

	return result, nil
}

func GenerateArtifactDescriptionReport(
	template_engine TemplateEngine,
	config_obj *config_proto.Config) (
	string, error) {
	artifact := template_engine.GetArtifact()

	repository, err := artifacts.GetGlobalRepository(config_obj)
	if err != nil {
		return "", err
	}

	template_artifact, pres := repository.Get("Server.Internal.ArtifactDescription")
	if pres {
		template_engine.SetEnv("artifact", artifact)
		for _, report := range getArtifactReports(
			template_artifact, []string{"internal"}) {
			return template_engine.Execute(report.Template)
		}
	}
	return "", nil
}

// Get reports from the artifact or generate a default report if it
// does not exist.
func getArtifactReports(
	artifact *artifacts_proto.Artifact,
	report_types []string) []*artifacts_proto.Report {
	reports := []*artifacts_proto.Report{}
	for _, report := range artifact.Reports {
		for _, report_type := range report_types {
			if report.Type == report_type {
				reports = append(reports, report)
			}
		}
	}
	if len(reports) > 0 {
		return reports
	}

	// Generate a default report if none are defined.
	for _, source := range artifact.Sources {
		parameters := ""
		name := artifact.Name

		if source.Name != "" {
			name += "/" + source.Name
			parameters = "source='" + source.Name + "'"
		}

		reports = append(reports, &artifacts_proto.Report{
			Type: report_types[0],
			Template: fmt.Sprintf(`
## %s

{{ Query "SELECT * FROM source(%s) LIMIT 500" | Table }}

`, name, parameters),
		})
	}

	return reports
}

func GenerateServerMonitoringReport(
	template_engine TemplateEngine,
	start, end uint64,
	parameters []*artifacts_proto.ArtifactParameter) (string, error) {

	template_engine.SetEnv("ReportMode", "SERVER_EVENT")
	template_engine.SetEnv("StartTime", int64(start))
	template_engine.SetEnv("EndTime", int64(end))
	template_engine.SetEnv("EndTime", int64(end))
	template_engine.SetEnv("ArtifactName", template_engine.GetArtifact().Name)

	result := ""
	for _, report := range getArtifactReports(
		template_engine.GetArtifact(), []string{"server_event"}) {
		for _, param := range report.Parameters {
			template_engine.SetEnv(param.Name, param.Default)
		}

		// Override with user specified parameters.
		for _, param := range parameters {
			template_engine.SetEnv(param.Name, param.Default)
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
	client_id, flow_id string,
	parameters []*artifacts_proto.ArtifactParameter) (string, error) {
	template_engine.SetEnv("ReportMode", "CLIENT")
	template_engine.SetEnv("FlowId", flow_id)
	template_engine.SetEnv("ClientId", client_id)
	template_engine.SetEnv("ArtifactName", template_engine.GetArtifact().Name)

	result := ""
	for _, report := range getArtifactReports(
		template_engine.GetArtifact(), []string{
			"client",
			"server",
		}) {
		for _, param := range report.Parameters {
			template_engine.SetEnv(param.Name, param.Default)
		}

		// Override with user specified parameters.
		for _, param := range parameters {
			template_engine.SetEnv(param.Name, param.Default)
		}

		value, err := template_engine.Execute(report.Template)
		if err != nil {
			return "", err
		}
		result += value
	}

	return result, nil
}

func GenerateHuntReport(template_engine TemplateEngine,
	hunt_id string,
	parameters []*artifacts_proto.ArtifactParameter) (string, error) {
	template_engine.SetEnv("ReportMode", "HUNT")
	template_engine.SetEnv("HuntId", hunt_id)
	template_engine.SetEnv("ArtifactName", template_engine.GetArtifact().Name)

	result := ""
	for _, report := range getArtifactReports(
		template_engine.GetArtifact(), []string{
			"hunt",
		}) {
		for _, param := range report.Parameters {
			template_engine.SetEnv(param.Name, param.Default)
		}

		// Override with user specified parameters.
		for _, param := range parameters {
			template_engine.SetEnv(param.Name, param.Default)
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
	config_obj *config_proto.Config,
	artifact_name string) (
	*BaseTemplateEngine, error) {
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

	scope := artifacts.MakeScope(repository).AppendVars(env)
	return &BaseTemplateEngine{
		Artifact: artifact,
		Scope:    scope,
		Env:      env,
		logger:   logging.GetLogger(config_obj, &logging.FrontendComponent),
	}, nil
}
