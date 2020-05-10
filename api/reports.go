package api

import (
	"encoding/json"
	"strings"

	errors "github.com/pkg/errors"
	context "golang.org/x/net/context"
	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/reporting"
)

func getReport(ctx context.Context,
	config_obj *config_proto.Config,
	principal string,
	in *api_proto.GetReportRequest) (
	*api_proto.GetReportResponse, error) {

	template_engine, err := reporting.NewGuiTemplateEngine(
		config_obj, ctx, principal, nil, in.Artifact)
	if err != nil {
		if strings.HasPrefix(in.Artifact, "Custom.") {
			template_engine, err = reporting.NewGuiTemplateEngine(
				config_obj, ctx, principal, nil,
				strings.TrimPrefix(in.Artifact, "Custom."))
		}
		if err != nil {
			return nil, err
		}
	}
	defer template_engine.Close()

	var template_data string

	switch in.Type {
	default:
		return nil, errors.New("Report type not supported")

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
		Messages: *template_engine.Messages,
		Template: template_data,
	}, nil

}
