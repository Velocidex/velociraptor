package survey

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/Velocidex/yaml/v2"
	"github.com/charmbracelet/huh"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
)

var (
	fileDoesNotExist = errors.New("New File will be created")
)

func StoreServerConfig(config_obj *config_proto.Config) error {
	server_path, err := filepath.Abs("./server.config.yaml")
	if err != nil {
		return err
	}

	for {

		form := huh.NewForm(
			huh.NewGroup(
				huh.NewNote().
					Title("Let's store the server configuration file.").
					Description(`
You will need this file to build the server deb package using:

velociraptor --config server.config.yaml debian server

You can derive the client configuration file:

velociraptor --config server.config.yaml config client > client.config.yaml

`),
				huh.NewInput().
					Title("Name of file to write").
					DescriptionFunc(func() string {
						err := checkFile(server_path)
						if err == fileDoesNotExist {
							return "New File will be created"
						}

						if err == nil {
							return "File will be overwritten"
						}

						return err.Error()
					}, &server_path).
					Validate(func(in string) error {
						err := checkFile(server_path)
						if err == nil || err == fileDoesNotExist {
							return nil
						}

						return err
					}).
					Value(&server_path),
			),
		).WithTheme(getTheme())

		err = form.Run()
		if err != nil {
			return err
		}

		res, err := yaml.Marshal(config_obj)
		if err != nil {
			return fmt.Errorf("Yaml Marshal: %w", err)
		}

		fd, err := os.OpenFile(server_path,
			os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0600)
		if err != nil {
			err = fmt.Errorf("Open file %s: %w", server_path, err)
			if showError(
				"Unable to create file",
				"Retry to create the file again with a different name, or abort", err) {
				continue
			}
			return err
		}
		_, err = fd.Write(res)
		if err != nil {
			err = fmt.Errorf("Write file %s: %w", server_path, err)
			if showError(
				"Unable to create file",
				"Retry to create the file again with a different name, or abort", err) {
				continue
			}
			return err
		}
		return fd.Close()
	}
}

func showError(title, message string, err error) bool {
	retry := false

	form := huh.NewForm(
		huh.NewGroup(
			huh.NewNote().
				Title(title).
				Description(fmt.Sprintf("Error: %v\n\n%v", err, message)),
			huh.NewConfirm().
				Title("Try again?").
				Value(&retry),
		),
	).WithTheme(getTheme())

	err = form.Run()
	if err != nil {
		return false
	}

	return retry
}

func checkFile(write_path string) error {
	dirname := filepath.Dir(write_path)
	st, err := os.Lstat(dirname)
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("Directory %v does not exist", dirname)
		}
		if os.IsPermission(err) {
			return fmt.Errorf("Directory %v is not accessibile: %w",
				dirname, err)
		}

		return fmt.Errorf("Directory %v is not valid", dirname)
	}

	if !st.IsDir() {
		return fmt.Errorf("Directory %v is not a valid directory", dirname)
	}

	st, err = os.Lstat(write_path)
	if err != nil {
		if os.IsNotExist(err) {
			return fileDoesNotExist
		}

		if os.IsPermission(err) {
			return fmt.Errorf("Unable to create file: %w", err)
		}
	}

	if st.IsDir() {
		return fmt.Errorf("Path %v is a directory", write_path)
	}

	return nil
}
