package api

import (
	"context"
	"fmt"
	"strings"

	errors "github.com/go-errors/errors"
	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/constants"
	"www.velocidex.com/golang/velociraptor/json"
	"www.velocidex.com/golang/velociraptor/paths"
	"www.velocidex.com/golang/velociraptor/reporting"
	"www.velocidex.com/golang/velociraptor/services"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
)

// Reports are used for various dashboards. They are almost like a
// notebook (but historically predate it).
// TODO: Think about consolidating reports and notebooks
// Currently this is used from:
// 1. Home screen (dashboard)  (type: "SERVER_EVENT")
// 2. Client's VQL Drilldown screen (type: "CLIENT")
// 3. View Artifacts screen (type: "ARTIFACT_DESCRIPTION")
func getReport(ctx context.Context,
	config_obj *config_proto.Config,
	acl_manager vql_subsystem.ACLManager,
	repository services.Repository,
	in *api_proto.GetReportRequest) (
	*api_proto.GetReportResponse, error) {

	// Dashboards receive their own notebook ID in a predictable
	// location.
	bare_artifact_name := strings.TrimPrefix(in.Artifact,
		constants.ARTIFACT_CUSTOM_NAME_PREFIX)

	notebook_cell_path_manager := paths.NewDashboardPathManager(
		in.Type, bare_artifact_name, in.ClientId)

	template_engine, err := reporting.NewGuiTemplateEngine(
		config_obj, ctx, nil, /* default scope */
		acl_manager, repository,
		notebook_cell_path_manager,
		in.Artifact)
	if err != nil {
		if strings.HasPrefix(in.Artifact,
			constants.ARTIFACT_CUSTOM_NAME_PREFIX) {
			template_engine, err = reporting.NewGuiTemplateEngine(
				config_obj, ctx, nil, /* default scope */
				acl_manager, repository,
				notebook_cell_path_manager,
				bare_artifact_name)
		}
		if err != nil {
			return nil, err
		}
	}
	defer template_engine.Close()

	var template_data string

	if in.Type == "" {
		definition, pres := repository.Get(
			ctx, config_obj, "Custom."+in.Artifact)
		if !pres {
			definition, pres = repository.Get(ctx, config_obj, in.Artifact)
		}
		if pres {
			for _, report := range definition.Reports {
				in.Type = strings.ToUpper(report.Type)
			}
		}
	}

	switch in.Type {
	default:
		return nil, errors.New(fmt.Sprintf(
			"Report type %v not supported", in.Type))

	// A CLIENT artifact report is a specific artifact
	// collected from a client.
	case "CLIENT", "SERVER":
		template_data, err = reporting.GenerateClientReport(
			template_engine, in.ClientId, in.FlowId,
			in.Parameters)

	case "HUNT":
		template_data, err = reporting.GenerateHuntReport(
			template_engine, in.HuntId,
			in.Parameters)

	// Server event artifacts run on the server. Typically they
	// post process client event streams.
	case "SERVER_EVENT":
		template_data, err = reporting.
			GenerateServerMonitoringReport(
				template_engine,
				in.StartTime, in.EndTime,
				in.Parameters)

	// A MONITORING_DAILY report is a report generated
	// over a single day of a monitoring artifact
	case "MONITORING_DAILY", "CLIENT_EVENT":
		template_data, err = reporting.GenerateMonitoringDailyReport(
			template_engine, in.ClientId, in.StartTime, in.EndTime)

	case "ARTIFACT_DESCRIPTION":
		template_data, err = reporting.GenerateArtifactDescriptionReport(ctx,
			template_engine, config_obj)
	}

	if err != nil {
		return nil, err
	}

	encoded_data, err := json.Marshal(template_engine.Data)
	if err != nil {
		return nil, err
	}

	return &api_proto.GetReportResponse{
		Data:     string(encoded_data),
		Messages: template_engine.Messages(),
		Template: template_data,
	}, nil

}
