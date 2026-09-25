package ssh

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"net"

	"golang.org/x/crypto/ssh"
	"www.velocidex.com/golang/velociraptor/acls"
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
	HostKey    string `vfilter:"optional,field=hostkey,doc=A base64 encoded host key. If specified we reject connections that do not present this host key."`
}

func GetSSHClient(scope vfilter.Scope) (
	client *ssh.Client, closer func() error, err error) {

	// TODO: Extract the context from the scope.
	ctx := context.TODO()

	err = vql_subsystem.CheckAccess(scope, acls.NETWORK)
	if err != nil {
		return nil, nil, err
	}

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
		secret_args, err := getSecret(ctx, scope, arg.Secret)
		if err != nil {
			return nil, nil, err
		}

		// The secret is authoritative for the connection details, so
		// the args it provides replace those from SSH_CONFIG. The host
		// key is the exception - secrets predate the hostkey field and
		// most do not carry one, so we keep any host key the caller
		// set explicitly rather than silently dropping it.
		arg = mergeSecretArgs(arg, secret_args)
	}

	config := &ssh.ClientConfig{
		User: arg.Username,
		// The SSH accessor is used to log into remote systems to
		// perform read-only analysis. Typically it is hard to know in
		// advance the host key since VQL queries run
		// non-interactively.
		//
		// If there is MITM attack then the accessor may receive
		// incorrect or fake results. This is acceptable as the risk
		// is the same as a rogue client or a compromised endpoint.
		//
		// As a compromise we log the host key we receive here. Users
		// can use this information to detect a potential MITM attack
		// in post analysis review.
		HostKeyCallback: func(hostname string,
			remote net.Addr, key ssh.PublicKey) error {
			encoded_key := string(base64.StdEncoding.EncodeToString(key.Marshal()))
			if arg.HostKey != "" {
				if arg.HostKey == encoded_key {
					scope.Log("ssh: Accepted host key %v", encoded_key)
					return nil
				}
				return fmt.Errorf("Rejected host key %v: Did not match %v",
					encoded_key, arg.HostKey)
			}

			scope.Log("ssh: Accepted host key %v", encoded_key)
			return nil
		},
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

	// Not running on the server, secrets don't work.
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

// Combine the args given in SSH_CONFIG with those recovered from a
// secret. The secret wins for the credentials it defines, because it
// may be shared between users and is the authoritative record of how
// to reach the remote system.
//
// The host key is treated differently: a secret is not required to
// carry one, so an explicitly configured hostkey is only used when the
// secret does not define its own. This keeps a host key enforced by a
// secret from being overridden, while still allowing a caller to pin a
// key for a secret that has none.
func mergeSecretArgs(from_config *SSHAccessorArgs, from_secret *SSHAccessorArgs) *SSHAccessorArgs {
	if from_config == nil {
		return from_secret
	}
	if from_secret == nil {
		return from_config
	}

	merged := *from_secret
	if merged.HostKey == "" {
		merged.HostKey = from_config.HostKey
	}
	return &merged
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
		HostKey:    secret_record.GetString("hostkey"),
	}
	return arg, nil
}
