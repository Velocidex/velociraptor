package notebooks

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"html"
	"net/url"
	"os"
	"regexp"
	"sync"
	"time"

	"github.com/Velocidex/ordereddict"
	"github.com/Velocidex/yaml/v2"
	"github.com/go-errors/errors"
	"www.velocidex.com/golang/velociraptor/accessors"
	"www.velocidex.com/golang/velociraptor/acls"
	actions_proto "www.velocidex.com/golang/velociraptor/actions/proto"
	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/constants"
	"www.velocidex.com/golang/velociraptor/file_store"
	"www.velocidex.com/golang/velociraptor/file_store/api"
	"www.velocidex.com/golang/velociraptor/file_store/path_specs"
	"www.velocidex.com/golang/velociraptor/json"
	"www.velocidex.com/golang/velociraptor/logging"
	"www.velocidex.com/golang/velociraptor/paths"
	"www.velocidex.com/golang/velociraptor/reporting"
	"www.velocidex.com/golang/velociraptor/result_sets"
	"www.velocidex.com/golang/velociraptor/services"
	"www.velocidex.com/golang/velociraptor/utils"
	"www.velocidex.com/golang/velociraptor/vql"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/vfilter"
	"www.velocidex.com/golang/vfilter/arg_parser"
)

var ()

type ExportNotebookArg struct {
	NotebookId string `vfilter:"required,field=notebook_id,doc=The id of the notebook to export"`
	Filename   string `vfilter:"optional,field=filename,doc=The name of the export. If not set this will be named according to the notebook id and timestamp"`
	Type       string `vfilter:"optional,field=type,doc=Set the type of the export (html or zip)."`
}

type ExportNotebookFunction struct{}

func (self ExportNotebookFunction) Call(ctx context.Context,
	scope vfilter.Scope,
	args *ordereddict.Dict) vfilter.Any {

	arg := &ExportNotebookArg{}
	err := arg_parser.ExtractArgsWithContext(ctx, scope, args, arg)
	if err != nil {
		scope.Log("notebook_export: %v", err)
		return vfilter.Null{}
	}

	err = vql_subsystem.CheckAccess(scope, acls.PREPARE_RESULTS)
	if err != nil {
		scope.Log("notebook_export: %v", err)
		return vfilter.Null{}
	}

	err = services.RequireFrontend()
	if err != nil {
		scope.Log("notebook_export: %v", err)
		return vfilter.Null{}
	}

	config_obj, ok := vql_subsystem.GetServerConfig(scope)
	if !ok {
		scope.Log("notebook_export: Command can only run on the server")
		return vfilter.Null{}
	}

	wg := &sync.WaitGroup{}
	defer func() {
		// Wait here until the export is done.
		wg.Wait()
	}()

	principal := vql_subsystem.GetPrincipal(scope)

	switch arg.Type {
	case "", "zip":
		result, err := ExportNotebookToZip(ctx,
			config_obj, wg, arg.NotebookId,
			principal, arg.Filename)
		if err != nil {
			scope.Log("notebook_export: %v", err)
			return vfilter.Null{}
		}
		return result

	case "html":
		result, err := ExportNotebookToHTML(
			config_obj, wg, arg.NotebookId,
			principal, arg.Filename)
		if err != nil {
			scope.Log("notebook_export: %v", err)
			return vfilter.Null{}
		}
		return result

	default:
		scope.Log("notebook_export: unsupported export type %v", arg.Type)
		return vfilter.Null{}
	}
}

func (self ExportNotebookFunction) Info(scope vfilter.Scope, type_map *vfilter.TypeMap) *vfilter.FunctionInfo {
	return &vfilter.FunctionInfo{
		Name:     "notebook_export",
		Doc:      "Exports a notebook to a zip file or HTML.",
		ArgType:  type_map.AddType(scope, &ExportNotebookArg{}),
		Metadata: vql.VQLMetadata().Permissions(acls.PREPARE_RESULTS).Build(),
	}
}

func init() {
	vql_subsystem.RegisterFunction(&ExportNotebookFunction{})
}

var (
	// Must match the output emitted by GuiTemplateEngine.Table
	csvViewerRegexp = regexp.MustCompile(
		`<velo-csv-viewer base-url="'v1/GetTable'" params='([^']+)' />`)

	imageRegex = regexp.MustCompile(
		`<img src=".+?/(?P<NotebookId>N\.[^/]+)/attach/(?P<Attachment>NA\.[^\?"]+)(?P<Extra>[^>]*)>`)

	hrefRegex = regexp.MustCompile(
		`<a href=".+?/(?P<NotebookId>N\.[^/]+)/attach/(?P<Attachment>NA\.[^\?"]+)(?P<Extra>[^>]*)>(?P<Filename>[^<]+)</a>`)
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

<script>

function base64ToArrayBuffer(_base64Str) {
      var binaryString = window.atob(_base64Str);
      var binaryLen = binaryString.length;
      var bytes = new Uint8Array(binaryLen);
      for (var i = 0; i < binaryLen; i++) {
            var ascii = binaryString.charCodeAt(i);
            bytes[i] = ascii;
     }
     return bytes;
}

function downloadFile(_base64Str, filename) {
      var byte = base64ToArrayBuffer(_base64Str);
      var a = document.createElement("a");
      document.body.appendChild(a);
      var blob = new Blob([byte], { type: "binary/octet-stream" });
      var url = window.URL.createObjectURL(blob);
        a.href = url;
        a.download = filename;
        a.click();
}
</script>


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
	notebook_id, principal, preferred_name string) (api.FSPathSpec, error) {

	notebook_manager, err := services.GetNotebookManager(config_obj)
	if err != nil {
		return nil, err
	}

	notebook, err := notebook_manager.GetNotebook(ctx, notebook_id,
		services.DO_NOT_INCLUDE_UPLOADS)
	if err != nil {
		return nil, err
	}

	if !notebook_manager.CheckNotebookAccess(notebook, principal) {
		return nil, fmt.Errorf("%w: Notebook is not shared with user.",
			utils.InvalidStatus)
	}

	notebook_path_manager := paths.NewNotebookPathManager(
		notebook_id)
	output_filename := notebook_path_manager.ZipExport()
	if preferred_name != "" {
		output_filename = output_filename.Dir().AddUnsafeChild(preferred_name)
	}

	// Replace the cell metadata with the full cell definition for
	// export.
	for idx, metadata := range notebook.CellMetadata {
		if metadata.CellId == "" {
			continue
		}
		cell, err := notebook_manager.GetNotebookCell(ctx,
			notebook_id, metadata.CellId,
			metadata.CurrentVersion)
		if err != nil {
			continue
		}
		notebook.CellMetadata[idx] = cell
	}

	serialized, err := yaml.Marshal(notebook)
	if err != nil {
		return nil, err
	}

	file_store_factory := file_store.GetFileStore(config_obj)
	fd, err := file_store_factory.WriteFileWithCompletion(
		output_filename, utils.SyncCompleter)
	if err != nil {
		return nil, err
	}

	err = fd.Truncate()
	if err != nil {
		return nil, err
	}

	// Create a new ZipContainer to write on. The container will close
	// the underlying writer.
	zip_writer, err := reporting.NewContainerFromWriter(
		output_filename.String(),
		config_obj, fd, "", reporting.DEFAULT_COMPRESSION, reporting.NO_METADATA)
	if err != nil {
		return nil, err
	}

	// zip_writer now owns fd and will close it when it closes below.

	exported_path_manager := reporting.NewNotebookExportPathManager(notebook.NotebookId)

	cell_copier := func(cell_id, version string) {
		cell_path_manager := notebook_path_manager.Cell(cell_id, version)

		// Copy cell contents
		err := copyUploads(ctx, config_obj,
			cell_path_manager.Directory(),
			exported_path_manager.CellDirectory(cell_id),
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

		timeout := int64(600)
		if config_obj.Defaults != nil &&
			config_obj.Defaults.ExportMaxTimeoutSec > 0 {
			timeout = config_obj.Defaults.ExportMaxTimeoutSec
		}

		ctx, cancel := context.WithTimeout(context.Background(),
			time.Second*time.Duration(timeout))
		defer cancel()

		opts := services.ContainerOptions{
			Type:              services.NotebookExport,
			NotebookId:        notebook_id,
			StatsPath:         notebook_path_manager.PathStats(output_filename),
			ContainerFilename: output_filename,
		}

		// Report the progress as we write the container.
		progress_reporter := reporting.NewProgressReporter(ctx, config_obj,
			output_filename, opts, zip_writer)
		defer progress_reporter.Close()

		// Will also close the underlying fd.
		defer zip_writer.Close()

		for _, cell := range notebook.CellMetadata {
			cell_copier(cell.CellId, cell.CurrentVersion)
		}

		// Copy the attachments - Attachments may not exist if there
		// are none in the notebook - so this is not an error.
		// Attachments are added to the notebook when the user pastes
		// them into it (e.g. an image)
		err := copyUploads(ctx, config_obj,
			notebook_path_manager.AttachmentDirectory(),
			exported_path_manager.AttachmentRoot(),
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

		_, _ = f.Write(serialized)
	}()

	err = services.LogAudit(ctx,
		config_obj, principal, "ExportNotebook",
		ordereddict.NewDict().
			Set("notebook_id", notebook_id).
			Set("output_filename", output_filename))

	if err != nil {
		logger := logging.GetLogger(config_obj, &logging.FrontendComponent)
		logger.Error("<red>ExportNotebook</> %v %v", principal, notebook_id)
	}

	return output_filename, nil
}

func copyUploads(
	ctx context.Context,
	config_obj *config_proto.Config,
	src api.FSPathSpec,
	dest *accessors.OSPath,
	zip_writer *reporting.Container,
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
	config_obj *config_proto.Config,
	wg *sync.WaitGroup,
	notebook_id, principal, preferred_name string) (api.FSPathSpec, error) {

	// Allow an hour to actually do the export. Our caller can wait on
	// the wg or allow operation in background.
	ctx, cancel := context.WithTimeout(context.Background(), time.Hour)

	notebook_path_manager := paths.NewNotebookPathManager(notebook_id)
	output_filename := notebook_path_manager.HtmlExport(preferred_name)

	exporter_func := func() (api.FSPathSpec, error) {
		defer wg.Done()
		defer cancel()

		file_store_factory := file_store.GetFileStore(config_obj)

		output, err := file_store_factory.WriteFileWithCompletion(
			output_filename, utils.SyncCompleter)
		if err != nil {
			return nil, err
		}
		defer output.Close()

		err = output.Truncate()
		if err != nil {
			return nil, err
		}

		sha_sum := sha256.New()
		tee_writer := utils.NewTee(output, sha_sum)

		notebook_manager, err := services.GetNotebookManager(config_obj)
		if err != nil {
			return nil, err
		}

		notebook, err := notebook_manager.GetNotebook(
			ctx, notebook_id, services.INCLUDE_UPLOADS)
		if err != nil {
			return nil, err
		}

		_, err = tee_writer.Write([]byte(fmt.Sprintf(HtmlPreable, notebook.Name)))
		if err != nil {
			return nil, err
		}

		stats := &api_proto.ContainerStats{
			Timestamp:  uint64(time.Now().Unix()),
			Type:       "html",
			Components: path_specs.AsGenericComponentList(output_filename),
		}

		export_manager, err := services.GetExportManager(config_obj)
		if err != nil {
			return nil, err
		}

		opts := services.ContainerOptions{
			Type:              services.NotebookExport,
			NotebookId:        notebook_id,
			StatsPath:         notebook_path_manager.PathStats(output_filename),
			ContainerFilename: output_filename,
		}

		err = export_manager.SetContainerStats(ctx, config_obj, stats, opts)
		if err != nil {
			return nil, err
		}

		// Write the postscript and stats when we are done.
		defer func() {
			_, _ = tee_writer.Write([]byte(HtmlPostscript))

			stats.TotalUncompressedBytes = uint64(tee_writer.Count())
			stats.TotalCompressedBytes = uint64(tee_writer.Count())
			stats.TotalContainerFiles = 1
			stats.Hash = hex.EncodeToString(sha_sum.Sum(nil))
			stats.TotalDuration = uint64(time.Now().Unix()) - stats.Timestamp

			_ = export_manager.SetContainerStats(ctx, config_obj, stats, opts)
		}()

		for _, cell_md := range notebook.CellMetadata {
			cell, err := notebook_manager.GetNotebookCell(ctx,
				notebook_id, cell_md.CellId, cell_md.CurrentVersion)
			if err != nil {
				return nil, err
			}

			_, err = tee_writer.Write([]byte("<div class=\"notebook-cell\">\n"))
			if err != nil {
				return nil, err
			}

			get_file_base64 := func(notebook_id, attachment string) (string, error) {
				notebook_path_manager := paths.NewNotebookPathManager(notebook_id)
				item_path := notebook_path_manager.AttachmentDirectory().
					AddChild(attachment)
				fd, err := file_store_factory.ReadFile(item_path)
				if err != nil {
					return "", err
				}

				data, err := utils.ReadAllWithLimit(fd, constants.MAX_MEMORY)
				if err != nil {
					return "", err
				}

				return base64.StdEncoding.EncodeToString(data), nil
			}

			// Fix up img tags.
			cell_output := imageRegex.ReplaceAllStringFunc(
				cell.Output, func(in string) string {

					submatches := imageRegex.FindStringSubmatch(in)
					if len(submatches) != 4 {
						return in
					}
					data, err := get_file_base64(submatches[1], submatches[2])
					if err != nil {
						return in
					}
					return fmt.Sprintf(`<img src="data:image/png;base64,%s">`,
						data)
				})

			// Fix up <a href tags.
			cell_output = hrefRegex.ReplaceAllStringFunc(
				cell_output, func(in string) string {

					submatches := hrefRegex.FindStringSubmatch(in)
					if len(submatches) != 5 {
						return in
					}

					notebook_id := submatches[1]
					attachment_id := submatches[2]
					filename := submatches[4]

					data, err := get_file_base64(notebook_id, attachment_id)
					if err != nil {
						return in
					}
					return fmt.Sprintf(`<a href="#" onclick="downloadFile('%s', '%s')">%s</a>`,
						data, filename, filename)
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

			// Now create links for uploads
			if notebook.AvailableUploads != nil {
				new_cell_output += "\n<h5>Uploads</h5>\n<ul>\n"
				for _, upload := range notebook.AvailableUploads.Files {
					if upload.Stats == nil {
						continue
					}

					item_path := path_specs.NewSafeFilestorePath().
						AddUnsafeChild(upload.Stats.Components...).
						SetType(api.PATH_TYPE_FILESTORE_ANY)

					fd, err := file_store_factory.ReadFile(item_path)
					if err != nil {
						continue
					}

					data, err := utils.ReadAllWithLimit(fd,
						constants.MAX_MEMORY)
					if err != nil {
						continue
					}

					filename := upload.Name
					new_cell_output += fmt.Sprintf(
						`<li><a href="#" onclick="downloadFile('%s', '%s')">%s</a></li>`,
						base64.StdEncoding.EncodeToString(data), filename, filename)
				}
				new_cell_output += "</ul>\n"
			}

			_, err = tee_writer.Write([]byte(new_cell_output))
			if err != nil {
				return nil, err
			}

			_, err = tee_writer.Write([]byte("</div>\n"))
			if err != nil {
				return nil, err
			}
		}

		return output_filename, nil
	}

	wg.Add(1)
	go func() {
		_, err := exporter_func()
		if err != nil {
			logger := logging.GetLogger(config_obj, &logging.GUIComponent)
			logger.Error("<red>ExportNotebookToHTML</>: %v", err)
		}
	}()

	err := services.LogAudit(ctx,
		config_obj, principal, "ExportNotebook",
		ordereddict.NewDict().
			Set("notebook_id", notebook_id).
			Set("output_filename", output_filename))
	if err != nil {
		logger := logging.GetLogger(config_obj, &logging.FrontendComponent)
		logger.Error("<red>ExportNotebook</> %v %v", principal, notebook_id)
	}

	return output_filename, nil
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
		params.CellId, params.CellVersion).QueryStorage(params.TableId)
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
		for _, v := range row.Values() {
			output.WriteString(
				fmt.Sprintf("    <td>%s</td>\n",
					html.EscapeString(fmt.Sprintf("%v", v))))
		}
		output.WriteString("  </tr>\n")
	}

	output.WriteString(" </tbody>\n")
	output.WriteString("</table>\n")

	return output.String(), nil
}
