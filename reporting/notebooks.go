package reporting

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"html"
	"io"
	"regexp"

	"github.com/Velocidex/yaml/v2"
	"github.com/alexmullins/zip"
	"github.com/pkg/errors"
	actions_proto "www.velocidex.com/golang/velociraptor/actions/proto"
	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/datastore"
	"www.velocidex.com/golang/velociraptor/file_store"
	"www.velocidex.com/golang/velociraptor/utils"
)

var (
	// Must match the output emitted by GuiTemplateEngine.Table
	csvViewerRegexp = regexp.MustCompile(
		`<grr-csv-viewer base-url="'v1/GetTable'" params='([^']+)' />`)
)

const (
	HtmlPreable = `
<html>
  <head>
    <meta charset="utf-8">
    <meta name="viewport" content="width=device-width, initial-scale=1, shrink-to-fit=no">
    <title>%s</title>

    <!-- Bootstrap core CSS -->
    <link rel="stylesheet" href="https://stackpath.bootstrapcdn.com/bootstrap/4.4.1/css/bootstrap.min.css" integrity="sha384-Vkoo8x4CGsO3+Hhxv8T/Q5PaXtkKtu6ug5TOeNV6gBiFeWPGFN9MuhOf23Q9Ifjh" crossorigin="anonymous">
    <script src="https://code.jquery.com/jquery-3.4.1.slim.min.js" integrity="sha384-J6qa4849blE2+poT4WnyKhv5vZF5SrPo0iEjwBvKU7imGFAV0wwj1yYfoRSJoZ+n" crossorigin="anonymous"></script>
    <script src="https://cdn.jsdelivr.net/npm/popper.js@1.16.0/dist/umd/popper.min.js" integrity="sha384-Q6E9RHvbIyZFJoft+2mJbHaEWldlvI9IOYy5n3zV9zzTtmI3UksdQRVvoxMfooAo" crossorigin="anonymous"></script>
    <script src="https://stackpath.bootstrapcdn.com/bootstrap/4.4.1/js/bootstrap.min.js" integrity="sha384-wfSDF2E50Y2D1uUdj0O3uMBJnjuUD4Ih7YwaYd1iqfktj0Uod8GCExl3Og8ifwB6" crossorigin="anonymous"></script>

    <style>
pre {
    display: block;
    padding: 8px;
    margin: 0 0 8.5px;
    font-size: 12px;
    line-height: 1.31;
    word-break: break-all;
    word-wrap: break-word;
    color: #333333;
    background-color: #f5f5f5;
    border: 1px solid #ccc;
    border-radius: 4px;
}

.notebook-cell {
    border-color: transparent;
    display: flex;
    flex-direction: column;
    align-items: stretch;
    border-radius: 20px;
    border-width: 3px;
    border-style: none;
    border-color: #ababab;
    padding: 20px;
    margin: 0px;
    position: relative;
    overflow: auto;
}

/* Error */  .chromaerr { color: #a61717; background-color: #e3d2d2 }
/* LineTableTD */  .chromalntd { vertical-align: top; padding: 0; margin: 0; border: 0; }
/* LineTable */  .chromalntable { border-spacing: 0; padding: 0; margin: 0; border: 0; width: auto; overflow: auto; display: block; }
/* LineHighlight */  .chromahl { display: block; width: 100%%; }
/* LineNumbersTable */  .chromalnt { margin-right: 0.4em; padding: 0 0.4em 0 0.4em; }
/* LineNumbers */  .chromaln { margin-right: 0.4em; padding: 0 0.4em 0 0.4em; }
/* Keyword */  .chromak { color: #000000; font-weight: bold }
/* KeywordConstant */  .chromakc { color: #000000; font-weight: bold }
/* KeywordDeclaration */  .chromakd { color: #000000; font-weight: bold }
/* KeywordNamespace */  .chromakn { color: #000000; font-weight: bold }
/* KeywordPseudo */  .chromakp { color: #000000; font-weight: bold }
/* KeywordReserved */  .chromakr { color: #000000; font-weight: bold }
/* KeywordType */  .chromakt { color: #445588; font-weight: bold }
/* NameAttribute */  .chromana { color: #008080 }
/* NameBuiltin */  .chromanb { color: #0086b3 }
/* NameBuiltinPseudo */  .chromabp { color: #999999 }
/* NameClass */  .chromanc { color: #445588; font-weight: bold }
/* NameConstant */  .chromano { color: #008080 }
/* NameDecorator */  .chromand { color: #3c5d5d; font-weight: bold }
/* NameEntity */  .chromani { color: #800080 }
/* NameException */  .chromane { color: #990000; font-weight: bold }
/* NameFunction */  .chromanf { color: #990000; font-weight: bold }
/* NameLabel */  .chromanl { color: #990000; font-weight: bold }
/* NameNamespace */  .chromann { color: #555555 }
/* NameTag */  .chromant { color: #000080 }
/* NameVariable */  .chromanv { color: #008080 }
/* NameVariableClass */  .chromavc { color: #008080 }
/* NameVariableGlobal */  .chromavg { color: #008080 }
/* NameVariableInstance */  .chromavi { color: #008080 }
/* LiteralString */  .chromas { color: #dd1144 }
/* LiteralStringAffix */  .chromasa { color: #dd1144 }
/* LiteralStringBacktick */  .chromasb { color: #dd1144 }
/* LiteralStringChar */  .chromasc { color: #dd1144 }
/* LiteralStringDelimiter */  .chromadl { color: #dd1144 }
/* LiteralStringDoc */  .chromasd { color: #dd1144 }
/* LiteralStringDouble */  .chromas2 { color: #dd1144 }
/* LiteralStringEscape */  .chromase { color: #dd1144 }
/* LiteralStringHeredoc */  .chromash { color: #dd1144 }
/* LiteralStringInterpol */  .chromasi { color: #dd1144 }
/* LiteralStringOther */  .chromasx { color: #dd1144 }
/* LiteralStringRegex */  .chromasr { color: #009926 }
/* LiteralStringSingle */  .chromas1 { color: #dd1144 }
/* LiteralStringSymbol */  .chromass { color: #990073 }
/* LiteralNumber */  .chromam { color: #009999 }
/* LiteralNumberBin */  .chromamb { color: #009999 }
/* LiteralNumberFloat */  .chromamf { color: #009999 }
/* LiteralNumberHex */  .chromamh { color: #009999 }
/* LiteralNumberInteger */  .chromami { color: #009999 }
/* LiteralNumberIntegerLong */  .chromail { color: #009999 }
/* LiteralNumberOct */  .chromamo { color: #009999 }
/* Operator */  .chromao { color: #000000; font-weight: bold }
/* OperatorWord */  .chromaow { color: #000000; font-weight: bold }
/* Comment */  .chromac { color: #999988; font-style: italic }
/* CommentHashbang */  .chromach { color: #999988; font-style: italic }
/* CommentMultiline */  .chromacm { color: #999988; font-style: italic }
/* CommentSingle */  .chromac1 { color: #999988; font-style: italic }
/* CommentSpecial */  .chromacs { color: #999999; font-weight: bold; font-style: italic }
/* CommentPreproc */  .chromacp { color: #999999; font-weight: bold; font-style: italic }
/* CommentPreprocFile */  .chromacpf { color: #999999; font-weight: bold; font-style: italic }
/* GenericDeleted */  .chromagd { color: #000000; background-color: #ffdddd }
/* GenericEmph */  .chromage { color: #000000; font-style: italic }
/* GenericError */  .chromagr { color: #aa0000 }
/* GenericHeading */  .chromagh { color: #999999 }
/* GenericInserted */  .chromagi { color: #000000; background-color: #ddffdd }
/* GenericOutput */  .chromago { color: #888888 }
/* GenericPrompt */  .chromagp { color: #555555 }
/* GenericStrong */  .chromags { font-weight: bold }
/* GenericSubheading */  .chromagu { color: #aaaaaa }
/* GenericTraceback */  .chromagt { color: #aa0000 }
/* TextWhitespace */  .chromaw { color: #bbbbbb }
</style>
  </head>
  <body>
    <main role="main" class="container">

`

	HtmlPostscript = `
    </main>
   </body>
</html>
`
)

func ExportNotebookToZip(
	ctx context.Context,
	config_obj *config_proto.Config,
	notebook_path_manager *NotebookPathManager) error {

	db, err := datastore.GetDB(config_obj)
	if err != nil {
		return err
	}

	notebook := &api_proto.NotebookMetadata{}
	err = db.GetSubject(config_obj, notebook_path_manager.Path(),
		notebook)
	if err != nil {
		return err
	}

	for _, metadata := range notebook.CellMetadata {
		if metadata.CellId != "" {
			db.GetSubject(config_obj,
				notebook_path_manager.Cell(metadata.CellId).Path(),
				metadata)

			metadata.Data = ""
		}
	}

	serialized, err := yaml.Marshal(notebook)
	if err != nil {
		return err
	}
	fmt.Println(string(serialized))

	file_store_factory := file_store.GetFileStore(config_obj)
	fd, err := file_store_factory.WriteFile(notebook_path_manager.ZipExport())
	if err != nil {
		return err
	}
	defer fd.Close()

	err = fd.Truncate()
	if err != nil {
		return err
	}

	// Do these first to ensure errors are returned if the zip file
	// is not writable.
	zip_writer := zip.NewWriter(fd)
	defer zip_writer.Close()

	cell_copier := func(notebook_id, cell_id string) {
		path_manager := NewNotebookPathManager(notebook_id)
		exported_path_manager := NewNotebookExportPathManager(
			notebook_id)

		children, err := file_store_factory.ListDirectory(
			path_manager.CellDirectory(cell_id))
		if err != nil {
			return
		}

		for _, child := range children {
			out_fd, err := zip_writer.Create(
				exported_path_manager.CellItem(cell_id, child.Name()))
			if err != nil {
				continue
			}

			fd, err := file_store_factory.ReadFile(
				path_manager.Cell(cell_id).Item(child.Name()))
			if err != nil {
				continue
			}
			defer fd.Close()

			utils.Copy(ctx, out_fd, fd)
		}

		return
	}

	for _, cell := range notebook.CellMetadata {
		cell_copier(notebook.NotebookId, cell.CellId)
	}

	f, err := zip_writer.Create("Notebook.yaml")
	if err != nil {
		fd.Close()
		return err
	}
	f.Write(serialized)

	return nil
}

func ExportNotebookToHTML(
	ctx context.Context,
	config_obj *config_proto.Config,
	notebook_id string, output io.Writer) error {

	db, err := datastore.GetDB(config_obj)
	if err != nil {
		return err
	}

	notebook_path_manager := NewNotebookPathManager(notebook_id)
	notebook := &api_proto.NotebookMetadata{}
	err = db.GetSubject(config_obj, notebook_path_manager.Path(),
		notebook)
	if err != nil {
		return err
	}

	output.Write([]byte(fmt.Sprintf(HtmlPreable, notebook.Name)))
	defer output.Write([]byte(HtmlPostscript))

	cell := &api_proto.NotebookCell{}
	for _, cell_md := range notebook.CellMetadata {
		err = db.GetSubject(config_obj,
			notebook_path_manager.Cell(cell_md.CellId).Path(),
			cell)
		if err != nil {
			return err
		}

		output.Write([]byte("<div class=\"notebook-cell\">\n"))

		// Expand tables
		cell_output := csvViewerRegexp.ReplaceAllStringFunc(
			cell.Output, func(in string) string {
				result, err := convertCSVTags(ctx, config_obj, in, cell)
				if err != nil {
					return fmt.Sprintf(
						"<error>%s</error>",
						html.EscapeString(err.Error()))
				}
				return result
			})

		_, err := output.Write([]byte(cell_output))
		output.Write([]byte("</div>\n"))
		if err != nil {
			return err
		}
	}

	return nil
}

// Unpack the table referenced in the csv view tag from the data field.
func convertCSVTags(
	ctx context.Context,
	config_obj *config_proto.Config,
	in string,
	cell *api_proto.NotebookCell) (string, error) {
	output := &bytes.Buffer{}

	data := make(map[string]*actions_proto.VQLResponse)
	err := json.Unmarshal([]byte(cell.Data), &data)
	if err != nil {
		return "", err
	}

	m := csvViewerRegexp.FindStringSubmatch(in)
	if len(m) < 1 {
		return "", errors.New("Unexpected regexp match")
	}

	params := &api_proto.GetTableRequest{}
	err = json.Unmarshal([]byte(m[1]), params)
	if err != nil {
		return "", err
	}

	path_manager := NewNotebookPathManager(params.NotebookId).Cell(
		params.CellId).QueryStorage(params.TableId)
	row_chan, err := file_store.GetTimeRange(ctx, config_obj, path_manager, 0, 0)
	if err != nil {
		return "", err
	}

	headers := false
	for row := range row_chan {
		if !headers {
			output.WriteString("\n<table class=\"table table-striped\">\n <thead>\n")
			output.WriteString("  <tr>\n")
			for _, header := range row.Keys() {
				output.WriteString(fmt.Sprintf(
					"    <th>%s</th>\n", html.EscapeString(header)))
			}
			output.WriteString("  </tr>\n </thead>\n")
			output.WriteString(" <tbody>\n")
			headers = true
		}

		output.WriteString("  <tr>\n")
		for _, header := range row.Keys() {
			value, pres := row.Get(header)
			if !pres {
				value = ""
			}
			output.WriteString(
				fmt.Sprintf("    <td>%s</td>\n",
					html.EscapeString(
						fmt.Sprintf("%v", value))))
		}
		output.WriteString("  </tr>\n")
	}

	output.WriteString(" </tbody>\n")
	output.WriteString("</table>\n")

	return output.String(), nil
}
