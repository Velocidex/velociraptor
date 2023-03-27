//go:build linux

package efi

import (
	"fmt"
	"io/ioutil"
	"strings"
)

const efiPath = "/sys/firmware/efi/efivars/"

func GetEfiVariables() ([]EfiVariable, error) {
	var result []EfiVariable
	files, err := ioutil.ReadDir(efiPath)
	if err != nil {
		return nil, err
	}

	for _, file := range files {
		filename := file.Name()
		parts := strings.SplitN(filename, "-", 2)
		if len(parts) != 2 {
			continue
		}
		result = append(result, EfiVariable{
			Namespace: fmt.Sprintf("{%s}", parts[1]),
			Name:      parts[0],
			Value:     nil,
		})
	}
	return result, nil
}

func GetEfiVariableValue(namespace string, name string) ([]byte, error) {
	namespace = strings.Trim(namespace, "{}")

	return ioutil.ReadFile(fmt.Sprintf("%s%s-%s", efiPath, name, namespace))
}
