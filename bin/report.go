package main

import (
	"fmt"

	kingpin "gopkg.in/alecthomas/kingpin.v2"
	"www.velocidex.com/golang/velociraptor/reporting"
)

var (
	report_command = app.Command(
		"report", "Generate a report.")

	report_command_daily_monitoring = report_command.Command(
		"daily", "Generate a daily report for a client.")

	report_command_daily_monitoring_artifact = report_command_daily_monitoring.Arg(
		"artifact", "The artifact to report on.").
		Required().String()

	report_command_daily_monitoring_client = report_command_daily_monitoring.Arg(
		"client_id", "The client id to generate the report for.").
		Required().String()

	report_command_daily_monitoring_day_name = report_command_daily_monitoring.Arg(
		"day", "The day to generate the report for.").
		String()
)

func doDailyMonitoring() {
	config_obj, err := get_server_config(*config_path)
	kingpin.FatalIfError(err, "Unable to load config file")

	getRepository(config_obj)

	template_engine, err := reporting.NewTextTemplateEngine(
		config_obj, *report_command_daily_monitoring_artifact, *env_map)
	kingpin.FatalIfError(err, "Generating report")

	res, err := reporting.GenerateMonitoringDailyReport(
		template_engine,
		*report_command_daily_monitoring_client,
		*report_command_daily_monitoring_day_name)
	kingpin.FatalIfError(err, "Generating report")
	fmt.Println(res)
}

func init() {
	command_handlers = append(command_handlers, func(command string) bool {
		switch command {
		case report_command_daily_monitoring.FullCommand():
			doDailyMonitoring()

		default:
			return false
		}
		return true
	})
}
