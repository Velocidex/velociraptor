// This implements a template renderer for the GUI environment.

package reporting

import (
	"bytes"
	"context"
	"fmt"
	"html"
	"log"
	"regexp"
	"strings"
	"sync"
	"text/template"
	"time"

	"github.com/Depado/bfchroma"
	"github.com/Masterminds/sprig"
	"github.com/Velocidex/ordereddict"
	"github.com/pkg/errors"

	chroma_html "github.com/alecthomas/chroma/formatters/html"
	"github.com/microcosm-cc/bluemonday"
	blackfriday "github.com/russross/blackfriday/v2"
	actions_proto "www.velocidex.com/golang/velociraptor/actions/proto"
	artifacts_proto "www.velocidex.com/golang/velociraptor/artifacts/proto"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/file_store"
	"www.velocidex.com/golang/velociraptor/json"
	"www.velocidex.com/golang/velociraptor/paths"
	"www.velocidex.com/golang/velociraptor/result_sets"
	"www.velocidex.com/golang/velociraptor/services"
	"www.velocidex.com/golang/velociraptor/timelines"
	"www.velocidex.com/golang/velociraptor/utils"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/vfilter"
	"www.velocidex.com/golang/vfilter/types"
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

	whitespace_regexp = regexp.MustCompile(`^\s*$`)
)

type GuiTemplateEngine struct {
	*BaseTemplateEngine
	tmpl         *template.Template
	ctx          context.Context
	log_writer   *logWriter
	path_manager *paths.NotebookCellPathManager
	Data         map[string]*actions_proto.VQLResponse
	Progress     utils.ProgressReporter
	Start        time.Time
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

// When rendering a table the user can introduce options into the
// scope.
func (self *GuiTemplateEngine) getTableOptions() (*ordereddict.Dict, error) {
	column_types, pres := self.Scope.Resolve("ColumnTypes")
	if !pres {
		return nil, errors.New("Not found")
	}

	// Reduce the object if needed.
	column_types_lazy, ok := column_types.(types.StoredExpression)
	if ok {
		column_types = column_types_lazy.Reduce(self.ctx, self.Scope)
	}

	serialized, err := json.Marshal(column_types)
	if err != nil {
		return nil, err
	}

	options, err := utils.ParseJsonToDicts(serialized)
	if err != nil {
		return nil, err
	}

	// Not the right number of options
	if len(options) != 1 {
		return ordereddict.NewDict(), nil
	}

	return options[0], nil
}

func (self *GuiTemplateEngine) Expand(values ...interface{}) interface{} {
	_, argv := parseOptions(values)
	// Not enough args.
	if len(argv) != 1 {
		return ""
	}

	results := []interface{}{}

	switch t := argv[0].(type) {
	default:
		return t

	case []*paths.NotebookCellQuery:
		if len(t) == 0 { // No rows returned.
			self.Scope.Log("Query produced no rows.")
			return results
		}

		for _, item := range t {
			file_store_factory := file_store.GetFileStore(self.config_obj)
			reader, err := result_sets.NewResultSetReader(
				file_store_factory, item.Path())
			if err == nil {
				for row := range reader.Rows(self.ctx) {
					results = append(results, row)
				}
				reader.Close()
			}

		}

	case []*ordereddict.Dict:
		if len(t) == 0 { // No rows returned.
			self.Scope.Log("Query produced no rows.")
			return results
		}
		for _, item := range t {
			results = append(results, item)
		}
	}

	return results
}

func (self *GuiTemplateEngine) Import(artifact, name string) interface{} {
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

func (self *GuiTemplateEngine) Table(values ...interface{}) interface{} {
	_, argv := parseOptions(values)
	// Not enough args.
	if len(argv) != 1 {
		return ""
	}

	switch t := argv[0].(type) {
	default:
		return t

	case []*paths.NotebookCellQuery:
		if len(t) == 0 { // No rows returned.
			self.Scope.Log("Query produced no rows.")
			return ""
		}

		table_options, err := self.getTableOptions()
		if err != nil {
			table_options = ordereddict.NewDict()
		}

		result := ""
		for _, item := range t {
			options := item.Params()
			options.Set("TableOptions", table_options)

			result += fmt.Sprintf(
				`<div class="panel"><grr-csv-viewer base-url="'v1/GetTable'" `+
					`params='%s' /></div>`,
				utils.QueryEscape(options.String()))
		}
		return result

	case []*ordereddict.Dict:
		if len(t) == 0 { // No rows returned.
			self.Scope.Log("Query produced no rows.")
			return ""
		}

		opts := vql_subsystem.EncOptsFromScope(self.Scope)
		encoded_rows, err := json.MarshalWithOptions(t, opts)
		if err != nil {
			return self.Error("Error: %v", err)
		}

		key := fmt.Sprintf("table%d", len(self.Data))
		self.Data[key] = &actions_proto.VQLResponse{
			Response: string(encoded_rows),
			Columns:  self.Scope.GetMembers(t[0]),
		}
		return fmt.Sprintf(
			`<div class="panel"><inline-table-viewer value="%s" /></div>`,
			utils.QueryEscape(key))
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

	case []*paths.NotebookCellQuery:
		result := ""
		for _, item := range t {
			params := item.Params()
			params.MergeFrom(options)

			result += fmt.Sprintf(
				`<div class="panel"><notebook-line-chart base-url="'v1/GetTable'" `+
					`params='%s' /></div>`,
				utils.QueryEscape(params.String()))
		}
		return result

	case []*ordereddict.Dict:
		if len(t) == 0 {
			return ""
		}

		opts := vql_subsystem.EncOptsFromScope(self.Scope)
		encoded_rows, err := json.MarshalWithOptions(t, opts)
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
			key, utils.QueryEscape(string(parameters)))
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

	case string:
		timeline_path_manager := self.path_manager.Notebook().
			SuperTimeline(t)
		parameters := "{}"
		reader, err := timelines.NewSuperTimelineReader(self.config_obj, timeline_path_manager, nil)
		if err == nil {
			parameters = json.MustMarshalString(reader.Stat())
		}

		return fmt.Sprintf(
			`<div class="panel"><grr-timeline name='%s' `+
				`params='%s' /></div>`, utils.QueryEscape(t),
			utils.QueryEscape(parameters))

	case []*paths.NotebookCellQuery:
		result := ""
		for _, item := range t {
			result += fmt.Sprintf(
				`<div class="panel"><grr-timeline base-url="'v1/GetTable'" `+
					`params='%s' /></div>`,
				utils.QueryEscape(item.Params().String()))
		}
		return result

	case []*ordereddict.Dict:
		if len(t) == 0 {
			return ""
		}
		opts := vql_subsystem.EncOptsFromScope(self.Scope)
		encoded_rows, err := json.MarshalWithOptions(t, opts)
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
			key, utils.QueryEscape(string(parameters)))
	}
}

func (self *GuiTemplateEngine) Execute(report *artifacts_proto.Report) (string, error) {
	if self.Scope == nil {
		return "", errors.New("Scope not configured")
	}

	template_string := report.Template

	// Hard limit for report generation can be specified in the
	// definition.
	if report.Timeout > 0 {
		ctx, cancel := context.WithTimeout(self.ctx, time.Second*time.Duration(report.Timeout))
		defer cancel()
		self.ctx = ctx
	}

	tmpl, err := self.tmpl.Parse(SanitizeGoTemplates(template_string))
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
	result := bm_policy.Sanitize(output_string)
	return result, nil
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

	result := []*paths.NotebookCellQuery{}
	for _, query := range queries {
		query, err := self.getMultiLineQuery(query)
		if err != nil {
			self.Error("VQL Error: %v", err)
			return nil
		}

		// Specifically trap the empty string.
		if whitespace_regexp.MatchString(query) {
			self.Error("Please specify a query to run")
			return nil
		}

		multi_vql, err := vfilter.MultiParse(query)
		if err != nil {
			self.Error("VQL Error: %v", err)
			return nil
		}

		for _, vql := range multi_vql {
			// Replace the previously calculated json file.
			opts := vql_subsystem.EncOptsFromScope(self.Scope)

			// Ignore LET queries but still run them.
			if vql.Let != "" {
				for range vql.Eval(self.ctx, self.Scope) {
				}
				continue
			}

			path := self.path_manager.NewQueryStorage()
			result = append(result, path)

			file_store_factory := file_store.GetFileStore(self.config_obj)

			rs_writer, err := result_sets.NewResultSetWriter(
				file_store_factory, path.Path(),
				opts, true /* truncate */)
			if err != nil {
				self.Error("Error: %v\n", err)
				return nil
			}
			defer rs_writer.Close()

			rs_writer.Flush()

			row_idx := 0
			next_progress := time.Now().Add(4 * time.Second)
			eval_chan := vql.Eval(self.ctx, self.Scope)

			defer self.Progress.Report("Completed query")

		do_query:
			for {
				select {
				case <-self.ctx.Done():
					return result

				case row, ok := <-eval_chan:
					if !ok {
						break do_query
					}
					row_idx++
					rs_writer.Write(vfilter.RowToDict(self.ctx, self.Scope, row))

					if self.Progress != nil && (row_idx%100 == 0 ||
						time.Now().After(next_progress)) {
						rs_writer.Flush()
						self.Progress.Report(fmt.Sprintf(
							"Total Rows %v", row_idx))
						next_progress = time.Now().Add(4 * time.Second)
					}

					// Report progress even if no row is emitted
				case <-time.After(4 * time.Second):
					rs_writer.Flush()
					self.Progress.Report(fmt.Sprintf(
						"Total Rows %v", row_idx))
					next_progress = time.Now().Add(4 * time.Second)
				}
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
			self.Error("VQL Error: %v", err)
			return nil
		}

		multi_vql, err := vfilter.MultiParse(query)
		if err != nil {
			self.Error("VQL Error: %v", err)
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

func (self *GuiTemplateEngine) Messages() []string {
	return self.log_writer.Messages()
}

type logWriter struct {
	mu       sync.Mutex
	messages []string
}

func (self *logWriter) Write(b []byte) (int, error) {
	self.mu.Lock()
	defer self.mu.Unlock()

	// Do not allow the log messages to accumulate too much.
	self.messages = append(self.messages, string(b))
	if len(self.messages) > 1000 {
		self.messages = nil
	}

	return len(b), nil
}

func (self *logWriter) Messages() []string {
	self.mu.Lock()
	defer self.mu.Unlock()

	return self.messages[:]
}

func NewGuiTemplateEngine(
	config_obj *config_proto.Config,
	ctx context.Context,
	scope vfilter.Scope,
	acl_manager vql_subsystem.ACLManager,
	repository services.Repository,
	notebook_cell_path_manager *paths.NotebookCellPathManager,
	artifact_name string) (
	*GuiTemplateEngine, error) {

	base_engine, err := newBaseTemplateEngine(
		config_obj, scope, acl_manager, repository, artifact_name)
	if err != nil {
		return nil, err
	}

	log_writer := &logWriter{}
	base_engine.Scope.SetLogger(log.New(log_writer, "", 0))
	template_engine := &GuiTemplateEngine{
		BaseTemplateEngine: base_engine,
		ctx:                ctx,
		log_writer:         log_writer,
		path_manager:       notebook_cell_path_manager,
		Data:               make(map[string]*actions_proto.VQLResponse),
		Start:              time.Now(),
	}
	template_engine.tmpl = template.New("").Funcs(sprig.TxtFuncMap()).Funcs(
		template.FuncMap{
			"Query":     template_engine.Query,
			"Scope":     template_engine.GetScope,
			"Table":     template_engine.Table,
			"LineChart": template_engine.LineChart,
			"Timeline":  template_engine.Timeline,
			"Get":       template_engine.getFunction,
			"Expand":    template_engine.Expand,
			"import":    template_engine.Import,
			"str":       strval,
		})
	return template_engine, nil
}

func NewBlueMondayPolicy() *bluemonday.Policy {
	p := bluemonday.UGCPolicy()

	p.AllowStandardURLs()

	// Angular directives.
	p.AllowAttrs("value", "params").OnElements("grr-csv-viewer")
	p.AllowAttrs("value", "params").OnElements("inline-table-viewer")
	p.AllowAttrs("value", "params").OnElements("grr-line-chart")
	p.AllowAttrs("params").OnElements("notebook-line-chart")
	p.AllowAttrs("name", "params").OnElements("grr-timeline")
	p.AllowAttrs("name").OnElements("grr-tool-viewer")

	// Required for syntax highlighting.
	p.AllowAttrs("class").OnElements("span")
	p.AllowAttrs("class").OnElements("div")
	p.AllowAttrs("class").OnElements("table")
	p.AllowAttrs("class").OnElements("a")

	return p
}
