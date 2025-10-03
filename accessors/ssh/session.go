package ssh

import (
	"context"
	"errors"
	"fmt"

	"golang.org/x/crypto/ssh"
	"www.velocidex.com/golang/velociraptor/constants"
	"www.velocidex.com/golang/velociraptor/services"
	"www.velocidex.com/golang/velociraptor/utils"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/vfilter"
	"www.velocidex.com/golang/vfilter/arg_parser"
	"www.velocidex.com/golang/vfilter/utils/dict"
)

type SSHAccessorArgs struct {
	Secret     string `vfilter:"optional,field=secret,doc=The name of a secret to use."`
	Username   string `vfilter:"optional,field=username,doc=The username to use to log into the remote system."`
	Password   string `vfilter:"optional,field=password,doc=The password to use to log into the remote system."`
	PrivateKey string `vfilter:"optional,field=private_key,doc=A private key to use to log into the remote system instead of a password."`
	Hostname   string `vfilter:"optional,field=hostname,doc=The hostname to log into."`
}

func GetSSHClient(scope vfilter.Scope) (
	client *ssh.Client, closer func() error, err error) {

	// TODO: Extract the context from the scope.
	ctx := context.TODO()

	setting, pres := scope.Resolve(constants.SSH_CONFIG)
	if !pres {
		return nil, nil, errors.New("Configure the 'ssh' accessor using 'LET SSH_CONFIG <= dict(...)'")
	}

	args := dict.RowToDict(ctx, scope, setting)
	arg := &SSHAccessorArgs{}
	err = arg_parser.ExtractArgsWithContext(ctx, scope, args, arg)
	if err != nil {
		return nil, nil, err
	}

	err = maybeForceSecrets(ctx, scope, arg)
	if err != nil {
		return nil, nil, err
	}

	if arg.Secret != "" {
		arg, err = getSecret(ctx, scope, arg.Secret)
		if err != nil {
			return nil, nil, err
		}
	}

	config := &ssh.ClientConfig{
		User:            arg.Username,
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
	}

	if arg.Password != "" {
		config.Auth = append(config.Auth, ssh.Password(arg.Password))
	}

	if arg.PrivateKey != "" {
		// Attempt to parse it
		signer, err := ssh.ParsePrivateKey([]byte(arg.PrivateKey))
		if err != nil {
			return nil, nil, fmt.Errorf("ssh: While parsing private key: %w", err)
		}

		config.Auth = append(config.Auth, ssh.PublicKeys(signer))
	}

	if arg.Hostname == "" {
		return nil, nil, errors.New("ssh: No hostname specified in SSH_CONFIG")
	}

	client, err = ssh.Dial("tcp", arg.Hostname, config)
	if err != nil {
		return nil, nil, err
	}

	scope.Log("INFO:ssh: Initiated connection to host %v", arg.Hostname)

	return client, client.Close, nil
}

func maybeForceSecrets(
	ctx context.Context, scope vfilter.Scope, arg *SSHAccessorArgs) error {

	// Not running on the server, secrets dont work.
	config_obj, ok := vql_subsystem.GetServerConfig(scope)
	if !ok {
		return nil
	}

	if config_obj.Security == nil {
		return nil
	}

	if !config_obj.Security.VqlMustUseSecrets {
		return nil
	}

	// If an explicit secret is defined let it filter the URLs.
	if arg.Secret != "" {
		return nil
	}

	return utils.SecretsEnforced
}

func getSecret(
	ctx context.Context,
	scope vfilter.Scope,
	secret string) (*SSHAccessorArgs, error) {
	config_obj, ok := vql_subsystem.GetServerConfig(scope)
	if !ok {
		return nil, errors.New("Secrets may only be used on the server")
	}

	secrets_service, err := services.GetSecretsService(config_obj)
	if err != nil {
		return nil, err
	}

	principal := vql_subsystem.GetPrincipal(scope)

	secret_record, err := secrets_service.GetSecret(ctx, principal,
		constants.SSH_PRIVATE_KEY, secret)
	if err != nil {
		return nil, err
	}

	// Override the following from the secret
	arg := &SSHAccessorArgs{
		Username:   secret_record.GetString("username"),
		Hostname:   secret_record.GetString("hostname"),
		Password:   secret_record.GetString("password"),
		PrivateKey: secret_record.GetString("private_key"),
	}
	return arg, nil
}
