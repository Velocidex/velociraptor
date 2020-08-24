package api

import (
	"fmt"
	"strings"

	errors "github.com/pkg/errors"
	context "golang.org/x/net/context"
	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/constants"
	"www.velocidex.com/golang/velociraptor/json"
	"www.velocidex.com/golang/velociraptor/reporting"
	"www.velocidex.com/golang/velociraptor/services"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
)

func getReport(ctx context.Context,
	config_obj *config_proto.Config,
	acl_manager vql_subsystem.ACLManager,
	repository services.Repository,
	in *api_proto.GetReportRequest) (
	*api_proto.GetReportResponse, error) {

	template_engine, err := reporting.NewGuiTemplateEngine(
		config_obj, ctx, nil, /* default scope */
		acl_manager, repository, nil, in.Artifact)
	if err != nil {
		if strings.HasPrefix(in.Artifact,
			constants.ARTIFACT_CUSTOM_NAME_PREFIX) {
			template_engine, err = reporting.NewGuiTemplateEngine(
				config_obj, ctx, nil, /* default scope */
				acl_manager, repository, nil,
				strings.TrimPrefix(in.Artifact,
					constants.ARTIFACT_CUSTOM_NAME_PREFIX))
		}
		if err != nil {
			return nil, err
		}
	}
	defer template_engine.Close()

	var template_data string

	if in.Type == "" {
		definition, pres := repository.Get(
			config_obj, "Custom."+in.Artifact)
		if !pres {
			definition, pres = repository.Get(config_obj, in.Artifact)
			if pres {
				for _, report := range definition.Reports {
					in.Type = strings.ToUpper(report.Type)
				}
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
		template_data, err = reporting.GenerateArtifactDescriptionReport(
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
