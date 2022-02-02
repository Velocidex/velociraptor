package plugins

import (
	_ "www.velocidex.com/golang/velociraptor/accessors"
	_ "www.velocidex.com/golang/velociraptor/accessors/data"
	_ "www.velocidex.com/golang/velociraptor/accessors/file"
	_ "www.velocidex.com/golang/velociraptor/accessors/file_store"
	_ "www.velocidex.com/golang/velociraptor/accessors/ntfs"
	_ "www.velocidex.com/golang/velociraptor/accessors/pipe"
	_ "www.velocidex.com/golang/velociraptor/accessors/process"
	_ "www.velocidex.com/golang/velociraptor/accessors/raw_registry"
	_ "www.velocidex.com/golang/velociraptor/accessors/sparse"
	_ "www.velocidex.com/golang/velociraptor/accessors/zip"
)
