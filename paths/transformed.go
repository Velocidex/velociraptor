package paths

import (
	"www.velocidex.com/golang/velociraptor/file_store/api"
	"www.velocidex.com/golang/velociraptor/result_sets"
)

// Where we should store the transformed result sets.
// Transformed result sets are relative to the base path.
func NewTransformedPathManager(
	base api.FSPathSpec, transform result_sets.ResultSetOptions) api.FSPathSpec {
	if transform.SortColumn != "" {
		if transform.SortAsc {
			return base.AddChild("sorted", transform.SortColumn, "asc")
		}
		return base.AddChild("sorted", transform.SortColumn, "desc")
	}
	return base
}
