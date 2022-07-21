package test_utils

import (
	"context"
	"time"

	"github.com/Velocidex/ordereddict"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/logging"
	"www.velocidex.com/golang/velociraptor/services"
	"www.velocidex.com/golang/velociraptor/vql/acl_managers"
	"www.velocidex.com/golang/vfilter"
)

// A convenience function for running a query and getting back a set
// of rows.
func RunQuery(
	config_obj *config_proto.Config,
	query string,
	env *ordereddict.Dict) ([]*ordereddict.Dict, error) {

	builder := services.ScopeBuilder{
		Config:     config_obj,
		ACLManager: acl_managers.NullACLManager{},
		Logger: logging.NewPlainLogger(
			config_obj, &logging.FrontendComponent),
		Env: env,
	}
	manager, err := services.GetRepositoryManager(config_obj)
	if err != nil {
		return nil, err
	}

	scope := manager.BuildScope(builder)
	defer scope.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	multi_vql, err := vfilter.MultiParse(query)
	if err != nil {
		return nil, err
	}

	rows := []*ordereddict.Dict{}
	for _, vql := range multi_vql {
		for row := range vql.Eval(ctx, scope) {
			rows = append(rows, vfilter.RowToDict(ctx, scope, row))
		}
	}

	return rows, nil
}
