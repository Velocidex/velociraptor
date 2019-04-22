// This implements a template renderer for the GUI environment.

package reporting

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"html/template"

	"github.com/Depado/bfchroma"
	"github.com/alecthomas/chroma/formatters/html"
	blackfriday "gopkg.in/russross/blackfriday.v2"
	actions_proto "www.velocidex.com/golang/velociraptor/actions/proto"
	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	"www.velocidex.com/golang/velociraptor/utils"
	"www.velocidex.com/golang/vfilter"
)

type GuiTemplateEngine struct {
	*BaseTemplateEngine
	tmpl *template.Template
	Data map[string]*actions_proto.VQLResponse
}

// Go templates can call functions which take args. The pipeline is
// always called last, so any options must come before it. This
// function takes care of parsing the args in a consistent way -
// keyword options are
func parseOptions(values []interface{}) (*vfilter.Dict, []interface{}) {
	result := []interface{}{}
	dict := vfilter.NewDict()
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

func (self *GuiTemplateEngine) Table(values ...interface{}) template.HTML {
	options, argv := parseOptions(values)
	// Not enough args.
	if len(argv) != 1 {
		return ""
	}

	rows, ok := argv[0].([]vfilter.Row)
	if !ok { // Not the right type
		return ""
	}

	if len(rows) == 0 { // No rows returned.
		return ""
	}

	encoded_rows, err := json.MarshalIndent(rows, "", " ")
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
		Columns:  self.Scope.GetMembers(rows[0]),
	}
	return template.HTML(fmt.Sprintf(
		`<grr-csv-viewer value="data['%s']" params='%s' />`,
		key, string(parameters)))
}

// Currently supported line chart options:
// 1) xaxis_mode: time - specifies x axis is time since epoch.
func (self *GuiTemplateEngine) LineChart(values ...interface{}) template.HTML {
	options, argv := parseOptions(values)
	// Not enough args.
	if len(argv) != 1 {
		return ""
	}

	rows, ok := argv[0].([]vfilter.Row)
	if !ok { // Not the right type
		return ""
	}

	if len(rows) == 0 {
		return ""
	}
	encoded_rows, err := json.MarshalIndent(rows, "", " ")
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
		Columns:  self.Scope.GetMembers(rows[0]),
	}
	return template.HTML(fmt.Sprintf(
		`<grr-line-chart value="data['%s']" params='%s' />`, key, string(parameters)))

}

func (self *GuiTemplateEngine) Execute(template_string string) (string, error) {
	tmpl, err := self.tmpl.Parse(template_string)
	if err != nil {
		return "", err
	}

	buffer := &bytes.Buffer{}
	err = tmpl.Execute(buffer, nil)
	if err != nil {
		utils.Debug(err)
		return "", err
	}

	// We expect the template to be in markdown format, so now generate the HTML
	output := blackfriday.Run(
		buffer.Bytes(),
		blackfriday.WithRenderer(bfchroma.NewRenderer(
			bfchroma.ChromaOptions(html.WithLineNumbers()),
			bfchroma.Style("github"),
		)))
	return string(output), nil
}

func (self *GuiTemplateEngine) Query(queries ...string) []vfilter.Row {
	result := []vfilter.Row{}

	for _, query := range queries {
		t := self.tmpl.Lookup(query)
		if t != nil {
			buf := &bytes.Buffer{}
			err := t.Execute(buf, nil)
			if err != nil {
				self.logger.Err("Template Error (%s): %v",
					self.Artifact.Name, err)
				return []vfilter.Row{}
			}
			query = buf.String()
		}

		vql, err := vfilter.Parse(query)
		if err != nil {
			self.logger.Err("VQL Error while reporting %s: %v",
				self.Artifact.Name, err)
			return result
		}

		for row := range vql.Eval(context.Background(), self.Scope) {
			result = append(result, row)
		}
	}

	return result
}

func NewGuiTemplateEngine(config_obj *api_proto.Config,
	artifact_name string,
	parameters map[string]string) (*GuiTemplateEngine, error) {
	base_engine, err := newBaseTemplateEngine(
		config_obj, artifact_name, parameters)
	if err != nil {
		return nil, err
	}

	template_engine := &GuiTemplateEngine{
		BaseTemplateEngine: base_engine,
		Data:               make(map[string]*actions_proto.VQLResponse),
	}
	template_engine.tmpl = template.New("").Funcs(
		template.FuncMap{
			"Query":     template_engine.Query,
			"Scope":     template_engine.GetScope,
			"Table":     template_engine.Table,
			"LineChart": template_engine.LineChart,
			"Get":       template_engine.getFunction,
			"str":       strval,
		})
	return template_engine, nil
}
