package utils

import (
	"errors"

	"www.velocidex.com/golang/velociraptor/constants"
	"www.velocidex.com/golang/velociraptor/services"
	"www.velocidex.com/golang/vfilter"
)

func GetRepository(scope vfilter.Scope) (services.Repository, error) {
	any_obj, pres := scope.Resolve(constants.SCOPE_REPOSITORY)
	if !pres {
		return nil, errors.New("Repository not found in scope!!")
	}
	repository, ok := any_obj.(services.Repository)
	if !ok {
		return nil, errors.New("Repository not found in scope!!")
	}
	return repository, nil
}
