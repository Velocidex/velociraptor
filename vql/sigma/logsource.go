package sigma

import (
	"context"
	"strings"
	"sync"

	"github.com/Velocidex/ordereddict"
	"github.com/Velocidex/sigma-go"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/vfilter"
	"www.velocidex.com/golang/vfilter/arg_parser"
	"www.velocidex.com/golang/vfilter/types"
)

type LogSourceProvider struct {
	mu sync.Mutex

	queries map[string]types.StoredQuery
}

func (self *LogSourceProvider) Queries() map[string]types.StoredQuery {
	self.mu.Lock()
	defer self.mu.Unlock()

	res := make(map[string]types.StoredQuery)
	for k, v := range self.queries {
		res[k] = v
	}

	return res
}

// Add an associative protocol to LogSourceProvider so we can iterate
// over it.
type LogSourceProviderAssociative struct{}

func (self LogSourceProviderAssociative) Applicable(a vfilter.Any, b vfilter.Any) bool {
	_, a_ok := a.(*LogSourceProvider)
	_, b_ok := b.(string)
	return a_ok && b_ok
}

func (self LogSourceProviderAssociative) GetMembers(
	scope vfilter.Scope, a vfilter.Any) []string {

	ls, ok := a.(*LogSourceProvider)
	if !ok {
		return nil
	}

	ls.mu.Lock()
	defer ls.mu.Unlock()

	res := make([]string, 0, len(ls.queries))
	for k := range ls.queries {
		res = append(res, k)
	}
	return res
}

func (self *LogSourceProviderAssociative) Associative(scope vfilter.Scope, a vfilter.Any, b vfilter.Any) (
	vfilter.Any, bool) {

	ls, ok := a.(*LogSourceProvider)
	if !ok {
		return vfilter.Null{}, false
	}

	key, ok := b.(string)
	if !ok {
		return vfilter.Null{}, false
	}

	ls.mu.Lock()
	defer ls.mu.Unlock()

	res, ok := ls.queries[key]
	if !ok {
		return vfilter.Null{}, false
	}

	return vfilter.FormatToString(scope, res), true
}

type LogSourcesFunction struct{}

func (self *LogSourcesFunction) Call(ctx context.Context,
	scope vfilter.Scope,
	args *ordereddict.Dict) vfilter.Any {

	result := &LogSourceProvider{
		queries: make(map[string]types.StoredQuery),
	}

	args = arg_parser.NormalizeArgs(args)

	for _, field := range scope.GetMembers(args) {
		value, _ := scope.Associative(args, field)

		query, ok := value.(vfilter.StoredQuery)
		if !ok {
			scope.Log("ERROR:sigma_log_sources: log provider for %v must be a query", field)
			return &vfilter.Null{}
		}

		result.queries[field] = query
	}

	if len(result.queries) == 0 {
		scope.Log("ERROR:sigma_log_sources: No log sources provided")
	}

	return result
}

func (self LogSourcesFunction) Info(scope vfilter.Scope, type_map *vfilter.TypeMap) *vfilter.FunctionInfo {
	return &vfilter.FunctionInfo{
		Name:         "sigma_log_sources",
		Doc:          "Constructs a Log sources object to be used in sigma rules. Call with args being category/product/service and values being stored queries. You may use a * as a placeholder for any of these fields.",
		FreeFormArgs: true,
		Version:      2,
	}
}

func parseLogSourceTarget(name string) *sigma.Logsource {
	parts := strings.Split(name, "/")
	for i, p := range parts {
		if p == "*" {
			parts[i] = ""
		}
	}

	if len(parts) == 1 {
		return &sigma.Logsource{
			Category: parts[0],
		}
	}

	if len(parts) == 2 {
		return &sigma.Logsource{
			Category: parts[0],
			Product:  parts[1],
		}
	}

	return &sigma.Logsource{
		Category: parts[0],
		Product:  parts[1],
		Service:  parts[2],
	}
}

func matchLogSource(log_target *sigma.Logsource, rule sigma.Rule) bool {
	if rule.Logsource.Service != "" && log_target.Service == "" {
		return false
	}

	if rule.Logsource.Service != "" && rule.Logsource.Service != log_target.Service {
		return false
	}

	if rule.Logsource.Product != "" && log_target.Product == "" {
		return false
	}

	if rule.Logsource.Product != "" && rule.Logsource.Product != log_target.Product {
		return false
	}

	if rule.Logsource.Category != "" && log_target.Category == "" {
		return false
	}

	if rule.Logsource.Category != "" && rule.Logsource.Category != log_target.Category {
		return false
	}

	return true
}

func init() {
	vql_subsystem.RegisterFunction(&LogSourcesFunction{})
	vql_subsystem.RegisterProtocol(&LogSourceProviderAssociative{})
}
