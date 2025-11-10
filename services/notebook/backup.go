package notebook

import (
	"context"
	"fmt"
	"strings"
	"sync"

	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	artifacts_proto "www.velocidex.com/golang/velociraptor/artifacts/proto"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/json"
	"www.velocidex.com/golang/velociraptor/paths"
	"www.velocidex.com/golang/velociraptor/reporting"
	"www.velocidex.com/golang/velociraptor/services"
	"www.velocidex.com/golang/velociraptor/vql/acl_managers"
	"www.velocidex.com/golang/vfilter"
)

type BackupRecord struct {
	NotebookMetadata     *api_proto.NotebookMetadata `json:"notebook"`
	NotebookCellMetadata *api_proto.NotebookCell     `json:"cell"`
}

type NotebookBackupProvider struct {
	config_obj       *config_proto.Config
	notebook_manager *NotebookManager
}

func (self NotebookBackupProvider) ProviderName() string {
	return "NotebookBackupProvider"
}

func (self NotebookBackupProvider) Name() []string {
	return []string{"notebooks.json"}
}

// The backup will just dump out the contents of the hunt dispatcher.
func (self NotebookBackupProvider) BackupResults(
	ctx context.Context, wg *sync.WaitGroup,
	container services.BackupContainerWriter) (<-chan vfilter.Row, error) {

	output_chan := make(chan vfilter.Row)

	notebooks, err := self.notebook_manager.GetAllNotebooks(
		ctx, services.NotebookSearchOptions{})
	if err != nil {
		return nil, err
	}

	wg.Add(1)
	go func() {
		defer wg.Done()
		defer close(output_chan)

		for _, notebook := range notebooks {
			output_chan <- &BackupRecord{
				NotebookMetadata: notebook,
			}

			for _, cell_md := range notebook.CellMetadata {
				cell, err := self.notebook_manager.GetNotebookCell(
					ctx, notebook.NotebookId, cell_md.CellId,
					cell_md.CurrentVersion)
				if err == nil {
					cell.NotebookId = notebook.NotebookId
					cell.AvailableVersions = []string{cell.CurrentVersion}
					output_chan <- &BackupRecord{
						NotebookCellMetadata: cell,
					}
				}
			}
		}
	}()

	return output_chan, nil
}

func (self NotebookBackupProvider) renderCell(
	ctx context.Context,
	cell *api_proto.NotebookCell) error {

	if strings.ToLower(cell.Type) != "vql" {
		return nil
	}

	acl_manager := acl_managers.NullACLManager{}
	manager, err := services.GetRepositoryManager(self.config_obj)
	if err != nil {
		return err
	}
	repository, err := manager.GetGlobalRepository(self.config_obj)
	if err != nil {
		return err
	}

	notebook_path_manager := paths.NewNotebookPathManager(
		cell.NotebookId)
	tmpl, err := reporting.NewGuiTemplateEngine(
		self.config_obj, ctx, nil, acl_manager, repository,
		notebook_path_manager.Cell(cell.CellId, cell.CurrentVersion),
		"Server.Internal.ArtifactDescription")
	if err != nil {
		return err
	}
	defer tmpl.Close()

	data := "Recalculate this cell:\n" +
		"```vql\n" +
		cell.Input +
		"\n```\n"

	cell.Output, err = tmpl.Execute(&artifacts_proto.Report{
		Template: data,
	})
	if err != nil {
		cell.Output = data
	}

	return err
}

func (self NotebookBackupProvider) Restore(ctx context.Context,
	container services.BackupContainerReader,
	in <-chan vfilter.Row) (stat services.BackupStat, err error) {

	notebook_count := 0
	cell_count := 0

	defer func() {
		if stat.Error == nil {
			stat.Error = err
		}
		stat.Message += fmt.Sprintf("Restored %v notebooks and %v cells\n",
			notebook_count, cell_count)
	}()

	for {
		select {
		case <-ctx.Done():
			return stat, nil

		case row, ok := <-in:
			if !ok {
				return stat, nil
			}

			serialized, err := json.MarshalWithOptions(
				row, json.DefaultEncOpts())
			if err != nil {
				continue
			}

			record := &BackupRecord{}
			err = json.Unmarshal(serialized, record)
			if err != nil {
				continue
			}

			if record.NotebookMetadata != nil {
				notebook := record.NotebookMetadata
				err := self.notebook_manager.Store.SetNotebook(notebook)
				if err != nil {
					stat.Warnings = append(stat.Warnings,
						fmt.Sprintf("NewNotebook %s: %v",
							notebook.NotebookId, err))
					continue
				}
				notebook_count++
				continue
			}

			if record.NotebookCellMetadata != nil {
				cell := record.NotebookCellMetadata

				err = self.renderCell(ctx, cell)
				if err != nil {
					stat.Warnings = append(stat.Warnings,
						fmt.Sprintf("NewNotebookCell %s: %v",
							cell.CellId, err))
					continue
				}

				err = self.notebook_manager.Store.SetNotebookCell(
					cell.NotebookId, cell)
				if err != nil {
					stat.Warnings = append(stat.Warnings,
						fmt.Sprintf("NewNotebookCell %s: %v",
							cell.CellId, err))
					continue
				}
				cell_count++
				continue
			}
		}
	}
}
