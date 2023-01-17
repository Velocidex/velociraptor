package utils

import "regexp"

var (
	// Client IDs always start with "C." or they can refer to the "server"
	client_id_regex = regexp.MustCompile("^(C\\.[a-z0-9]+|server)")
)

func ValidateClientId(client_id string) bool {
	return client_id_regex.MatchString(client_id)
}
