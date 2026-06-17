package test_utils

import (
	"archive/zip"
	"io"

	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/file_store"
	"www.velocidex.com/golang/velociraptor/file_store/api"
	"www.velocidex.com/golang/velociraptor/utils"
)

func UnzipToFilestore(
	config_obj *config_proto.Config,
	base api.FSPathSpec,
	zip_path string) error {

	reader, err := zip.OpenReader(zip_path)
	if err != nil {
		return err
	}
	defer reader.Close()

	file_store_factory := file_store.GetFileStore(config_obj)

	for _, f := range reader.File {
		components := utils.SplitComponents(f.Name)
		output_path := base.AddChild(components...).
			SetType(api.PATH_TYPE_FILESTORE_ANY)
		out_fd, err := file_store_factory.WriteFile(output_path)
		if err != nil {
			return err
		}
		in_fd, err := reader.Open(f.Name)
		if err != nil {
			out_fd.Close()
			continue
		}

		_, err = io.Copy(out_fd, in_fd)
		out_fd.Close()

		if err != nil {
			return err
		}
	}

	return nil
}
