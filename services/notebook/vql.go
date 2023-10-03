package notebook

import (
	artifacts_proto "www.velocidex.com/golang/velociraptor/artifacts/proto"
	"www.velocidex.com/golang/velociraptor/paths"
	"www.velocidex.com/golang/velociraptor/reporting"
	"www.velocidex.com/golang/vfilter"
)

type VqlCell struct {
}

type VqlProcessor interface {
	Process(GuiTemplateEngineI, string) (string, error)
}

type GuiTemplateEngineI interface {
	Error(fmt_str string, argv ...interface{}) string
	Execute(report *artifacts_proto.Report) (string, error)
	RunQuery(vql *vfilter.VQL, result []*paths.NotebookCellQuery) ([]*paths.NotebookCellQuery, error)
	Table(values ...interface{}) interface{}
}

func (self *VqlCell) Process(tmpl GuiTemplateEngineI, input string) (string, error) {
	output := ""
	// No query, nothing to do
	if reporting.IsEmptyQuery(input) {
		tmpl.Error("Please specify a query to run")
	} else {
		vqls, err := vfilter.MultiParseWithComments(input)
		if err != nil {
			// Try parsing without comments if comment parser fails
			vqls, err = vfilter.MultiParse(input)
			if err != nil {
				return output, err
			}
		}

		no_query := true
		for _, vql := range vqls {
			if vql.Comments != nil {
				// Only extract multiline comments to render template
				// Ignore code comments
				comments := multiLineCommentsToString(vql)
				if comments != "" {
					fragment_output, err := tmpl.Execute(
						&artifacts_proto.Report{Template: comments})
					if err != nil {
						return output, err
					}
					output += fragment_output
				}
			}
			if vql.Let != "" || vql.Query != nil || vql.StoredQuery != nil {
				no_query = false
				rows, err := tmpl.RunQuery(vql, nil)

				if err != nil {
					return output, err
				}

				// VQL Let won't return rows. Ignore
				if vql.Let == "" {
					output_any, ok := tmpl.Table(rows).(string)
					if ok {
						output += output_any
					}
				}
			}
		}
		// No VQL found, only comments
		if no_query {
			tmpl.Error("Please specify a query to run")
		}
	}

	return output, nil
}

func multiLineCommentsToString(vql *vfilter.VQL) string {
	output := ""

	for _, comment := range vql.Comments {
		if comment.MultiLine != nil {
			output += *comment.MultiLine
		}
	}

	if output != "" {
		return output[2 : len(output)-2]
	} else {
		return output
	}
}
