package plugins

import (
	_ "www.velocidex.com/golang/velociraptor/accessors"
	_ "www.velocidex.com/golang/velociraptor/accessors/collector"
	_ "www.velocidex.com/golang/velociraptor/accessors/data"
	_ "www.velocidex.com/golang/velociraptor/accessors/file"
	_ "www.velocidex.com/golang/velociraptor/accessors/file_store"
	_ "www.velocidex.com/golang/velociraptor/accessors/ntfs"
	_ "www.velocidex.com/golang/velociraptor/accessors/offset"
	_ "www.velocidex.com/golang/velociraptor/accessors/pipe"
	_ "www.velocidex.com/golang/velociraptor/accessors/process"
	_ "www.velocidex.com/golang/velociraptor/accessors/raw_file"
	_ "www.velocidex.com/golang/velociraptor/accessors/raw_registry"
	_ "www.velocidex.com/golang/velociraptor/accessors/registry"
	_ "www.velocidex.com/golang/velociraptor/accessors/s3"
	_ "www.velocidex.com/golang/velociraptor/accessors/smb"
	_ "www.velocidex.com/golang/velociraptor/accessors/sparse"
	_ "www.velocidex.com/golang/velociraptor/accessors/zip"
)
