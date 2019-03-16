package reporting

import (
	"context"
	"io"

	"github.com/olekukonko/tablewriter"
	"www.velocidex.com/golang/velociraptor/utils"
	"www.velocidex.com/golang/vfilter"
)

func EvalQueryToTable(ctx context.Context,
	scope *vfilter.Scope,
	vql *vfilter.VQL,
	out io.Writer) *tablewriter.Table {

	output_chan := vql.Eval(ctx, scope)
	table := tablewriter.NewWriter(out)

	columns := vql.Columns(scope)
	table.SetHeader(*columns)
	table.SetCaption(true, vql.ToString(scope))
	table.SetAutoFormatHeaders(false)
	table.SetAutoWrapText(false)

	for row := range output_chan {
		string_row := []string{}
		if len(*columns) == 0 {
			members := scope.GetMembers(row)
			table.SetHeader(members)
			columns = &members
		}

		for _, key := range *columns {
			cell := ""
			value, pres := scope.Associative(row, key)
			if pres && !utils.IsNil(value) {
				cell = utils.Stringify(value, scope)
			}
			string_row = append(string_row, cell)
		}

		table.Append(string_row)
		vfilter.ChargeOp(scope)
	}

	return table
}

func OutputRowsToTable(scope *vfilter.Scope,
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
				cell = utils.Stringify(value, scope)
			}
			string_row = append(string_row, cell)
		}

		table.Append(string_row)
	}

	return table
}
