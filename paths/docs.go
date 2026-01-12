package paths

import "www.velocidex.com/golang/velociraptor/file_store/api"

type DocsIndexPathManager struct{}

func (self DocsIndexPathManager) Metadata() api.FSPathSpec {
	return CONFIG_ROOT.AsFilestorePath().
		AddChild("docs", "index_meta.json").
		SetType(api.PATH_TYPE_FILESTORE_ANY)
}

func (self DocsIndexPathManager) Index() api.FSPathSpec {
	return CONFIG_ROOT.AsFilestorePath().
		AddChild("docs").
		SetType(api.PATH_TYPE_FILESTORE_ANY)
}

func NewDocsIndexPathManager() *DocsIndexPathManager {
	return &DocsIndexPathManager{}
}
