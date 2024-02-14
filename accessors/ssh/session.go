package ssh

import (
	"context"
	"errors"
	"fmt"

	"github.com/Velocidex/ordereddict"
	"golang.org/x/crypto/ssh"
	"www.velocidex.com/golang/velociraptor/constants"
	"www.velocidex.com/golang/velociraptor/services"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/vfilter"
)

func GetSSHClient(scope vfilter.Scope) (
	client *ssh.Client, closer func() error, err error) {
	setting, pres := scope.Resolve(constants.SSH_CONFIG)
	if !pres {
		return nil, nil, errors.New("Configure the 'ssh' accessor using 'LET SSH_CONFIG <= dict(...)'")
	}

	// Check for a secret from the secrets service
	secret := vql_subsystem.GetStringFromRow(scope, setting, "secret")
	if secret != "" {
		setting, err = getSecret(scope, secret)
		if err != nil {
			return nil, nil, err
		}
	}

	config := &ssh.ClientConfig{
		User:            vql_subsystem.GetStringFromRow(scope, setting, "username"),
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
	}

	password := vql_subsystem.GetStringFromRow(scope, setting, "password")
	if password != "" {
		config.Auth = append(config.Auth, ssh.Password(password))
	}

	private_key := vql_subsystem.GetStringFromRow(scope, setting, "private_key")
	if private_key != "" {
		// Attempt to parse it
		signer, err := ssh.ParsePrivateKey([]byte(private_key))
		if err != nil {
			return nil, nil, fmt.Errorf("ssh: While parsing private key: %w", err)
		}

		config.Auth = append(config.Auth, ssh.PublicKeys(signer))
	}

	hostname := vql_subsystem.GetStringFromRow(scope, setting, "hostname")
	if hostname == "" {
		return nil, nil, errors.New("ssh: No hostname specified in SSH_CONFIG")
	}

	client, err = ssh.Dial("tcp", hostname, config)
	if err != nil {
		return nil, nil, err
	}

	scope.Log("INFO:ssh: Initiated connection to host %v", hostname)

	return client, client.Close, nil
}

func getSecret(scope vfilter.Scope, secret string) (
	*ordereddict.Dict, error) {
	config_obj, ok := vql_subsystem.GetServerConfig(scope)
	if !ok {
		return nil, errors.New("Secrets may only be used on the server")
	}

	secrets_service, err := services.GetSecretsService(config_obj)
	if err != nil {
		return nil, err
	}

	principal := vql_subsystem.GetPrincipal(scope)

	// Extract the context from the scope.
	ctx := context.TODO()

	secret_record, err := secrets_service.GetSecret(ctx, principal,
		constants.SSH_PRIVATE_KEY, secret)
	if err != nil {
		return nil, err
	}

	return secret_record.Data, nil
}
