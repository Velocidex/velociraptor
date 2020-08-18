package downloads

import (
	"bytes"
	"context"
	"sync"

	"github.com/pkg/errors"
	"www.velocidex.com/golang/velociraptor/api"
	"www.velocidex.com/golang/velociraptor/artifacts"
	artifacts_proto "www.velocidex.com/golang/velociraptor/artifacts/proto"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/file_store"
	"www.velocidex.com/golang/velociraptor/flows"
	"www.velocidex.com/golang/velociraptor/paths"
	"www.velocidex.com/golang/velociraptor/reporting"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/vfilter"
)

type ReportPart struct {
	Artifact *artifacts_proto.Artifact
	HTML     string
}

func getHTMLTemplate(name string, repository *artifacts.Repository) (string, error) {
	template_artifact, ok := repository.Get(name)
	if !ok || len(template_artifact.Reports) == 0 {
		return "", errors.New("Not found")
	}

	for _, report := range template_artifact.Reports {
		if report.Type == "html" {
			return report.Template, nil
		}
	}
	return "", errors.New("Not found")
}

func createFlowReport(
	config_obj *config_proto.Config,
	scope *vfilter.Scope,
	flow_id, client_id, template string,
	wait bool) (string, error) {

	flow_details, err := flows.GetFlowDetails(config_obj, client_id, flow_id)
	if err != nil {
		return "", err
	}

	if flow_details == nil ||
		flow_details.Context == nil ||
		flow_details.Context.Request == nil {
		return "", errors.New("Invalid flow object")
	}

	hostname := api.GetHostname(config_obj, client_id)
	flow_path_manager := paths.NewFlowPathManager(client_id, flow_id)
	download_file := flow_path_manager.GetReportsFile(hostname).Path()

	file_store_factory := file_store.GetFileStore(config_obj)
	writer, err := file_store_factory.WriteFile(download_file)
	if err != nil {
		return "", err
	}
	err = writer.Truncate()
	if err != nil {
		return "", err
	}

	lock_file, err := file_store_factory.WriteFile(download_file + ".lock")
	if err != nil {
		return "", err
	}
	lock_file.Close()

	repository, err := artifacts.GetGlobalRepository(config_obj)
	if err != nil {
		return "", err
	}

	builder := artifacts.ScopeBuilderFromScope(scope)
	builder.Uploader = nil

	subscope := builder.Build()

	wg := sync.WaitGroup{}
	wg.Add(1)

	go func() {
		defer wg.Done()
		defer writer.Close()
		defer subscope.Close()
		defer file_store_factory.Delete(download_file + ".lock")

		html_template_string, err := getHTMLTemplate(template, repository)
		if err != nil {
			scope.Log("Artifact %v not found %v\n", template, err)
			return
		}

		parts := []*ReportPart{}
		main := ""

		for _, name := range flow_details.Context.Request.Artifacts {
			definition, pres := repository.Get(name)
			if !pres {
				scope.Log("Artifact %v not found %v\n", name, err)
				continue
			}

			content_writer := &bytes.Buffer{}

			scope.Log("Rendering artifact %v\n", definition.Name)
			for _, report := range definition.Reports {
				if report.Type != "client" {
					continue
				}

				// Do not sanitize_html since we are writing a
				// stand along HTML file - artifacts may
				// generate arbitrary HTML.
				template_engine, err := reporting.NewHTMLTemplateEngine(
					config_obj, context.Background(), subscope,
					vql_subsystem.NullACLManager{}, repository,
					definition.Name, false /* sanitize_html */)
				if err != nil {
					scope.Log("Error creating report for %v: %v",
						definition.Name, err)
					continue
				}

				for _, param := range report.Parameters {
					template_engine.SetEnv(param.Name, param.Default)
				}
				template_engine.SetEnv("ClientId", client_id)
				template_engine.SetEnv("FlowId", flow_id)

				res, err := reporting.GenerateClientReport(
					template_engine, "", "", nil)
				if err != nil {
					scope.Log("Error creating report for %v: %v",
						definition.Name, err)
					continue
				}

				content_writer.Write([]byte(res))
			}
			parts = append(parts, &ReportPart{
				Artifact: definition, HTML: content_writer.String()})
			main += content_writer.String()
		}

		template_engine, err := reporting.NewHTMLTemplateEngine(
			config_obj, context.Background(), subscope,
			vql_subsystem.NullACLManager{}, repository,
			template, false /* sanitize_html */)
		if err != nil {
			scope.Log("Error creating report for %v: %v",
				template, err)
			return
		}

		template_engine.SetEnv("main", main)
		template_engine.SetEnv("parts", parts)
		template_engine.SetEnv("ClientId", client_id)
		template_engine.SetEnv("FlowId", flow_id)

		result, err := template_engine.RenderRaw(
			html_template_string, template_engine.Env.ToDict())
		if err != nil {
			scope.Log("Error creating report for %v: %v",
				template, err)
			return
		}
		_, err = writer.Write([]byte(result))
		if err != nil {
			scope.Log("Error creating report for %v: %v",
				template, err)
			return
		}
	}()

	if wait {
		wg.Wait()
	}

	return download_file, nil
}
