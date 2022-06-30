package tools

import (
	"bytes"
	"context"
	"errors"
	"io"

	"github.com/Velocidex/ordereddict"
	artifacts_proto "www.velocidex.com/golang/velociraptor/artifacts/proto"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/reporting"
	"www.velocidex.com/golang/velociraptor/services"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/vfilter"
	"www.velocidex.com/golang/vfilter/arg_parser"
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

// Produce a collector report.
func produceReport(
	ctx context.Context,
	config_obj *config_proto.Config,
	archive *reporting.Archive,
	template string,
	repository services.Repository,
	writer io.Writer,
	definitions []*artifacts_proto.Artifact,
	scope vfilter.Scope,
	arg *CollectPluginArgs) error {

	builder := services.ScopeBuilderFromScope(scope)
	builder.Repository = repository
	builder.Uploader = nil

	// Build scope from scratch and replace the source()
	// plugin. We hook the source plugin to read results from the
	// collection container.
	manager, err := services.GetRepositoryManager(config_obj)
	if err != nil {
		return err
	}
	subscope := manager.BuildScopeFromScratch(builder)
	defer subscope.Close()

	// Reports can query the container directly.
	subscope.AppendPlugins(&ArchiveSourcePlugin{Archive: archive})

	html_template_string, err := getHTMLTemplate(config_obj, template, repository)
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
				config_obj, ctx, subscope,
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
		config_obj, ctx, subscope,
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

type SourcePluginArgs struct {
	Artifact string `vfilter:"optional,field=artifact,doc=The name of the artifact collection to fetch"`
	Source   string `vfilter:"optional,field=source,doc=An optional named source within the artifact"`
}

type ArchiveSourcePlugin struct {
	Archive *reporting.Archive
}

func (self *ArchiveSourcePlugin) Call(
	ctx context.Context,
	scope vfilter.Scope,
	args *ordereddict.Dict) <-chan vfilter.Row {
	output_chan := make(chan vfilter.Row)

	go func() {
		defer close(output_chan)

		// This plugin will take parameters from environment
		// parameters. This allows its use to be more concise in
		// reports etc where many parameters can be inferred from
		// context.
		arg := &SourcePluginArgs{}
		ParseSourceArgsFromScope(arg, scope)

		// Allow the plugin args to override the environment scope.
		err := arg_parser.ExtractArgsWithContext(ctx, scope, args, arg)
		if err != nil {
			scope.Log("source: %v", err)
			return
		}

		if arg.Source != "" {
			arg.Artifact = arg.Artifact + "/" + arg.Source
			arg.Source = ""
		}

		for row := range self.Archive.ReadArtifactResults(ctx, scope, arg.Artifact) {
			select {
			case <-ctx.Done():
				return
			case output_chan <- row:
			}
		}
	}()

	return output_chan
}

func (self ArchiveSourcePlugin) Info(
	scope vfilter.Scope, type_map *vfilter.TypeMap) *vfilter.PluginInfo {
	return &vfilter.PluginInfo{
		Name:    "source",
		Doc:     "Retrieve rows from stored result sets. This is a one stop show for retrieving stored result set for post processing.",
		ArgType: type_map.AddType(scope, &SourcePluginArgs{}),
	}
}

func ParseSourceArgsFromScope(arg *SourcePluginArgs, scope vfilter.Scope) {
	artifact_name, pres := scope.Resolve("ArtifactName")
	if pres {
		arg.Artifact = artifact_name.(string)
	}
}
