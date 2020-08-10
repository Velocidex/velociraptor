// +build server_vql

package tools

import (
	"bytes"
	"context"
	"errors"
	"io"

	"github.com/Velocidex/ordereddict"
	"www.velocidex.com/golang/velociraptor/artifacts"
	artifacts_proto "www.velocidex.com/golang/velociraptor/artifacts/proto"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/reporting"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/velociraptor/vql/server"
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

// Produce a collector report.
func produceReport(
	config_obj *config_proto.Config,
	container *reporting.Container,
	template string,
	repository *artifacts.Repository,
	writer io.Writer,
	definitions []*artifacts_proto.Artifact,
	scope *vfilter.Scope,
	arg *CollectPluginArgs) error {

	builder := artifacts.ScopeBuilderFromScope(scope)
	builder.Uploader = nil

	// Build scope from scratch and replace the source()
	// plugin. We hook the source plugin to read results from the
	// collection container.
	subscope := builder.BuildFromScratch()
	defer subscope.Close()

	// Reports can query the container directly.
	subscope.AppendPlugins(&ContainerSourcePlugin{Container: container})

	html_template_string, err := getHTMLTemplate(template, repository)
	if err != nil {
		return err
	}

	parts := []*ReportPart{}
	main := ""
	for _, definition := range definitions {
		content_writer := &bytes.Buffer{}
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
				return err
			}

			for _, param := range report.Parameters {
				template_engine.SetEnv(param.Name, param.Default)
			}

			res, err := reporting.GenerateClientReport(
				template_engine, "", "", nil)
			if err != nil {
				return err
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
		return err
	}

	template_engine.SetEnv("main", main)
	template_engine.SetEnv("parts", parts)

	result, err := template_engine.RenderRaw(
		html_template_string, template_engine.Env.ToDict())
	if err != nil {
		return err
	}
	_, err = writer.Write([]byte(result))
	return err
}

// A special implementation of the source() plugin which retrieves
// data stored in reporting containers. This only exists in generating
// reports from zip files.
type ContainerSourcePlugin struct {
	server.SourcePlugin
	Container *reporting.Container
}

func (self *ContainerSourcePlugin) Call(
	ctx context.Context,
	scope *vfilter.Scope,
	args *ordereddict.Dict) <-chan vfilter.Row {
	output_chan := make(chan vfilter.Row)

	go func() {
		defer close(output_chan)

		// This plugin will take parameters from environment
		// parameters. This allows its use to be more concise in
		// reports etc where many parameters can be inferred from
		// context.
		arg := &server.SourcePluginArgs{}
		server.ParseSourceArgsFromScope(arg, scope)

		// Allow the plugin args to override the environment scope.
		err := vfilter.ExtractArgs(scope, args, arg)
		if err != nil {
			scope.Log("source: %v", err)
			return
		}

		if arg.Source != "" {
			arg.Artifact = arg.Artifact + "/" + arg.Source
			arg.Source = ""
		}

		for row := range self.Container.ReadArtifactResults(ctx, scope, arg.Artifact) {
			output_chan <- row
		}
	}()

	return output_chan
}

type ArchiveSourcePlugin struct {
	server.SourcePlugin
	Archive *reporting.Archive
}

func (self *ArchiveSourcePlugin) Call(
	ctx context.Context,
	scope *vfilter.Scope,
	args *ordereddict.Dict) <-chan vfilter.Row {
	output_chan := make(chan vfilter.Row)

	go func() {
		defer close(output_chan)

		// This plugin will take parameters from environment
		// parameters. This allows its use to be more concise in
		// reports etc where many parameters can be inferred from
		// context.
		arg := &server.SourcePluginArgs{}
		server.ParseSourceArgsFromScope(arg, scope)

		// Allow the plugin args to override the environment scope.
		err := vfilter.ExtractArgs(scope, args, arg)
		if err != nil {
			scope.Log("source: %v", err)
			return
		}

		if arg.Source != "" {
			arg.Artifact = arg.Artifact + "/" + arg.Source
			arg.Source = ""
		}

		for row := range self.Archive.ReadArtifactResults(ctx, scope, arg.Artifact) {
			output_chan <- row
		}
	}()

	return output_chan
}
