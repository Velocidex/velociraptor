package acls

import "www.velocidex.com/golang/velociraptor/utils"

var (
	PermissionDenied = utils.Wrap(utils.PermissionDenied, "PermissionDenied")
)
