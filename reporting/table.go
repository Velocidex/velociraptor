package reporting

import (
	"io"

	"github.com/olekukonko/tablewriter"
	"www.velocidex.com/golang/velociraptor/utils"
	"www.velocidex.com/golang/vfilter"
)

func OutputRowsToTable(scope vfilter.Scope,
	rows []vfilter.Row,
	out io.Writer) *tablewriter.Table {
	var columns []string

	table := tablewriter.NewWriter(out)
	table.SetAutoFormatHeaders(false)
	table.SetAutoWrapText(false)

	for _, row := range rows {
		string_row := []string{}
		if columns == nil {
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
