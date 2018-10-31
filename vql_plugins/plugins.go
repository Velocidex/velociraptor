package plugins

// This is a do nothing package which just forces an import of the
// various VQL plugin directories. The plugins will register
// themselves.

import (
	_ "www.velocidex.com/golang/velociraptor/vql/common"
	_ "www.velocidex.com/golang/velociraptor/vql/filesystem"
	_ "www.velocidex.com/golang/velociraptor/vql/functions"
	_ "www.velocidex.com/golang/velociraptor/vql/networking"
	_ "www.velocidex.com/golang/velociraptor/vql/parsers"
	_ "www.velocidex.com/golang/velociraptor/vql/server"
)
