package downloads

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"sync"

	"github.com/pkg/errors"
	artifacts_proto "www.velocidex.com/golang/velociraptor/artifacts/proto"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/file_store"
	"www.velocidex.com/golang/velociraptor/file_store/api"
	"www.velocidex.com/golang/velociraptor/logging"
	"www.velocidex.com/golang/velociraptor/paths"
	"www.velocidex.com/golang/velociraptor/reporting"
	"www.velocidex.com/golang/velociraptor/services"
	"www.velocidex.com/golang/velociraptor/vql/acl_managers"
	"www.velocidex.com/golang/vfilter"
)

type ReportPart struct {
	Artifact *artifacts_proto.Artifact
	HTML     string
}

func getHTMLTemplate(
	config_obj *config_proto.Config,
	name string, repository services.Repository) (string, error) {
	template_artifact, ok := repository.Get(config_obj, name)
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

func WriteFlowReport(
	config_obj *config_proto.Config,
	scope vfilter.Scope,
	repository services.Repository,
	writer io.Writer,
	flow_id, client_id, template string) error {
	html_template_string, err := getHTMLTemplate(config_obj, template, repository)
	if err != nil {
		return errors.New(fmt.Sprintf("Artifact %v not found %v\n", template, err))
	}

	parts := []*ReportPart{}

	launcher, err := services.GetLauncher(config_obj)
	if err != nil {
		return err
	}
	flow_details, err := launcher.GetFlowDetails(config_obj, client_id, flow_id)
	if err != nil {
		return err
	}

	if flow_details == nil ||
		flow_details.Context == nil ||
		flow_details.Context.Request == nil {
		return errors.New("Invalid flow object")
	}

	for _, name := range flow_details.Context.Request.Artifacts {
		definition, pres := repository.Get(config_obj, name)
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
				config_obj, context.Background(), scope,
				acl_managers.NullACLManager{}, repository,
				definition.Name, false /* sanitize_html */)
			if err != nil {
				scope.Log("Error creating report for %v: %v",
					definition.Name, err)
				continue
			}

			for _, param := range report.Parameters {
				template_engine.SetEnv(param.Name, param.Default)
			}
			res, err := reporting.GenerateClientReport(
				template_engine, client_id, flow_id, nil)
			if err != nil {
				scope.Log("Error creating report for %v: %v",
					definition.Name, err)
				continue
			}

			content_writer.Write([]byte(res))
		}
		parts = append(parts, &ReportPart{
			Artifact: definition, HTML: content_writer.String()})
	}

	template_engine, err := reporting.NewHTMLTemplateEngine(
		config_obj, context.Background(), scope,
		acl_managers.NullACLManager{}, repository,
		template, false /* sanitize_html */)
	if err != nil {
		return err
	}

	template_engine.SetEnv("parts", parts)
	template_engine.SetEnv("ClientId", client_id)
	template_engine.SetEnv("FlowId", flow_id)

	result, err := template_engine.RenderRaw(
		html_template_string, template_engine.Env.ToDict())
	if err != nil {
		return err
	}
	_, err = writer.Write([]byte(result))
	return err
}

func CreateFlowReport(
	config_obj *config_proto.Config,
	scope vfilter.Scope,
	flow_id, client_id, template string,
	wait bool) (api.FSPathSpec, error) {

	hostname := services.GetHostname(config_obj, client_id)
	flow_path_manager := paths.NewFlowPathManager(client_id, flow_id)
	download_file := flow_path_manager.GetReportsFile(hostname)
	lock_file_spec := download_file.SetType(api.PATH_TYPE_FILESTORE_LOCK)

	file_store_factory := file_store.GetFileStore(config_obj)
	writer, err := file_store_factory.WriteFile(download_file)
	if err != nil {
		return nil, err
	}
	err = writer.Truncate()
	if err != nil {
		return nil, err
	}

	lock_file, err := file_store_factory.WriteFile(lock_file_spec)
	if err != nil {
		return nil, err
	}
	lock_file.Close()

	manager, err := services.GetRepositoryManager(config_obj)
	if err != nil {
		return nil, err
	}
	repository, err := manager.GetGlobalRepository(config_obj)
	if err != nil {
		return nil, err
	}

	builder := services.ScopeBuilderFromScope(scope)
	builder.Uploader = nil

	subscope := manager.BuildScope(builder)

	wg := sync.WaitGroup{}
	wg.Add(1)

	go func() {
		defer wg.Done()
		defer writer.Close()
		defer subscope.Close()
		defer func() {
			err := file_store_factory.Delete(lock_file_spec)
			if err != nil {
				logger := logging.GetLogger(config_obj, &logging.GUIComponent)
				logger.Error("Failed to bind to remove lock file for %v: %v",
					download_file, err)
			}

		}()

		err := WriteFlowReport(config_obj, subscope, repository,
			writer, flow_id, client_id, template)
		if err != nil {
			scope.Log("Writing report: %v", err)
		}
	}()

	if wait {
		wg.Wait()
	}

	return download_file, nil
}
