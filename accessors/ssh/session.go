package ssh

import (
	"errors"
	"fmt"

	"golang.org/x/crypto/ssh"
	"www.velocidex.com/golang/velociraptor/constants"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/vfilter"
)

func GetSSHClient(scope vfilter.Scope) (*ssh.Client, func() error, error) {
	// Empty credentials are OK - they just mean to get creds from the
	// process env
	setting, pres := scope.Resolve(constants.SSH_CONFIG)
	if !pres {
		return nil, nil, errors.New("Configure the 'ssh' accessor using 'LET SSH_CONFIG <= dict(...)'")
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

	client, err := ssh.Dial("tcp", hostname, config)
	if err != nil {
		return nil, nil, err
	}

	scope.Log("INFO:ssh: Initiated connection to host %v", hostname)

	return client, client.Close, nil
}
