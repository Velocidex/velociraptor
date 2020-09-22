// Produce reports of collected artifacts.

package reporting

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log"
	"os"

	"text/template"

	"github.com/Velocidex/ordereddict"
	"github.com/olekukonko/tablewriter"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/services"
	"www.velocidex.com/golang/velociraptor/utils"
	"www.velocidex.com/golang/vfilter"
)

func EvalQueryToTable(ctx context.Context,
	scope *vfilter.Scope,
	vql *vfilter.VQL,
	out io.Writer) *tablewriter.Table {

	output_chan := vql.Eval(ctx, scope)
	table := tablewriter.NewWriter(out)

	columns := []string{}
	table.SetCaption(true, vql.ToString(scope))
	table.SetAutoFormatHeaders(false)
	table.SetAutoWrapText(false)

	for row := range output_chan {
		string_row := []string{}
		if len(columns) == 0 {
			columns = scope.GetMembers(row)
			table.SetHeader(columns)
		}

		for _, key := range columns {
			cell := ""
			value, pres := scope.Associative(row, key)
			if pres && !utils.IsNil(value) {
				cell = utils.Stringify(value, scope, 120/len(columns))
			}
			string_row = append(string_row, cell)
		}

		table.Append(string_row)
		vfilter.ChargeOp(scope)
	}

	return table
}

type Expansions struct {
	config_obj *config_proto.Config
	rows       []vfilter.Row
	scope      *vfilter.Scope
}

// Support a number of expansions in description strings.
func FormatDescription(
	config_obj *config_proto.Config,
	description string,
	rows []vfilter.Row) string {

	expansion := &Expansions{
		config_obj: config_obj,
		rows:       rows,
	}

	tmpl, err := template.New("description").Funcs(
		template.FuncMap{
			"DocFrom": expansion.DocFrom,
			"Query":   expansion.Query,
		}).Parse(description)
	if err != nil {
		return description
	}

	buffer := &bytes.Buffer{}

	err = tmpl.Execute(buffer, expansion)
	if err != nil {
		return description
	}

	return buffer.String()
}

func (self *Expansions) DocFrom(artifact string) string {
	manager, err := services.GetRepositoryManager()
	if err != nil {
		return ""
	}

	repository, err := manager.GetGlobalRepository(self.config_obj)
	if err != nil {
		return ""
	}

	artifact_definition, pres := repository.Get(self.config_obj, artifact)
	if !pres {
		return ""
	}

	return artifact_definition.Description
}

func (self *Expansions) Query(queries ...string) string {
	result := &bytes.Buffer{}

	env := ordereddict.NewDict().Set("Rows", self.rows)
	scope := self.scope.Copy().AppendVars(env)

	defer scope.Close()

	scope.Logger = log.New(os.Stderr, " ", 0)

	for _, query := range queries {
		vql, err := vfilter.Parse(query)
		if err != nil {
			return fmt.Sprintf("Error: %v", err)
		}
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		table := EvalQueryToTable(ctx, scope, vql, result)
		table.Render()
	}

	return result.String()
}
