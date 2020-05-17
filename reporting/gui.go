// This implements a template renderer for the GUI environment.

package reporting

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"html"
	"log"
	"strings"
	"sync"
	"text/template"

	"github.com/Depado/bfchroma"
	"github.com/Masterminds/sprig"
	"github.com/Velocidex/ordereddict"

	chroma_html "github.com/alecthomas/chroma/formatters/html"
	"github.com/microcosm-cc/bluemonday"
	blackfriday "github.com/russross/blackfriday/v2"
	actions_proto "www.velocidex.com/golang/velociraptor/actions/proto"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/result_sets"
	"www.velocidex.com/golang/vfilter"
)

// The templating language is used to generate markdown and the
// markdown is then converted to html using the blackfriday
// renderer. We do not use golang's html/template because it will
// escape html once, and blackfriday will escape it again.

// To protect against XSS we use bluemonday to restrict the allowed
// tags. It is a bit of a hack though. It is possible for malformed
// input to mess up the page but hopefully not to XSS.
var (
	bm_policy = NewBlueMondayPolicy()
)

type GuiTemplateEngine struct {
	*BaseTemplateEngine
	tmpl         *template.Template
	ctx          context.Context
	Messages     *[]string
	path_manager *NotebookCellPathManager
	Data         map[string]*actions_proto.VQLResponse
}

// Go templates can call functions which take args. The pipeline is
// always called last, so any options must come before it. This
// function takes care of parsing the args in a consistent way -
// keyword options are
func parseOptions(values []interface{}) (*ordereddict.Dict, []interface{}) {
	result := []interface{}{}
	dict := ordereddict.NewDict()
	for i := 0; i < len(values); i++ {
		value := values[i]

		key, ok := value.(string)
		if !ok {
			result = append(result, value)
			continue
		}

		if i+1 < len(values) {
			dict.Set(key, values[i+1])
			i++
			continue
		}
		result = append(result, value)
	}
	return dict, result
}

func (self *GuiTemplateEngine) Table(values ...interface{}) interface{} {
	options, argv := parseOptions(values)
	// Not enough args.
	if len(argv) != 1 {
		return ""
	}

	switch t := argv[0].(type) {
	default:
		return t

	case []*NotebookCellQuery:
		result := ""
		for _, item := range t {
			result += fmt.Sprintf(
				`<div class="panel"><grr-csv-viewer base-url="'v1/GetTable'" `+
					`params='%s' /></div>`, item.Params())
		}
		return result

	case []*ordereddict.Dict:
		if len(t) == 0 { // No rows returned.
			self.Scope.Log("Query produced no rows.")
			return ""
		}

		encoded_rows, err := json.MarshalIndent(t, "", " ")
		if err != nil {
			return self.Error("Error: %v", err)
		}

		parameters, err := options.MarshalJSON()
		if err != nil {
			return self.Error("Error: %v", err)
		}

		key := fmt.Sprintf("table%d", len(self.Data))
		self.Data[key] = &actions_proto.VQLResponse{
			Response: string(encoded_rows),
			Columns:  self.Scope.GetMembers(t[0]),
		}
		return fmt.Sprintf(
			`<div class="panel"><grr-csv-viewer value="data['%s']" /></div>`, key)
	}
}

// Currently supported line chart options:
// 1) xaxis_mode: time - specifies x axis is time since epoch.
func (self *GuiTemplateEngine) LineChart(values ...interface{}) string {
	options, argv := parseOptions(values)
	// Not enough args.
	if len(argv) != 1 {
		return ""
	}

	switch t := argv[0].(type) {
	default:
		return ""

	case []*NotebookCellQuery:
		result := ""
		for _, item := range t {
			result += fmt.Sprintf(
				`<div class="panel"><grr-line-chart base-url="'v1/GetTable'" `+
					`params='%s' /></div>`, item.Params())
		}
		return result

	case []*ordereddict.Dict:
		if len(t) == 0 {
			return ""
		}
		encoded_rows, err := json.MarshalIndent(t, "", " ")
		if err != nil {
			return ""
		}

		parameters, err := options.MarshalJSON()
		if err != nil {
			return ""
		}

		key := fmt.Sprintf("table%d", len(self.Data))
		self.Data[key] = &actions_proto.VQLResponse{
			Response: string(encoded_rows),
			Columns:  self.Scope.GetMembers(t[0]),
		}
		return fmt.Sprintf(
			`<grr-line-chart value="data['%s']" params='%s' />`,
			key, string(parameters))
	}
}

// Currently supported timeline options:
func (self *GuiTemplateEngine) Timeline(values ...interface{}) string {
	options, argv := parseOptions(values)
	// Not enough args.
	if len(argv) != 1 {
		return ""
	}

	switch t := argv[0].(type) {
	default:
		return ""

	case []*NotebookCellQuery:
		result := ""
		for _, item := range t {
			result += fmt.Sprintf(
				`<div class="panel"><grr-timeline base-url="'v1/GetTable'" `+
					`params='%s' /></div>`, item.Params())
		}
		return result

	case []*ordereddict.Dict:
		if len(t) == 0 {
			return ""
		}
		encoded_rows, err := json.MarshalIndent(t, "", " ")
		if err != nil {
			return ""
		}

		parameters, err := options.MarshalJSON()
		if err != nil {
			return ""
		}

		key := fmt.Sprintf("timeline%d", len(self.Data))
		self.Data[key] = &actions_proto.VQLResponse{
			Response: string(encoded_rows),
			Columns:  self.Scope.GetMembers(t[0]),
		}
		return fmt.Sprintf(
			`<grr-timeline value="data['%s']" params='%s' />`,
			key, string(parameters))
	}
}

func (self *GuiTemplateEngine) Execute(template_string string) (string, error) {
	tmpl, err := self.tmpl.Parse(template_string)
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

	/* This is used to dump out the CSS to be included in
	   reporting.scss.

	formatter := chroma_html.New(
		chroma_html.ClassPrefix("chroma"),
		chroma_html.WithClasses())
	formatter.WriteCSS(os.Stdout, styles.Get("github"))
	*/

	// Sanitize the HTML.
	return bm_policy.Sanitize(output_string), nil
}

func (self *GuiTemplateEngine) getMultiLineQuery(query string) (string, error) {
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

func (self *GuiTemplateEngine) Query(queries ...string) interface{} {
	if self.path_manager == nil {
		return self.queryRows(queries...)
	}

	result := []*NotebookCellQuery{}
	for _, query := range queries {
		query, err := self.getMultiLineQuery(query)
		if err != nil {
			self.Error("VQL Error while reporting %s: %v",
				self.Artifact.Name, err)
			return nil
		}

		multi_vql, err := vfilter.MultiParse(query)
		if err != nil {
			self.Error("VQL Error while reporting %s: %v",
				self.Artifact.Name, err)
			return nil
		}

		ctx, cancel := context.WithCancel(self.ctx)
		defer cancel()

		for _, vql := range multi_vql {
			written := false

			// Replace the previously calculated json file.
			path_manager := self.path_manager.NewQueryStorage()
			rs_writer, err := result_sets.NewResultSetWriter(
				self.config_obj, path_manager, true /* truncate */)
			if err != nil {
				self.Error("Error: %v\n", err)
				return ""
			}
			defer rs_writer.Close()

			for row := range vql.Eval(ctx, self.Scope) {
				rs_writer.Write(vfilter.RowToDict(ctx, self.Scope, row))
				written = true
			}

			if written {
				result = append(result, path_manager)
			}
		}
	}
	return result
}

func (self *GuiTemplateEngine) queryRows(queries ...string) []*ordereddict.Dict {
	result := []*ordereddict.Dict{}

	for _, query := range queries {
		query, err := self.getMultiLineQuery(query)
		if err != nil {
			self.Error("VQL Error while reporting %s: %v",
				self.Artifact.Name, err)
			return nil
		}

		multi_vql, err := vfilter.MultiParse(query)
		if err != nil {
			self.Error("VQL Error while reporting %s: %v",
				self.Artifact.Name, err)
			return nil
		}

		ctx, cancel := context.WithCancel(self.ctx)
		defer cancel()

		for _, vql := range multi_vql {
			for row := range vql.Eval(ctx, self.Scope) {
				result = append(result, vfilter.RowToDict(
					ctx, self.Scope, row))

				// Do not let the query collect too many rows
				// - it impacts on server performance.
				if len(result) > 10000 {
					self.Error("Query cancelled because it "+
						"exceeded row count: '%s'", query)
					return result
				}
			}
		}
	}

	return result
}

func (self *GuiTemplateEngine) Error(fmt_str string, argv ...interface{}) string {
	self.Scope.Log(fmt_str, argv...)
	return ""
}

type logWriter struct {
	mu       sync.Mutex
	messages *[]string
}

func (self *logWriter) Write(b []byte) (int, error) {
	self.mu.Lock()
	defer self.mu.Unlock()

	*self.messages = append(*self.messages, string(b))
	return len(b), nil
}

func NewGuiTemplateEngine(
	config_obj *config_proto.Config,
	ctx context.Context,
	principal string,
	notebook_cell_path_manager *NotebookCellPathManager,
	artifact_name string) (
	*GuiTemplateEngine, error) {

	base_engine, err := newBaseTemplateEngine(
		config_obj, principal, artifact_name)
	if err != nil {
		return nil, err
	}

	messages := []string{}
	base_engine.Scope.Logger = log.New(&logWriter{messages: &messages}, "", 0)
	template_engine := &GuiTemplateEngine{
		BaseTemplateEngine: base_engine,
		ctx:                ctx,
		Messages:           &messages,
		path_manager:       notebook_cell_path_manager,
		Data:               make(map[string]*actions_proto.VQLResponse),
	}
	template_engine.tmpl = template.New("").Funcs(sprig.TxtFuncMap()).Funcs(
		template.FuncMap{
			"Query":     template_engine.Query,
			"Scope":     template_engine.GetScope,
			"Table":     template_engine.Table,
			"LineChart": template_engine.LineChart,
			"Timeline":  template_engine.Timeline,
			"Get":       template_engine.getFunction,
			"str":       strval,
		})
	return template_engine, nil
}

func NewBlueMondayPolicy() *bluemonday.Policy {
	p := bluemonday.UGCPolicy()

	p.AllowStandardURLs()

	// Angular directives.
	p.AllowAttrs("value", "params").OnElements("grr-csv-viewer")
	p.AllowAttrs("value", "params").OnElements("grr-line-chart")
	p.AllowAttrs("value", "params").OnElements("grr-timeline")

	// Required for syntax highlighting.
	p.AllowAttrs("class").OnElements("span")
	p.AllowAttrs("class").OnElements("div")
	p.AllowAttrs("class").OnElements("table")
	p.AllowAttrs("class").OnElements("a")

	return p
}
