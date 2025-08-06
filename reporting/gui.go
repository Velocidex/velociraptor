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
	"text/template"
	"time"

	"github.com/Depado/bfchroma"
	"github.com/Masterminds/sprig/v3"
	"github.com/Velocidex/ordereddict"
	"github.com/go-errors/errors"

	chroma_html "github.com/alecthomas/chroma/formatters/html"
	"github.com/microcosm-cc/bluemonday"
	blackfriday "github.com/russross/blackfriday/v2"
	"www.velocidex.com/golang/velociraptor/actions"
	actions_proto "www.velocidex.com/golang/velociraptor/actions/proto"
	artifacts_proto "www.velocidex.com/golang/velociraptor/artifacts/proto"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/constants"
	"www.velocidex.com/golang/velociraptor/file_store"
	"www.velocidex.com/golang/velociraptor/json"
	"www.velocidex.com/golang/velociraptor/paths"
	"www.velocidex.com/golang/velociraptor/result_sets"
	"www.velocidex.com/golang/velociraptor/services"
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
	log_writer   *notebookCellLogger
	path_manager *paths.NotebookCellPathManager
	Data         map[string]*actions_proto.VQLResponse
	Progress     utils.ProgressReporter
	Start        time.Time
}

// Go templates can call functions which take args. The pipeline is
// always called last, so any options must come before it. This
// function takes care of parsing the args in a consistent way -
// keyword options are extracted first and argv contains non keyword
// options.
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
	column_types, pres := self.Scope.Resolve(constants.COLUMN_TYPES)
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
	definition, pres := self.BaseTemplateEngine.Repository.Get(self.ctx,
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
			options.Set("Version", utils.GetTime().Now().Unix())

			result += fmt.Sprintf(
				`<div class="panel"><velo-csv-viewer base-url="'v1/GetTable'" `+
					`params='%s' /></div>`,
				utils.QueryEscape(options.String()))
		}
		return result
	}
}

func (self *GuiTemplateEngine) SigmaEditor(values ...interface{}) string {
	options, _ := parseOptions(values)
	return fmt.Sprintf(
		`<velo-sigma-editor params="%s" />`, utils.QueryEscape(options.String()))
}

func (self *GuiTemplateEngine) LineChart(values ...interface{}) string {
	return self.genericChart("velo-line-chart", "notebook-line-chart", values...)
}

func (self *GuiTemplateEngine) TimeChart(values ...interface{}) string {
	return self.genericChart("time-chart", "notebook-time-chart", values...)
}

func (self *GuiTemplateEngine) BarChart(values ...interface{}) string {
	return self.genericChart("bar-chart", "notebook-bar-chart", values...)
}

func (self *GuiTemplateEngine) ScatterChart(values ...interface{}) string {
	return self.genericChart("scatter-chart", "notebook-scatter-chart", values...)
}

func (self *GuiTemplateEngine) genericChart(
	report_directive, notebook_directive string, values ...interface{}) string {
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
			params.Set("Version", utils.GetTime().Now().Unix())

			result += fmt.Sprintf(
				`<div class="panel"><%s base-url="'v1/GetTable'" `+
					`params='%s' /></div>`,
				notebook_directive,
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
			`<%s value="data['%s']" params='%s' />`,
			report_directive, key, utils.QueryEscape(string(parameters)))
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
		notebook_manager, err := services.GetNotebookManager(self.config_obj)
		if err != nil {
			return ""
		}

		timelines, err := notebook_manager.Timelines(self.ctx,
			self.path_manager.NotebookId())
		if err != nil {
			return ""
		}

		for _, timeline := range timelines {
			if timeline.Name == t {
				parameters := json.MustMarshalString(timeline)
				return fmt.Sprintf(
					`<div class="panel"><velo-timeline name='%s' `+
						`params='%s' /></div>`, utils.QueryEscape(t),
					utils.QueryEscape(parameters))
			}
		}

		// If we get here the timeline does not exist, we just make it
		// up
		return fmt.Sprintf(
			`<div class="panel"><velo-timeline name='%s' `+
				`params='%s' /></div>`, utils.QueryEscape(t),
			utils.QueryEscape("{}"))

	case []*paths.NotebookCellQuery:
		result := ""
		for _, item := range t {
			result += fmt.Sprintf(
				`<div class="panel"><velo-timeline base-url="'v1/GetTable'" `+
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
			`<velo-timeline value="data['%s']" params='%s' />`,
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
	defer utils.CheckForPanic("Evaluating Query")

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
		if IsEmptyQuery(query) {
			self.Error("Please specify a query to run")
			return nil
		}

		multi_vql, err := vfilter.MultiParse(query)
		if err != nil {
			self.Error("VQL Error: %v", err)
			return nil
		}

		for _, vql := range multi_vql {
			result, err = self.RunQuery(vql, result)
			if err != nil {
				self.Error("Error: %v\n", err)
				return nil
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

// Render the special GUI markup for given value.
func (self *GuiTemplateEngine) renderFunction(a interface{}, opts ...interface{}) interface{} {
	switch t := a.(type) {
	case time.Time:
		res := fmt.Sprintf(`<velo-value value="%v"></velo-value>`,
			t.Format(time.RFC3339))
		return res
	}

	return a
}

func (self *GuiTemplateEngine) Error(fmt_str string, argv ...interface{}) string {
	self.Scope.Log(fmt_str, argv...)
	return ""
}

func (self *GuiTemplateEngine) Messages() []string {
	return self.log_writer.Messages()
}

func (self *GuiTemplateEngine) MoreMessages() bool {
	return self.log_writer.MoreMessages()
}

func (self *GuiTemplateEngine) Close() {
	if self.log_writer != nil {
		self.log_writer.Flush()
		self.log_writer.Close()
	}
	self.BaseTemplateEngine.Close()
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

	uploader := &NotebookUploader{
		config_obj:                 config_obj,
		notebook_cell_path_manager: notebook_cell_path_manager,
	}

	base_engine, err := newBaseTemplateEngine(
		ctx, config_obj, scope, acl_manager,
		uploader, repository, artifact_name)
	if err != nil {
		return nil, err
	}

	// Write logs to this result set.
	log_writer, err := newNotebookCellLogger(ctx,
		config_obj, notebook_cell_path_manager.Logs())
	if err != nil {
		return nil, err
	}

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
			"Query":        template_engine.Query,
			"Scope":        template_engine.GetScope,
			"Table":        template_engine.Table,
			"BarChart":     template_engine.BarChart,
			"LineChart":    template_engine.LineChart,
			"ScatterChart": template_engine.ScatterChart,
			"TimeChart":    template_engine.TimeChart,
			"SigmaEditor":  template_engine.SigmaEditor,
			"Timeline":     template_engine.Timeline,
			"Get":          template_engine.GetFunction,
			"Render":       template_engine.renderFunction,
			"Expand":       template_engine.Expand,
			"import":       template_engine.Import,
			"str":          utils.ToString,

			// Remove sprig's functions
			"env": func() string {
				return ""
			},
			"expandenv": func() string {
				return ""
			},
		})
	return template_engine, nil
}

func NewBlueMondayPolicy() *bluemonday.Policy {
	p := bluemonday.UGCPolicy()

	p.AllowStandardURLs()
	// DATA urls are useful for markdown cells
	p.AllowURLSchemes("http", "https", "data")

	// Directives for the GUI.
	p.AllowAttrs("value", "params").OnElements("velo-csv-viewer")
	p.AllowAttrs("value", "params").OnElements("velo-line-chart")
	p.AllowAttrs("value", "params").OnElements("velo-sigma-editor")
	p.AllowAttrs("value", "params").OnElements("bar-chart")
	p.AllowAttrs("value", "params").OnElements("scatter-chart")
	p.AllowAttrs("value", "params").OnElements("time-chart")
	p.AllowAttrs("value").OnElements("velo-value")

	//p.AllowNoAttrs().OnElements("accordion")
	p.AllowAttrs("params").OnElements("notebook-bar-chart")
	p.AllowAttrs("params").OnElements("notebook-line-chart")
	p.AllowAttrs("params").OnElements("notebook-scatter-chart")
	p.AllowAttrs("params").OnElements("notebook-time-chart")
	p.AllowAttrs("name", "params").OnElements("velo-timeline")
	p.AllowAttrs("name", "version").OnElements("velo-tool-viewer")

	// Required for syntax highlighting.
	p.AllowAttrs("class").OnElements("span")
	p.AllowAttrs("class").OnElements("div")
	p.AllowAttrs("class").OnElements("table")
	p.AllowAttrs("class").OnElements("a")

	return p
}

func (self *GuiTemplateEngine) RunQuery(vql *vfilter.VQL,
	result []*paths.NotebookCellQuery) ([]*paths.NotebookCellQuery, error) {
	query_log := actions.QueryLog.AddQuery(
		vfilter.FormatToString(self.Scope, vql))
	defer query_log.Close()

	if result == nil {
		result = []*paths.NotebookCellQuery{}
	}
	opts := vql_subsystem.EncOptsFromScope(self.Scope)

	// Ignore LET queries but still run them.
	if vql.Let != "" {
		for range vql.Eval(self.ctx, self.Scope) {
		}
		return result, nil
	}

	path := self.path_manager.NewQueryStorage()
	result = append(result, path)

	file_store_factory := file_store.GetFileStore(self.config_obj)
	rs_writer, err := result_sets.NewResultSetWriter(
		file_store_factory, path.Path(),
		opts, utils.SyncCompleter,
		result_sets.TruncateMode)
	if err != nil {
		self.Error("Error: %v\n", err)
		return result, nil
	}

	// We must ensure results are visible immediately because
	// the GUI will need to refresh the cell content as soon
	// as we complete.
	rs_writer.SetSync()

	defer rs_writer.Close()

	rs_writer.Flush()

	row_idx := 0
	next_progress := time.Now().Add(4 * time.Second)
	eval_chan := vql.Eval(self.ctx, self.Scope)

	if self.Progress != nil {
		defer self.Progress.Report("Completed query")
	}

	for {
		select {
		case <-self.ctx.Done():
			return result, nil

		case row, ok := <-eval_chan:
			if !ok {
				return result, nil
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
			if self.Progress != nil {
				self.Progress.Report(fmt.Sprintf(
					"Total Rows %v", row_idx))
			}
			next_progress = time.Now().Add(4 * time.Second)

		}
	}
}

func IsEmptyQuery(query string) bool {
	return whitespace_regexp.MatchString(query)
}
