// This implements a template renderer for the GUI environment.

package reporting

import (
	"bytes"
	"context"
	"fmt"
	"html"
	"log"
	"strings"
	"text/template"

	"github.com/Depado/bfchroma"
	"github.com/Masterminds/sprig"
	"github.com/Velocidex/ordereddict"

	chroma_html "github.com/alecthomas/chroma/formatters/html"
	blackfriday "github.com/russross/blackfriday/v2"
	actions_proto "www.velocidex.com/golang/velociraptor/actions/proto"
	artifacts_proto "www.velocidex.com/golang/velociraptor/artifacts/proto"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/json"
	"www.velocidex.com/golang/velociraptor/services"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/vfilter"
)

type HTMLTemplateEngine struct {
	*BaseTemplateEngine
	tmpl         *template.Template
	ctx          context.Context
	log_writer   *logWriter
	Data         map[string]*actions_proto.VQLResponse
	SanitizeHTML bool
}

func (self *HTMLTemplateEngine) Expand(values ...interface{}) interface{} {
	_, argv := parseOptions(values)
	// Not enough args.
	if len(argv) != 1 {
		return ""
	}

	results := []*ordereddict.Dict{}

	switch t := argv[0].(type) {
	default:
		return t

	case chan *ordereddict.Dict:
		for item := range t {
			results = append(results, item)
		}
	}

	return results
}

func renderObject(value interface{}) string {
	serialized := ""
	switch t := value.(type) {
	case vfilter.Null, *vfilter.Null:
		return ""

	case string:
		return t

	case int, int64, int32, int16, int8,
		uint64, uint32, uint16, uint8,
		float64, float32, bool:
		return fmt.Sprintf("%v", value)

	default:
		serialized = "<pre>" + json.StringIndent(value) + "</pre>"
	}
	return serialized
}

func (self *HTMLTemplateEngine) Table(values ...interface{}) interface{} {
	_, argv := parseOptions(values)
	// Not enough args.
	if len(argv) != 1 {
		return ""
	}

	switch t := argv[0].(type) {
	default:
		return t

	case []*ordereddict.Dict:
		columns := []string{}

		result := "<div class=\"table\"><table class=\"table table-striped\"><thead>\n"

		for _, item := range t {
			if len(columns) == 0 {
				columns = item.Keys()
				result += "  <tr>\n"
				for _, name := range columns {
					result += "    <th>" + name + "</th>\n"
				}
				result += "  </tr>\n"
				result += "</thead>\n<tbody>\n"
			}
			result += "  <tr>\n"
			for _, name := range columns {
				value, _ := item.Get(name)
				result += fmt.Sprintf("    <td>%v</td>\n", renderObject(value))
			}
			result += "  </tr>\n"
		}
		result += "</tbody>\n/table></div>\n"
		return result

	case chan *ordereddict.Dict:
		columns := []string{}

		result := "<div class=\"table\"><table class=\"table table-striped\">\n<thead>\n"

		for item := range t {
			if len(columns) == 0 {
				columns = item.Keys()
				result += "  <tr>\n"
				for _, name := range columns {
					result += "    <th>" + name + "</th>\n"
				}
				result += "  </tr>\n"
				result += "</thead>\n<tbody>\n"
			}
			result += "  <tr>\n"
			for _, name := range columns {
				value, _ := item.Get(name)
				result += fmt.Sprintf("    <td>%v</td>\n", renderObject(value))
			}
			result += "  </tr>\n"
		}
		result += "</tbody>\n</table></div>\n"
		return result
	}
}

func (self *HTMLTemplateEngine) Noop(values ...interface{}) string {
	return ""
}

func (self *HTMLTemplateEngine) RenderRaw(
	template_string string, target interface{}) (string, error) {

	tmpl, err := self.tmpl.Parse(SanitizeGoTemplates(template_string))
	if err != nil {
		return "", err
	}

	buffer := &bytes.Buffer{}
	err = tmpl.Execute(buffer, target)
	if err != nil {
		return "", err
	}
	return buffer.String(), nil
}

func (self *HTMLTemplateEngine) Execute(report *artifacts_proto.Report) (string, error) {
	tmpl, err := self.tmpl.Parse(SanitizeGoTemplates(report.Template))
	if err != nil {
		return "", err
	}

	buffer := &bytes.Buffer{}
	err = tmpl.Execute(buffer, self.Artifact)
	if err != nil {
		self.Error("Template Erorr: %v", err)
		return "", err
	}

	// We expect the template to be in markdown format, so now
	// generate the HTML
	output := blackfriday.Run(
		buffer.Bytes(),
		blackfriday.WithRenderer(bfchroma.NewRenderer(
			bfchroma.ChromaOptions(
				chroma_html.ClassPrefix("chroma"),
				chroma_html.WithClasses(true),
				chroma_html.WithLineNumbers(true)),
			bfchroma.Style("github"),
		)))

	// Add classes to various tags
	output_string := strings.ReplaceAll(string(output),
		"<table>", "<table class=\"table table-striped\">")

	if !self.SanitizeHTML {
		return output_string, err
	}

	// Sanitize the HTML.
	return bm_policy.Sanitize(output_string), nil
}

func (self *HTMLTemplateEngine) getMultiLineQuery(query string) (string, error) {
	t := self.tmpl.Lookup(query)
	if t == nil {
		return query, nil
	}

	buf := &bytes.Buffer{}
	err := t.Execute(buf, self.Artifact)
	if err != nil {
		return "", err
	}

	// html/template escapes its template but this
	// is the wrong thing to do for us because we
	// use the template as a work around for
	// text/template actions not spanning multiple
	// lines.
	return html.UnescapeString(buf.String()), nil
}

func (self *HTMLTemplateEngine) Import(artifact, name string) interface{} {
	definition, pres := self.BaseTemplateEngine.Repository.Get(
		self.config_obj, artifact)
	if !pres {
		self.Error("Unknown artifact %v", artifact)
		return ""
	}

	for _, report := range definition.Reports {
		if report.Name == name {
			// We parse the template for new definitions,
			// we dont actually care about the output.
			_, err := self.tmpl.Parse(SanitizeGoTemplates(report.Template))
			if err != nil {
				self.Error("Template Erorr: %v", err)
			}
		}
	}

	return ""
}

func (self *HTMLTemplateEngine) Markdown(values ...string) interface{} {
	result := ""
	for _, value := range values {
		r, err := self.getMultiLineQuery(value)
		if err != nil {
			self.Error("Markdown error: %v", err)
			continue
		}

		result += r
	}

	// We expect the template to be in markdown format, so now
	// generate the HTML
	output := blackfriday.Run(
		[]byte(result),
		blackfriday.WithRenderer(bfchroma.NewRenderer(
			bfchroma.ChromaOptions(
				chroma_html.ClassPrefix("chroma"),
				chroma_html.WithClasses(true),
				chroma_html.WithLineNumbers(true)),
			bfchroma.Style("github"),
		)))

	// Add classes to various tags
	output_string := strings.ReplaceAll(string(output),
		"<table>", "<table class=\"table table-striped\">")

	if !self.SanitizeHTML {
		return output_string
	}

	// Sanitize the HTML.
	return bm_policy.Sanitize(output_string)
}

func (self *HTMLTemplateEngine) Query(queries ...string) interface{} {
	output_chan := make(chan *ordereddict.Dict)

	go func() {
		defer close(output_chan)

		for _, multiquery := range queries {
			query, err := self.getMultiLineQuery(multiquery)
			if err != nil {
				self.Error("VQL Error: %v", err)
				return
			}

			multi_vql, err := vfilter.MultiParse(query)
			if err != nil {
				self.Error("VQL Error: %v", err)
				return
			}

			ctx, cancel := context.WithCancel(self.ctx)
			defer cancel()

			for _, vql := range multi_vql {
				for row := range vql.Eval(ctx, self.Scope) {
					select {
					case <-ctx.Done():
						return
					case output_chan <- vfilter.RowToDict(
						ctx, self.Scope, row):
					}
				}
			}
		}
	}()

	return output_chan
}

func (self *HTMLTemplateEngine) Error(fmt_str string, argv ...interface{}) string {
	self.Scope.Log(fmt_str, argv...)
	return ""
}

func (self *HTMLTemplateEngine) Messages() []string {
	return self.log_writer.Messages()
}

func NewHTMLTemplateEngine(
	config_obj *config_proto.Config,
	ctx context.Context,
	scope vfilter.Scope,
	acl_manager vql_subsystem.ACLManager,
	repository services.Repository,
	artifact_name string,
	sanitize_html bool) (
	*HTMLTemplateEngine, error) {

	base_engine, err := newBaseTemplateEngine(
		config_obj, scope, acl_manager, repository,
		artifact_name)
	if err != nil {
		return nil, err
	}

	log_writer := &logWriter{}
	base_engine.Scope.SetLogger(log.New(log_writer, "", 0))
	template_engine := &HTMLTemplateEngine{
		BaseTemplateEngine: base_engine,
		ctx:                ctx,
		log_writer:         log_writer,
		SanitizeHTML:       sanitize_html,
	}
	template_engine.tmpl = template.New("").Funcs(sprig.TxtFuncMap()).Funcs(
		template.FuncMap{
			"Query":     template_engine.Query,
			"Markdown":  template_engine.Markdown,
			"Scope":     template_engine.GetScope,
			"Table":     template_engine.Table,
			"LineChart": template_engine.Noop,
			"Timeline":  template_engine.Noop,
			"Get":       template_engine.getFunction,
			"Expand":    template_engine.Expand,
			"import":    template_engine.Import,
			"str":       strval,
		})
	return template_engine, nil
}
