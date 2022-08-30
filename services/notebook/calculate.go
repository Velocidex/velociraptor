package notebook

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"runtime/debug"
	"strings"
	"time"

	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	artifacts_proto "www.velocidex.com/golang/velociraptor/artifacts/proto"
	"www.velocidex.com/golang/velociraptor/logging"
	"www.velocidex.com/golang/velociraptor/paths"
	"www.velocidex.com/golang/velociraptor/reporting"
	"www.velocidex.com/golang/velociraptor/services"
	"www.velocidex.com/golang/velociraptor/vql/acl_managers"
)

func (self *NotebookManager) UpdateNotebookCell(
	ctx context.Context,
	notebook_metadata *api_proto.NotebookMetadata,
	user_name string,
	in *api_proto.NotebookCellRequest) (*api_proto.NotebookCell, error) {

	notebook_cell := &api_proto.NotebookCell{
		Input:            in.Input,
		Output:           `<div class="padded"><i class="fa fa-spinner fa-spin fa-fw"></i> Calculating...</div>`,
		CellId:           in.CellId,
		Type:             in.Type,
		Timestamp:        time.Now().Unix(),
		CurrentlyEditing: in.CurrentlyEditing,
		Calculating:      true,
		Env:              in.Env,
	}

	notebook_path_manager := paths.NewNotebookPathManager(
		notebook_metadata.NotebookId)

	err := self.Store.SetNotebook(notebook_metadata)
	if err != nil {
		return nil, err
	}

	// Run the actual query independently.
	query_ctx, query_cancel := context.WithCancel(context.Background())

	acl_manager := acl_managers.NewServerACLManager(self.config_obj, user_name)

	manager, err := services.GetRepositoryManager(self.config_obj)
	if err != nil {
		return nil, err
	}
	global_repo, err := manager.GetGlobalRepository(self.config_obj)
	if err != nil {
		return nil, err
	}

	tmpl, err := reporting.NewGuiTemplateEngine(
		self.config_obj, query_ctx, nil, acl_manager, global_repo,
		notebook_path_manager.Cell(in.CellId),
		"Server.Internal.ArtifactDescription")
	if err != nil {
		return nil, err
	}

	tmpl.SetEnv("NotebookId", in.NotebookId)

	// Register a progress reporter so we can monitor how the
	// template rendering is going.
	tmpl.Progress = &progressReporter{
		config_obj:    self.config_obj,
		notebook_cell: notebook_cell,
		notebook_id:   in.NotebookId,
		start:         time.Now(),
		store:         self.Store,
	}

	// Add the notebook environment into the cell template.
	for _, env := range notebook_metadata.Env {
		tmpl.SetEnv(env.Key, env.Value)
	}

	// Also apply the cell env
	for _, env := range in.Env {
		tmpl.SetEnv(env.Key, env.Value)
	}

	input := in.Input
	cell_type := in.Type

	// Update the content asynchronously
	start_time := time.Now()

	// RPC call deadline - if we can complete within 1 second pass
	// the response directly to the RPC caller.
	sub_ctx, sub_cancel := context.WithTimeout(ctx, time.Second)

	// Main error will be delivered to the RPC caller if we can
	// complete the entire operation before the deadline.
	var main_err error

	// Watcher thread: Wait for cancellation from the GUI or a 10 min timeout.
	go func() {
		defer query_cancel()

		notifier, err := services.GetNotifier(self.config_obj)
		if err != nil {
			return
		}
		cancel_notify, remove_notification := notifier.
			ListenForNotification(in.CellId)
		defer remove_notification()

		default_notebook_expiry := self.config_obj.Defaults.NotebookCellTimeoutMin
		if default_notebook_expiry == 0 {
			default_notebook_expiry = 10
		}

		select {
		// Query is done - get out of here.
		case <-query_ctx.Done():

		// Active cancellation from the GUI.
		case <-cancel_notify:
			tmpl.Scope.Log("Cancelled after %v !", time.Since(start_time))

			// Set a timeout.
		case <-time.After(time.Duration(default_notebook_expiry) * time.Minute):
			tmpl.Scope.Log("Query timed out after %v !", time.Since(start_time))
		}

	}()

	// Main worker: Just run the query until done.
	go func() {
		// Cancel and release the main thread if we
		// finish quickly before the timeout.
		defer sub_cancel()

		// Make sure to cancel the query context if we
		// finished early - the Waiter goroutine above will be
		// released.
		defer query_cancel()

		// Close the template when we are done with it.
		defer tmpl.Close()

		resp, err := self.updateCellContents(query_ctx, tmpl,
			in.CurrentlyEditing, in.NotebookId,
			in.CellId, cell_type, in.Env, input, in.Input)
		if err != nil {
			main_err = err
			logger := logging.GetLogger(self.config_obj, &logging.GUIComponent)
			logger.Error("Rendering error: %v", err)
		}

		// Update the response if we can.
		if resp != nil {
			notebook_cell = resp
		}
	}()

	// Wait here up to 1 second for immediate response - but if
	// the response takes too long, just give up and return a
	// continuation. The GUI will continue polling for notebook
	// state and will pick up the changes by itself.
	<-sub_ctx.Done()

	return notebook_cell, main_err
}

func (self *NotebookManager) updateCellContents(
	ctx context.Context,
	tmpl *reporting.GuiTemplateEngine,
	currently_editing bool,
	notebook_id, cell_id, cell_type string,
	env []*api_proto.Env,
	input, original_input string) (res *api_proto.NotebookCell, err error) {

	output := ""

	cell_type = strings.ToLower(cell_type)

	// Create a new cell to set the result in.
	make_cell := func(output string) *api_proto.NotebookCell {
		messages := tmpl.Messages()

		encoded_data, err := json.Marshal(tmpl.Data)
		if err != nil {
			messages = append(messages,
				fmt.Sprintf("Error: %v", err))
		}

		return &api_proto.NotebookCell{
			Input:            original_input,
			Output:           output,
			Data:             string(encoded_data),
			Messages:         tmpl.Messages(),
			CellId:           cell_id,
			Type:             cell_type,
			Env:              env,
			Timestamp:        time.Now().Unix(),
			CurrentlyEditing: currently_editing,
			Duration:         int64(time.Since(tmpl.Start).Seconds()),
		}
	}

	// If an error occurs it is important to ensure the cell is
	// still written with an error message.
	make_error_cell := func(output string, err error) (
		*api_proto.NotebookCell, error) {
		notebook_cell := make_cell(output)
		notebook_cell.Messages = append(notebook_cell.Messages,
			fmt.Sprintf("Error: %v", err))
		self.Store.SetNotebookCell(notebook_id, notebook_cell)
		return notebook_cell, err
	}

	// Do not let exceptions take down the server.
	defer func() {
		r := recover()
		if r != nil {
			res, err = make_error_cell("", fmt.Errorf(
				"PANIC: %v: %v", r, string(debug.Stack())))
		}
	}()

	switch cell_type {

	case "vql_suggestion":
		// noop - these cells will be created by the user on demand.

	case "markdown", "md":
		// A Markdown cell just feeds directly into the
		// template.
		output, err = tmpl.Execute(&artifacts_proto.Report{Template: input})
		if err != nil {
			return make_error_cell(output, err)
		}

	case "vql":
		// A VQL cell gets converted to a set of VQL and
		// markdown fragments.
		cell_content, err := reporting.ConvertVQLCellToContent(input)
		if err != nil {
			// Ignore errors and just treat the whole
			// thing as VQL - this will fail to render the
			// comment and just ignore it - it is probably
			// malformed.
			cell_content = &reporting.Content{}
			cell_content.PushVQL(input)
		}

		for _, fragment := range cell_content.Fragments {
			if fragment.VQL != "" {
				rows := tmpl.Query(fragment.VQL)
				output_any, ok := tmpl.Table(rows).(string)
				if ok {
					output += output_any
				}

			} else if fragment.Comment != "" {
				lines := strings.SplitN(fragment.Comment, "\n", 2)
				if len(lines) <= 1 {
					input = lines[0]
				} else {
					input = lines[1]
				}
				fragment_output, err := tmpl.Execute(&artifacts_proto.Report{Template: input})
				if err != nil {
					return make_error_cell(output, err)
				}
				output += fragment_output
			}
		}

	default:
		return make_error_cell(output, errors.New("Unsupported cell type."))
	}

	tmpl.Close()

	notebook_cell := make_cell(output)
	return notebook_cell, self.Store.SetNotebookCell(notebook_id, notebook_cell)
}
