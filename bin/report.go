package main

import (
	"fmt"
	"time"

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
		config_obj, *report_command_daily_monitoring_artifact)
	kingpin.FatalIfError(err, "Generating report")

	for k, v := range *env_map {
		template_engine.SetEnv(k, v)
	}

	ts, err := time.Parse("2006-01-02", *report_command_daily_monitoring_day_name)
	kingpin.FatalIfError(err, "Invalid day name (e.g. 2019-02-28)")

	res, err := reporting.GenerateMonitoringDailyReport(
		template_engine,
		*report_command_daily_monitoring_client,
		uint64(ts.Unix()), uint64(ts.Unix()+60*60*24))
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
