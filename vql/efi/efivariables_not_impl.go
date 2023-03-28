// +build darwin freebsd

package efi

import "errors"

func GetEfiVariables() ([]EfiVariable, error) {
	return nil, errors.New("Not implemented")
}

func GetEfiVariableValue(namespace string, name string) ([]byte, error) {
	return nil, errors.New("Not implemented")
}
