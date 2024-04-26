// Produce reports of collected artifacts.

package reporting

import (
	"context"
	"io"

	"github.com/olekukonko/tablewriter"
	"www.velocidex.com/golang/velociraptor/utils"
	"www.velocidex.com/golang/vfilter"
)

func EvalQueryToTable(ctx context.Context,
	scope vfilter.Scope,
	vql *vfilter.VQL,
	out io.Writer) *tablewriter.Table {

	output_chan := vql.Eval(ctx, scope)
	table := tablewriter.NewWriter(out)

	columns := []string{}
	table.SetCaption(true, vfilter.FormatToString(scope, vql))
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
	}

	return table
}
