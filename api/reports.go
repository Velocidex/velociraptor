package api

import (
	"encoding/json"

	errors "github.com/pkg/errors"
	context "golang.org/x/net/context"
	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	"www.velocidex.com/golang/velociraptor/reporting"
)

func getReport(ctx context.Context,
	config_obj *api_proto.Config, in *api_proto.GetReportRequest) (
	*api_proto.GetReportResponse, error) {

	params := make(map[string]string)
	for _, env := range in.Parameters {
		params[env.Key] = env.Value
	}

	template_engine, err := reporting.NewGuiTemplateEngine(
		config_obj, ctx, in.Artifact, params)
	if err != nil {
		return nil, err
	}
	defer template_engine.Close()

	var template_data string

	switch in.Type {
	default:
		return nil, errors.New("Report type not supported")

	// A CLIENT artifact report is a specific artifact
	// collected from a client.
	case "CLIENT":
		template_data, err = reporting.GenerateClientReport(
			template_engine, in.ClientId, in.FlowId)
		if err != nil {
			return nil, err
		}

	case "SERVER_EVENT":
		template_data, err = reporting.
			GenerateServerMonitoringReport(
				template_engine, in.StartTime, in.EndTime)
		if err != nil {
			return nil, err
		}

	// A MONITORING_DAILY report is a report generated
	// over a single day of a monitoring artifact
	case "MONITORING_DAILY", "CLIENT_EVENT":
		template_data, err = reporting.GenerateMonitoringDailyReport(
			template_engine, in.ClientId, in.StartTime, in.EndTime)
		if err != nil {
			return nil, err
		}

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
