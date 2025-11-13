package main

import (
	"fmt"
	"log"
	"os"
	"path/filepath"

	"github.com/Velocidex/ordereddict"
	"www.velocidex.com/golang/velociraptor/accessors"
	"www.velocidex.com/golang/velociraptor/accessors/collector"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/constants"
	logging "www.velocidex.com/golang/velociraptor/logging"
	"www.velocidex.com/golang/velociraptor/services"
	"www.velocidex.com/golang/velociraptor/startup"
	"www.velocidex.com/golang/velociraptor/vql/acl_managers"
)

var (
	collector_decrypt = app.Command(
		"decrypt", "Decrypts an armoured collection file")
	collector_decrypt_input = collector_decrypt.Arg(
		"collection", "A collection zip file to decrypt").
		Required().String()
	collector_decrypt_output = collector_decrypt.Arg(
		"output",
		"The filename to write the decrypted collection on").
		String()

	collector_decrypt_show_password = collector_decrypt.Flag(
		"show_password", "Show the password").
		Short('p').Bool()

	collector_decrypt_format = collector_decrypt.Flag(
		"format", "Output format for csv output").
		Default("json").Enum("text", "json", "csv", "jsonl")
)

func doCollectorDecrypt() error {
	logging.DisableLogging()

	server_config_obj, err := makeDefaultConfigLoader().
		WithNullLoader().LoadAndValidate()
	if err != nil {
		return fmt.Errorf("Unable to load config file: %w", err)
	}

	ctx, cancel := install_sig_handler()
	defer cancel()

	config_obj := &config_proto.Config{}
	config_obj.Frontend = server_config_obj.Frontend

	config_obj.Services = services.GenericToolServices()
	sm, err := startup.StartToolServices(ctx, config_obj)
	defer sm.Close()

	if err != nil {
		return err
	}

	filename, err := filepath.Abs(*collector_decrypt_input)
	if err != nil {
		return err
	}

	_, err = os.Stat(filename)
	if os.IsNotExist(err) {
		return err
	}

	env := ordereddict.NewDict()

	logger := &LogWriter{config_obj: sm.Config}
	builder := services.ScopeBuilder{
		Config:     sm.Config,
		ACLManager: acl_managers.NewRoleACLManager(sm.Config, "administrator"),
		Logger:     log.New(logger, "", 0),
		Env:        env,
	}

	manager, err := services.GetRepositoryManager(builder.Config)
	if err != nil {
		return err
	}

	scope := manager.BuildScope(builder)
	defer scope.Close()

	accessor, err := accessors.GetAccessor("zip", scope)
	if err != nil {
		return err
	}

	pathspec := accessors.MustNewZipFilePath("/")
	pathspec.SetPathSpec(
		&accessors.PathSpec{
			DelegatePath:     *collector_decrypt_input,
			DelegateAccessor: "file",
		})

	password, err := collector.ExtractPassword(scope, accessor, pathspec)
	if err != nil {
		return err
	}

	if *collector_decrypt_output != "" {
		env.Set(constants.ZIP_PASSWORDS, password).
			Set("PATHSPEC", pathspec).
			Set("ShowPassword", *collector_decrypt_show_password).
			Set("OUTPUT", *collector_decrypt_output)

		query := `
SELECT OSPath, Size, hash(path=OSPath) AS Hash,
     ShowPassword && ZIP_PASSWORDS AS Password
FROM stat(filename=copy(
   filename=PATHSPEC + "data.zip",
   accessor="zip", dest=OUTPUT))
`
		return runQueryWithEnv(query, builder, *collector_decrypt_format)
	}

	if *collector_decrypt_show_password {
		fmt.Printf("Password is: %v\n", password)
	}

	return nil
}
