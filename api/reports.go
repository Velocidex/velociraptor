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

	switch in.Type {
	// A CLIENT artifact report is a specific artifact
	// collected from a client.
	case "CLIENT":
		template_engine, err := reporting.NewGuiTemplateEngine(
			config_obj, in.Artifact, params)
		if err != nil {
			return nil, err
		}

		template_data, err := reporting.GenerateClientReport(
			template_engine, in.ClientId, in.FlowId)
		if err != nil {
			return nil, err
		}

		encoded_data, err := json.Marshal(template_engine.Data)
		if err != nil {
			return nil, err
		}

		return &api_proto.GetReportResponse{
			Data:     string(encoded_data),
			Template: template_data,
		}, nil

	// A MONITORING_DAILY report is a report generated
	// over a single day of a monitoring artifact
	case "MONITORING_DAILY":
		template_engine, err := reporting.NewGuiTemplateEngine(
			config_obj, in.Artifact, params)
		if err != nil {
			return nil, err
		}

		template_data, err := reporting.GenerateMonitoringDailyReport(
			template_engine, in.ClientId, in.DayName)
		if err != nil {
			return nil, err
		}

		encoded_data, err := json.Marshal(template_engine.Data)
		if err != nil {
			return nil, err
		}

		return &api_proto.GetReportResponse{
			Data:     string(encoded_data),
			Template: template_data,
		}, nil

	}

	return nil, errors.New("Report type not supported")
}
