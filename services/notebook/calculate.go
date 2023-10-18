package notebook

import (
	"context"
	"errors"
	"fmt"
	"runtime"
	"runtime/debug"
	"strings"
	"time"

	"www.velocidex.com/golang/vfilter/types"

	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	artifacts_proto "www.velocidex.com/golang/velociraptor/artifacts/proto"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/json"
	"www.velocidex.com/golang/velociraptor/reporting"
	"www.velocidex.com/golang/velociraptor/services"
	"www.velocidex.com/golang/velociraptor/utils"
	"www.velocidex.com/golang/vfilter"
)

func (self *NotebookManager) UpdateNotebookCell(
	ctx context.Context,
	notebook_metadata *api_proto.NotebookMetadata,
	user_name string,
	in *api_proto.NotebookCellRequest) (*api_proto.NotebookCell, error) {
	request := &NotebookRequest{
		NotebookMetadata:    notebook_metadata,
		Username:            user_name,
		NotebookCellRequest: in,
	}

	scheduler, err := services.GetSchedulerService(self.config_obj)
	if err != nil {
		return nil, err
	}

	response_chan, err := scheduler.Schedule(ctx, services.SchedulerJob{
		Queue: "Notebook",
		Job:   json.MustMarshalString(request),
	})
	if err != nil {
		return nil, err
	}

	select {
	case <-ctx.Done():
		return nil, errors.New("Cancelled")

	case job_resp, ok := <-response_chan:
		if !ok {
			return nil, errors.New("Cancelled")
		}
		notebook_resp := &NotebookResponse{}
		err := json.Unmarshal([]byte(job_resp.Job), notebook_resp)
		if err != nil {
			return nil, err
		}

		return notebook_resp.NotebookCell, job_resp.Err
	}
}

func (self *NotebookManager) startNanny(
	ctx context.Context, config_obj *config_proto.Config,
	scope vfilter.Scope,
	notebook_id, cell_id string) {

	// Reduce memory use now so the next measure of memory use is more
	// reflective of our current workload.
	debug.FreeOSMemory()

	// Running in a goroutine it's ok to block.
	for {

		// Check for high memory use.
		if self.config_obj.Defaults != nil &&
			self.config_obj.Defaults.NotebookMemoryHighWaterMark > 0 {

			high_memory_level := self.config_obj.Defaults.NotebookMemoryHighWaterMark

			var m runtime.MemStats
			runtime.ReadMemStats(&m)

			// If we exceed memory we cancel this query.
			if high_memory_level < m.Alloc {
				scope.Log("ERROR:Insufficient resourcs: Query cancelled.")
				self.CancelNotebookCell(ctx, notebook_id, cell_id)
			}
		}

		select {
		case <-ctx.Done():
			return
		case <-time.After(time.Second):
		}

		// Check the cell for cancellation or errors
		notebook_cell, err := self.Store.GetNotebookCell(notebook_id, cell_id)
		if err != nil || notebook_cell.CellId != cell_id {
			return
		}

		// Cancel the query - this cell is not longer running
		if !notebook_cell.Calculating {
			// Notify the calculator immediately
			notifier, err := services.GetNotifier(self.config_obj)
			if err != nil {
				return
			}

			notifier.NotifyListener(ctx, self.config_obj, cell_id, "CancelNotebookCell")
		}
	}
}

func (self *NotebookManager) waitForMemoryLimit(
	ctx context.Context, scope types.Scope,
	config_obj *config_proto.Config) {
	// Wait until memory is below the low water mark.
	if self.config_obj.Defaults != nil &&
		self.config_obj.Defaults.NotebookMemoryLowWaterMark > 0 {

		low_memory_level := self.config_obj.Defaults.NotebookMemoryLowWaterMark

		for {
			var m runtime.MemStats
			runtime.ReadMemStats(&m)

			// Spin here until there is enough memory
			if low_memory_level > m.Alloc {
				break
			}

			select {
			case <-ctx.Done():
				scope.Log("ERROR:Unable to start query before timeout - insufficient resourcs.")
				return
			case <-time.After(time.Second):
			}
		}
	}
}

func (self *NotebookManager) updateCellContents(
	ctx context.Context,
	tmpl *reporting.GuiTemplateEngine,
	currently_editing bool,
	notebook_id, cell_id, cell_type string,
	env []*api_proto.Env,
	input, original_input string) (res *api_proto.NotebookCell, err error) {

	// Start a nanny to watch this calculation
	go self.startNanny(ctx, self.config_obj, tmpl.Scope,
		notebook_id, cell_id)

	output := ""
	now := utils.GetTime().Now().Unix()

	cell_type = strings.ToLower(cell_type)

	// Create a new cell to set the result in.
	make_cell := func(output string) *api_proto.NotebookCell {
		encoded_data, err := json.Marshal(tmpl.Data)
		if err != nil {
			tmpl.Scope.Log("Error: %v", err)
		}

		return &api_proto.NotebookCell{
			Input:            original_input,
			Output:           output,
			Data:             string(encoded_data),
			Messages:         tmpl.Messages(),
			MoreMessages:     tmpl.MoreMessages(),
			CellId:           cell_id,
			Type:             cell_type,
			Env:              env,
			Timestamp:        now,
			CurrentlyEditing: currently_editing,
			Duration:         int64(time.Since(tmpl.Start).Seconds()),
		}
	}

	// If an error occurs it is important to ensure the cell is
	// still written with an error message.
	make_error_cell := func(
		output string, err error) (*api_proto.NotebookCell, error) {
		tmpl.Scope.Log("ERROR: %v", err)
		error_cell := make_cell(output)
		error_cell.Calculating = false
		error_cell.Error = err.Error()

		self.Store.SetNotebookCell(notebook_id, error_cell)

		return error_cell, utils.InlineError
	}

	// Do not let exceptions take down the server.
	defer func() {
		r := recover()
		if r != nil {
			res, err = make_error_cell("", fmt.Errorf(
				"PANIC: %v: %v", r, string(debug.Stack())))
		}
	}()

	// Write a place holder immediately while we calculate the rest.
	notebook_cell := make_cell(output)
	notebook_cell.Calculating = true
	err = self.Store.SetNotebookCell(notebook_id, notebook_cell)
	if err != nil {
		return nil, err
	}
	self.waitForMemoryLimit(ctx, tmpl.Scope, self.config_obj)

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
		// No query, nothing to do
		if reporting.IsEmptyQuery(input) {
			tmpl.Error("Please specify a query to run")
		} else {
			vqls, err := vfilter.MultiParseWithComments(input)
			if err != nil {
				// Try parsing without comments if comment parser fails
				vqls, err = vfilter.MultiParse(input)
				if err != nil {
					return make_error_cell(output, err)
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
							return make_error_cell(output, err)
						}
						output += fragment_output
					}
				}
				if vql.Let != "" || vql.Query != nil || vql.StoredQuery != nil {
					no_query = false
					rows, err := tmpl.RunQuery(vql, nil)

					if err != nil {
						return make_error_cell(output, err)
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

	default:
		return make_error_cell(output, errors.New("Unsupported cell type."))
	}

	tmpl.Close()

	notebook_cell = make_cell(output)
	return notebook_cell, self.Store.SetNotebookCell(notebook_id, notebook_cell)
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
