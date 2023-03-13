package reporting

import (
	"bytes"
	"context"
	"encoding/base64"
	"fmt"
	"html"
	"io"
	"io/ioutil"
	"net/url"
	"os"
	"regexp"
	"sync"
	"time"

	"github.com/Velocidex/yaml/v2"
	"github.com/go-errors/errors"
	"www.velocidex.com/golang/velociraptor/accessors"
	actions_proto "www.velocidex.com/golang/velociraptor/actions/proto"
	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/datastore"
	"www.velocidex.com/golang/velociraptor/file_store"
	"www.velocidex.com/golang/velociraptor/file_store/api"
	"www.velocidex.com/golang/velociraptor/json"
	"www.velocidex.com/golang/velociraptor/logging"
	"www.velocidex.com/golang/velociraptor/paths"
	"www.velocidex.com/golang/velociraptor/result_sets"
	"www.velocidex.com/golang/velociraptor/utils"
)

var (
	// Must match the output emitted by GuiTemplateEngine.Table
	csvViewerRegexp = regexp.MustCompile(
		`<grr-csv-viewer base-url="'v1/GetTable'" params='([^']+)' />`)

	imageRegex = regexp.MustCompile(
		`<img src=\"/notebooks/(?P<NotebookId>N.[^/]+)/(?P<Attachment>NA.[^.]+.png)\" (?P<Extra>[^>]*)>`)
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
	wg *sync.WaitGroup,
	notebook_path_manager *paths.NotebookPathManager) error {

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
			err = db.GetSubject(config_obj,
				notebook_path_manager.Cell(metadata.CellId).Path(),
				metadata)
			if err != nil {
				return err
			}
			metadata.Data = ""
		}
	}

	serialized, err := yaml.Marshal(notebook)
	if err != nil {
		return err
	}

	file_store_factory := file_store.GetFileStore(config_obj)
	output_filename := notebook_path_manager.ZipExport()
	fd, err := file_store_factory.WriteFile(output_filename)
	if err != nil {
		return err
	}

	err = fd.Truncate()
	if err != nil {
		return err
	}

	// Create a new ZipContainer to write on. The container will close
	// the underlying writer.
	zip_writer, err := NewContainerFromWriter(
		config_obj, fd, "", DEFAULT_COMPRESSION, NO_METADATA)
	if err != nil {
		return err
	}

	// zip_writer now owns fd and will close it when it closes below.

	// Report the progress as we write the container.
	progress_reporter := NewProgressReporter(config_obj,
		notebook_path_manager.PathStats(output_filename),
		output_filename, zip_writer)

	exported_path_manager := NewNotebookExportPathManager(notebook.NotebookId)

	cell_copier := func(cell_id string) {
		cell_path_manager := notebook_path_manager.Cell(cell_id)

		// Copy cell contents
		err = copyUploads(ctx, config_obj,
			cell_path_manager.Directory(),
			exported_path_manager.CellDirectory(cell_id),
			zip_writer, file_store_factory)
		if err != nil {
			logger := logging.GetLogger(config_obj, &logging.GUIComponent)
			logger.Info("ExportNotebookToZip Erorr: %v\n", err)
		}

		// Now copy the uploads
		err = copyUploads(ctx, config_obj,
			cell_path_manager.UploadsDir(),
			exported_path_manager.CellUploadRoot(cell_id),
			zip_writer, file_store_factory)
		if err != nil {
			logger := logging.GetLogger(config_obj, &logging.GUIComponent)
			logger.Info("ExportNotebookToZip Erorr: %v\n", err)
		}
	}

	wg.Add(1)

	// Write the bulk of the data asyncronously.
	go func() {
		defer wg.Done()
		defer progress_reporter.Close()

		// Will also close the underlying fd.
		defer zip_writer.Close()

		for _, cell := range notebook.CellMetadata {
			cell_copier(cell.CellId)
		}

		// Copy the attachments - Attachmentrs may not exist if there
		// are none in the notebook - so this is not an error.
		// Attachments are added to the notebook when the user pastes
		// them into it (e.g. an image)
		err = copyUploads(ctx, config_obj,
			notebook_path_manager.AttachmentDirectory(),
			exported_path_manager.UploadRoot(),
			zip_writer, file_store_factory)
		if err != nil {
			logger := logging.GetLogger(config_obj, &logging.GUIComponent)
			logger.Info("ExportNotebookToZip Erorr: %v\n", err)
		}

		f, err := zip_writer.Create("Notebook.yaml", time.Time{})
		if err != nil {
			return
		}
		defer f.Close()

		_, err = f.Write(serialized)
	}()

	return nil
}

func copyUploads(
	ctx context.Context,
	config_obj *config_proto.Config,
	src api.FSPathSpec,
	dest *accessors.OSPath,
	zip_writer *Container,
	file_store_factory api.FileStore) error {

	return api.Walk(file_store_factory, src,
		func(filename api.FSPathSpec, info os.FileInfo) error {
			src_depth := len(src.Components())
			if len(filename.Components()) <= src_depth {
				return nil
			}

			out_filename := dest.Append(filename.Components()[src_depth:]...)

			out_fd, err := zip_writer.Create(
				out_filename.String()+
					api.GetExtensionForFilestore(filename),
				time.Time{})
			if err != nil {
				return nil
			}
			defer out_fd.Close()

			fd, err := file_store_factory.ReadFile(filename)
			if err != nil {
				return nil
			}
			defer fd.Close()

			_, err = utils.Copy(ctx, out_fd, fd)
			return err
		})
}

func ExportNotebookToHTML(
	ctx context.Context,
	config_obj *config_proto.Config,
	notebook_id string, output io.Writer) error {

	db, err := datastore.GetDB(config_obj)
	if err != nil {
		return err
	}

	notebook_path_manager := paths.NewNotebookPathManager(notebook_id)
	notebook := &api_proto.NotebookMetadata{}
	err = db.GetSubject(config_obj, notebook_path_manager.Path(),
		notebook)
	if err != nil {
		return err
	}

	_, err = output.Write([]byte(fmt.Sprintf(HtmlPreable, notebook.Name)))
	if err != nil {
		return err
	}

	// Write the postscript when we are done.
	defer func() {
		_, _ = output.Write([]byte(HtmlPostscript))
	}()

	cell := &api_proto.NotebookCell{}
	for _, cell_md := range notebook.CellMetadata {
		err = db.GetSubject(config_obj,
			notebook_path_manager.Cell(cell_md.CellId).Path(),
			cell)
		if err != nil {
			return err
		}

		_, err = output.Write([]byte("<div class=\"notebook-cell\">\n"))
		if err != nil {
			return err
		}

		cell_output := imageRegex.ReplaceAllStringFunc(
			cell.Output, func(in string) string {
				file_store_factory := file_store.GetFileStore(config_obj)

				submatches := imageRegex.FindStringSubmatch(in)
				if len(submatches) < 3 {
					return in
				}
				extra := ""
				if len(submatches) > 3 {
					extra = submatches[3]
				}

				item_path := notebook_path_manager.Cell("").Item(submatches[2])
				fd, err := file_store_factory.ReadFile(item_path)
				if err != nil {
					return in
				}

				data, err := ioutil.ReadAll(fd)
				if err != nil {
					return in
				}

				return fmt.Sprintf(`<img src="data:image/jpg;base64,%v" %s>`,
					base64.StdEncoding.EncodeToString(data), extra)
			})

		// Expand tables
		new_cell_output := csvViewerRegexp.ReplaceAllStringFunc(
			cell_output, func(in string) string {
				result, err := convertCSVTags(ctx, config_obj, in, cell)
				if err != nil {
					return fmt.Sprintf(
						"<error>%s</error>",
						html.EscapeString(err.Error()))
				}
				return result
			})

		_, err := output.Write([]byte(new_cell_output))
		if err != nil {
			return err
		}

		_, err = output.Write([]byte("</div>\n"))
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

	unescaped, err := url.QueryUnescape(m[1])
	if err != nil {
		return "", errors.New("Unexpected regexp match")
	}

	params := &api_proto.GetTableRequest{}
	err = json.Unmarshal([]byte(unescaped), params)
	if err != nil {
		return "", err
	}

	path_manager := paths.NewNotebookPathManager(params.NotebookId).Cell(
		params.CellId).QueryStorage(params.TableId)
	file_store_factory := file_store.GetFileStore(config_obj)
	reader, err := result_sets.NewResultSetReader(
		file_store_factory, path_manager.Path())
	if err != nil {
		return "", err
	}
	defer reader.Close()

	headers := false
	for row := range reader.Rows(ctx) {
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
