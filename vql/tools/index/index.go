package index

import (
	"context"
	"fmt"
	"os"

	"github.com/Velocidex/ordereddict"
	"github.com/Velocidex/velociraptor-site-search/api"
	"github.com/alitto/pond/v2"
	"github.com/blevesearch/bleve/v2"
	"github.com/blevesearch/bleve/v2/analysis/analyzer/standard"
	"github.com/blevesearch/bleve/v2/mapping"
	"www.velocidex.com/golang/velociraptor/accessors/file"
	"www.velocidex.com/golang/velociraptor/acls"
	"www.velocidex.com/golang/velociraptor/utils"
	"www.velocidex.com/golang/velociraptor/vql"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/vfilter"
	"www.velocidex.com/golang/vfilter/arg_parser"
)

const PAGESIZE = 100

func getFieldMapping(name string) (*mapping.FieldMapping, error) {
	switch name {
	case "", "text", "en-text":
		res := mapping.NewTextFieldMapping()
		res.Analyzer = standard.Name
		return res, nil

	case "number":
		return mapping.NewNumericFieldMapping(), nil

	case "date", "datetime":
		return mapping.NewDateTimeFieldMapping(), nil

	case "bool":
		return mapping.NewBooleanFieldMapping(), nil

	case "ip":
		return mapping.NewIPFieldMapping(), nil

	}

	return nil, fmt.Errorf("Unknown field type %v", name)
}

type IndexPluginArgs struct {
	Query           vfilter.StoredQuery `vfilter:"required,field=query,doc=A VQL Query to parse and execute."`
	Mapping         *ordereddict.Dict   `vfilter:"optional,field=mapping,doc=A dict to describe field mapping."`
	DefaultAnalyzer string              `vfilter:"optional,field=default_analyzer,doc=The default analyzer to use."`
	Output          string              `vfilter:"required,field=output,doc=The file path to create the index on."`
	Workers         int64               `vfilter:"optional,field=workers,doc=Index with this many workers (default 2)"`
	Purge           bool                `vfilter:"optional,field=purge,doc=If set we delete the index to start fresh"`
	BatchSize       int64               `vfilter:"optional,field=batch,doc=Default batch size for index (default 1000)"`
	Silent          bool                `vfilter:"optional,field=silent,doc=Do not forward events (this is faster)"`
}

type IndexPlugin struct{}

func (self IndexPlugin) Call(
	ctx context.Context,
	scope vfilter.Scope,
	args *ordereddict.Dict) <-chan vfilter.Row {
	output_chan := make(chan vfilter.Row)

	go func() {
		defer close(output_chan)
		defer vql_subsystem.RegisterMonitor(ctx, "index", args)()

		// This plugin just passes the current scope to the subquery
		// so there is no permissions check - the subquery will
		// receive the same privileges as the calling query.
		arg := &IndexPluginArgs{}
		err := arg_parser.ExtractArgsWithContext(ctx, scope, args, arg)
		if err != nil {
			scope.Log("index: %v", err)
			return
		}

		err = vql_subsystem.CheckAccess(scope, acls.FILESYSTEM_WRITE)
		if err != nil {
			scope.Log("index: %s", err)
			return
		}

		// Make sure we are allowed to write there.
		err = file.CheckPath(arg.Output)
		if err != nil {
			scope.Log("index: %v", err)
			return
		}

		if arg.BatchSize == 0 {
			arg.BatchSize = 1000
		}

		if arg.Purge {
			err := api.PurgeCache()
			if err != nil {
				scope.Log("index: %v", err)
				return
			}

			stat, err := os.Lstat(arg.Output)
			if err == nil && stat.IsDir() {
				err := os.RemoveAll(arg.Output)
				if err != nil {
					scope.Log("index: %v", err)
					return
				}
			}
		}

		if arg.DefaultAnalyzer == "" {
			arg.DefaultAnalyzer = "standard"
		}

		doc_mapping := bleve.NewDocumentMapping()
		if arg.Mapping != nil {
			for _, item := range arg.Mapping.Items() {
				mapping_name, ok := item.Value.(string)
				if !ok {
					scope.Log("index: field mapping name %v should be of type string not %T",
						item.Key, item.Value)
					return
				}

				if mapping_name == "none" {
					doc_mapping.AddSubDocumentMapping(item.Key,
						bleve.NewDocumentDisabledMapping())
					continue
				}

				field_mapping, err := getFieldMapping(mapping_name)
				if err != nil {
					scope.Log("index: %v", err)
					return
				}
				doc_mapping.AddFieldMappingsAt(item.Key, field_mapping)
			}
		}

		idx_mapping := bleve.NewIndexMapping()
		idx_mapping.DefaultMapping = doc_mapping
		idx_mapping.DefaultAnalyzer = arg.DefaultAnalyzer

		index, err := api.NewIndex(arg.Output, idx_mapping)
		if err != nil {
			scope.Log("index: %v", err)
			return
		}

		defer index.Close()

		workers := 2
		if arg.Workers > 0 {
			workers = int(arg.Workers)
		}

		pool := pond.NewPool(workers)

		// Wait here for all the workers on exit.
		defer pool.StopAndWait()

		row_chan := arg.Query.Eval(ctx, scope)
		batch_rows := make([]vfilter.Row, 0, arg.BatchSize+1)

		flush := func(batch_rows []vfilter.Row) {
			batch := index.NewBatch()
			for _, row := range batch_rows {
				row_dict := vfilter.RowToDict(ctx, scope, row)

				doc_id, pres := row_dict.GetString("_id")
				if !pres {
					doc_id = fmt.Sprintf("%v", utils.GetId())
				}

				m := make(map[string]string)
				for _, item := range row_dict.Items() {
					m[item.Key] = utils.ToString(item.Value)
				}

				err := batch.Index(doc_id, m)
				if err != nil {
					scope.Log("index: %v", err)
				}

				if !arg.Silent {
					select {
					case <-ctx.Done():
						return

					case output_chan <- row_dict:
					}
				}
			}

			err := index.Batch(batch)
			if err != nil {
				scope.Log("index: %v", err)
			}
		}

		// Flush any outstanding rows on exit
		defer flush(batch_rows)

		for {
			select {
			case <-ctx.Done():
				return
			case row, ok := <-row_chan:
				if !ok {
					return
				}

				batch_rows = append(batch_rows, row)
				if len(batch_rows) > int(arg.BatchSize) {
					flush(batch_rows)
					batch_rows = batch_rows[:0]
				}
			}
		}

	}()

	return output_chan

}

func (self IndexPlugin) Info(scope vfilter.Scope, type_map *vfilter.TypeMap) *vfilter.PluginInfo {
	return &vfilter.PluginInfo{
		Name:     "index",
		Doc:      "Create a local index from a query.",
		ArgType:  type_map.AddType(scope, &IndexPluginArgs{}),
		Metadata: vql.VQLMetadata().Permissions(acls.FILESYSTEM_WRITE).Build(),
		Version:  2,
	}
}

type IndexSearchPluginArgs struct {
	IndexPath   string   `vfilter:"required,field=path,doc=The file path to the index to open."`
	SearchQuery string   `vfilter:"required,field=search,doc=A Bleve search query. See https://blevesearch.com/docs/Query-String-Query/"`
	Fields      []string `vfilter:"optional,field=fields,doc=A list of fields to include from the index."`
	Sort        []string `vfilter:"optional,field=sort,doc=The field to sort by (preceed with - to sort in descending order)."`
	Start       uint64   `vfilter:"optional,field=start,doc=Row number to start."`
}

type IndexSearchPlugin struct{}

func (self IndexSearchPlugin) Call(
	ctx context.Context,
	scope vfilter.Scope,
	args *ordereddict.Dict) <-chan vfilter.Row {
	output_chan := make(chan vfilter.Row)

	go func() {
		defer close(output_chan)
		defer vql_subsystem.RegisterMonitor(ctx, "index_search", args)()

		// This plugin just passes the current scope to the subquery
		// so there is no permissions check - the subquery will
		// receive the same privileges as the calling query.
		arg := &IndexSearchPluginArgs{}
		err := arg_parser.ExtractArgsWithContext(ctx, scope, args, arg)
		if err != nil {
			scope.Log("index_search: %v", err)
			return
		}

		err = vql_subsystem.CheckAccess(scope, acls.FILESYSTEM_READ)
		if err != nil {
			scope.Log("index: %s", err)
			return
		}

		// Make sure we are allowed to write there.
		err = file.CheckPath(arg.IndexPath)
		if err != nil {
			scope.Log("index: %v", err)
			return
		}

		index, err := api.OpenIndex(arg.IndexPath)
		if err != nil {
			scope.Log("index_search: %v", err)
			return
		}
		defer index.Close()

		query := bleve.NewQueryStringQuery(arg.SearchQuery)
		searchRequest := bleve.NewSearchRequest(query)

		// If no fields specified, get them all
		searchRequest.Fields = arg.Fields
		if len(arg.Fields) == 0 {
			searchRequest.Fields, _ = index.Fields()
		}

		if len(arg.Sort) > 0 {
			searchRequest.SortBy(arg.Sort)
		}
		start := int(arg.Start)

		for {
			searchRequest.From = start
			searchRequest.Size = start + PAGESIZE

			result, err := index.Search(searchRequest)
			if err != nil {
				scope.Log("index_search: %v", err)
				return
			}

			for _, hit := range result.Hits {

				// Preverse the order of the fields since Bleve
				// returns an unordered map.
				row := ordereddict.NewDict()
				for _, f := range searchRequest.Fields {
					v, pres := hit.Fields[f]
					if pres {
						row.Set(f, v)
					}
				}

				select {
				case <-ctx.Done():
					return
				case output_chan <- row:
				}
			}

			if len(result.Hits) < PAGESIZE {
				return
			}

			start += PAGESIZE

		}

	}()

	return output_chan

}

func (self IndexSearchPlugin) Info(scope vfilter.Scope, type_map *vfilter.TypeMap) *vfilter.PluginInfo {
	return &vfilter.PluginInfo{
		Name:     "index_search",
		Doc:      "Search a previously created index.",
		ArgType:  type_map.AddType(scope, &IndexSearchPluginArgs{}),
		Metadata: vql.VQLMetadata().Permissions(acls.FILESYSTEM_READ).Build(),
		Version:  1,
	}
}

func init() {
	vql_subsystem.RegisterPlugin(&IndexPlugin{})
	vql_subsystem.RegisterPlugin(&IndexSearchPlugin{})
}
