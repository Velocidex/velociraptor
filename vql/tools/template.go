package tools

import (
	"bytes"
	"context"
	"text/template"

	"github.com/Masterminds/sprig/v3"
	"github.com/Velocidex/ordereddict"
	"www.velocidex.com/golang/velociraptor/reporting"
	"www.velocidex.com/golang/velociraptor/utils"
	"www.velocidex.com/golang/velociraptor/vql"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/vfilter"
	"www.velocidex.com/golang/vfilter/arg_parser"
)

type TemplateFunctionArgs struct {
	Template  string            `vfilter:"required,field=template,doc=A Go Template compatible string."`
	Expansion *ordereddict.Dict `vfilter:"required,field=expansion,doc=An object to be expanded into the template."`
}

type TemplateFunction struct{}

func (self *TemplateFunction) Call(ctx context.Context,
	scope vfilter.Scope,
	args *ordereddict.Dict) vfilter.Any {
	arg := &TemplateFunctionArgs{}

	err := arg_parser.ExtractArgsWithContext(ctx, scope, args, arg)
	if err != nil {
		scope.Log("template: %s", err.Error())
		return false
	}

	template_engine := &reporting.BaseTemplateEngine{
		Scope: scope,
	}

	tmpl := template.New("").Funcs(sprig.TxtFuncMap()).Funcs(
		template.FuncMap{
			"Scope": template_engine.GetScope,
			"Get":   template_engine.GetFunction,
			"str":   utils.ToString,
			"env": func() string {
				return ""
			},
			"expandenv": func() string {
				return ""
			},
		})

	template, err := tmpl.Parse(reporting.SanitizeGoTemplates(arg.Template))
	if err != nil {
		scope.Log("template: %v", err)
		return vfilter.Null{}
	}

	buffer := &bytes.Buffer{}
	err = template.Execute(buffer, arg.Expansion.ToMap())
	if err != nil {
		scope.Log("template: %v", err)
		return vfilter.Null{}
	}

	return string(buffer.Bytes())
}

func (self *TemplateFunction) Info(scope vfilter.Scope, type_map *vfilter.TypeMap) *vfilter.FunctionInfo {
	return &vfilter.FunctionInfo{
		Name:     "template",
		Doc:      "Expand a Go style template .",
		ArgType:  type_map.AddType(scope, &TemplateFunctionArgs{}),
		Metadata: vql.VQLMetadata().Build(),
	}
}

func init() {
	vql_subsystem.RegisterFunction(&TemplateFunction{})
}
