package main

import (
	"os"

	errors "github.com/go-errors/errors"
	"www.velocidex.com/golang/velociraptor/constants"
	"www.velocidex.com/golang/velociraptor/json"
	"www.velocidex.com/golang/velociraptor/utils"
	"www.velocidex.com/golang/velociraptor/vql/tools"
	"www.velocidex.com/golang/vfilter"
	"www.velocidex.com/golang/vfilter/marshal"
	"www.velocidex.com/golang/vfilter/types"
)

var (
	ignoreVars = []string{
		"config",
	}
)

func loadScopeFromFile(filename string, scope types.Scope) (types.Scope, error) {
	fd, err := os.Open(filename)
	if err != nil {
		// File is missing is not an error - just store it in
		// the file at the end.
		return scope, nil
	}
	defer fd.Close()

	data, err := utils.ReadAllWithLimit(fd, constants.MAX_MEMORY)
	if err != nil {
		return nil, err
	}

	// Build an unmarshaller
	unmarshaller := vfilter.NewUnmarshaller(ignoreVars)
	unmarshaller.RegisterHandler("StarlModule", tools.StarlModule{})

	unmarshal_item := &types.MarshalItem{}
	err = json.Unmarshal(data, &unmarshal_item)
	if err != nil {
		return nil, err
	}

	res, err := unmarshaller.Unmarshal(unmarshaller,
		scope, unmarshal_item)
	if err != nil {
		return nil, err
	}

	res_scope, ok := res.(types.Scope)
	if !ok {
		return nil, errors.New("Scope file does not contain a serialized scope")
	}

	return res_scope, err
}

func storeScopeInFile(filename string, scope types.Scope) error {
	intermediate, err := marshal.Marshal(scope, scope)
	if err != nil {
		return err
	}

	serialized, err := json.MarshalIndent(intermediate)
	if err != nil {
		return err
	}

	fd, err := os.OpenFile(filename, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0600)
	if err != nil {
		return err
	}
	defer fd.Close()

	_, err = fd.Write(serialized)

	return err
}
