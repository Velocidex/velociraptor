package simple

import (
	"context"
	"time"

	"github.com/Velocidex/ordereddict"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/file_store/api"
	"www.velocidex.com/golang/velociraptor/result_sets"
	"www.velocidex.com/golang/velociraptor/utils"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/velociraptor/vql/sorter"
	"www.velocidex.com/golang/vfilter"
)

func (self ResultSetFactory) NewResultSetReaderWithOptions(
	ctx context.Context,
	config_obj *config_proto.Config,
	file_store_factory api.FileStore,
	log_path api.FSPathSpec,
	options result_sets.ResultSetOptions) (result_sets.ResultSetReader, error) {

	// First do the filtering and then do the sorting.
	return self.getFilteredReader(ctx, config_obj, file_store_factory,
		log_path, options)
}

func (self ResultSetFactory) getFilteredReader(
	ctx context.Context,
	config_obj *config_proto.Config,
	file_store_factory api.FileStore,
	log_path api.FSPathSpec,
	options result_sets.ResultSetOptions) (result_sets.ResultSetReader, error) {

	// No filter required.
	if options.FilterColumn == "" || options.FilterRegex == nil {
		return self.getSortedReader(ctx, config_obj, file_store_factory,
			log_path, options)
	}

	transformed_path := log_path.AddUnsafeChild(
		"filter", options.FilterRegex.String())

	// Try to open the transformed result set if it is already cached.
	base_stat, err := file_store_factory.StatFile(log_path)
	if err != nil {
		return self.NewResultSetReader(file_store_factory, log_path)
	}

	cached_stat, err := file_store_factory.StatFile(transformed_path)
	if err == nil && cached_stat.ModTime().After(base_stat.ModTime()) {
		return self.getSortedReader(ctx, config_obj,
			file_store_factory, transformed_path, options)
	}

	// Nope - we have to build the new cache from the original table.
	reader, err := self.NewResultSetReader(file_store_factory, log_path)
	if err != nil {
		return nil, err
	}
	defer reader.Close()

	// Create the new writer
	writer, err := self.NewResultSetWriter(
		file_store_factory, transformed_path, nil, utils.SyncCompleter,
		result_sets.TruncateMode)
	if err != nil {
		return nil, err
	}

	// Default is 10 min to filter the file.
	default_notebook_expiry := int64(10)
	if config_obj.Defaults != nil &&
		config_obj.Defaults.NotebookCellTimeoutMin > 0 {
		default_notebook_expiry = config_obj.Defaults.NotebookCellTimeoutMin
	}

	sub_ctx, sub_cancel := context.WithTimeout(ctx,
		time.Duration(default_notebook_expiry)*time.Minute)
	defer sub_cancel()

	// Filter the table with the regex
	row_chan := reader.Rows(ctx)
outer:
	for {
		select {
		case <-sub_ctx.Done():
			break outer

		case row, ok := <-row_chan:
			if !ok {
				break outer
			}
			value, pres := row.Get(options.FilterColumn)
			if pres {
				value_str := utils.ToString(value)
				if options.FilterRegex.FindStringIndex(value_str) != nil {
					writer.Write(row)
				}
			}
		}
	}

	// Flush all the writes back
	writer.Close()

	return self.getSortedReader(ctx, config_obj, file_store_factory,
		transformed_path, options)
}

func (self ResultSetFactory) getSortedReader(
	ctx context.Context,
	config_obj *config_proto.Config,
	file_store_factory api.FileStore,
	log_path api.FSPathSpec,
	options result_sets.ResultSetOptions) (result_sets.ResultSetReader, error) {

	// No sorting required.
	if options.SortColumn == "" {
		return self.NewResultSetReader(file_store_factory, log_path)
	}

	var transformed_path api.FSPathSpec
	if options.SortAsc {
		transformed_path = log_path.AddUnsafeChild(
			"sorted", options.SortColumn, "asc")
	} else {
		transformed_path = log_path.AddUnsafeChild(
			"sorted", options.SortColumn, "desc")
	}

	// Try to open the transformed result set if it is already cached.
	base_stat, err := file_store_factory.StatFile(log_path)
	if err != nil {
		return self.NewResultSetReader(file_store_factory, log_path)
	}

	// Only use the cache if it is newer than the base file.
	cached_stat, err := file_store_factory.StatFile(transformed_path)
	if err == nil && cached_stat.ModTime().After(base_stat.ModTime()) {
		return self.NewResultSetReader(file_store_factory, transformed_path)
	}

	// Nope - we have to build the new cache from the original table.
	scope := vql_subsystem.MakeScope()
	reader, err := self.NewResultSetReader(file_store_factory, log_path)
	if err != nil {
		return nil, err
	}
	defer reader.Close()

	// Create the new writer
	writer, err := self.NewResultSetWriter(
		file_store_factory, transformed_path, nil, utils.SyncCompleter,
		result_sets.TruncateMode)
	if err != nil {
		return nil, err
	}

	sorter_input_chan := make(chan vfilter.Row)
	sorted_chan := sorter.MergeSorter{10000}.Sort(
		ctx, scope, sorter_input_chan,
		options.SortColumn, options.SortAsc)

	// Default is 10 min to sort the file.
	default_notebook_expiry := int64(10)
	if config_obj.Defaults != nil &&
		config_obj.Defaults.NotebookCellTimeoutMin > 0 {
		default_notebook_expiry = config_obj.Defaults.NotebookCellTimeoutMin
	}

	sub_ctx, sub_cancel := context.WithTimeout(ctx,
		time.Duration(default_notebook_expiry)*time.Minute)
	defer sub_cancel()

	// Now write into the sorter and read the sorted results.
	go func() {
		defer close(sorter_input_chan)

		row_chan := reader.Rows(ctx)

		for {
			select {
			case <-sub_ctx.Done():
				return

			case row, ok := <-row_chan:
				if !ok {
					return
				}
				sorter_input_chan <- row
			}
		}
	}()

	for row := range sorted_chan {
		row_dict, ok := row.(*ordereddict.Dict)
		if ok {
			writer.Write(row_dict)
		}
	}

	// Close synchronously to flush the data
	writer.Close()

	return self.NewResultSetReader(file_store_factory, transformed_path)
}
