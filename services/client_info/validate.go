package client_info

import (
	"errors"
	"regexp"
)

var (
	// Client IDs always start with "C." or they can refer to the "server"
	client_id_regex              = regexp.MustCompile("^(C\\.[a-z0-9]+|server)")
	client_id_not_provided_error = errors.New("ClientId not provided")
	client_id_not_valid_error    = errors.New("ClientId is not valid")
)

func (self *ClientInfoManager) ValidateClientId(client_id string) error {
	if client_id == "" {
		return client_id_not_provided_error
	}
	if !client_id_regex.MatchString(client_id) {
		return client_id_not_valid_error
	}
	return nil
}
