package remote

import (
	"www.velocidex.com/golang/velociraptor/file_store/api"
	"www.velocidex.com/golang/velociraptor/utils"
)

func (self *RPCFileStore) Move(src, dest api.FSPathSpec) error {
	return utils.NotImplementedError
}

func (self *RPCFileStore) Close() error {
	return nil
}
