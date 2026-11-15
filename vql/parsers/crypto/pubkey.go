package crypto

import (
	"context"
	"strings"

	"github.com/Velocidex/ordereddict"

	"www.velocidex.com/golang/velociraptor/acls"
	crypto_utils "www.velocidex.com/golang/velociraptor/crypto/utils"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/vfilter"
	"www.velocidex.com/golang/vfilter/arg_parser"
)

type PKEncryptArgs struct {
	Data       string `vfilter:"required,field=data,doc=The data to encrypt"`
	SigningKey string `vfilter:"optional,field=signing_key,doc=Private key to sign with"`
	PublicKey  string `vfilter:"optional,field=public_key,doc=Public key to encrypt with. Defaults to server public key"`
	Scheme     string `vfilter:"optional,field=scheme,doc=Encryption scheme to use. Defaults to X509. Currently supported: X509"`
}

type PKDecryptArgs struct {
	Data       string `vfilter:"required,field=data,doc=The data to decrypt"`
	SigningKey string `vfilter:"optional,field=signing_key,doc=Public key to verify signature"`
	PrivateKey string `vfilter:"optional,field=private_key,doc=Private key to decrypt with. Defaults to server private key"`
	Scheme     string `vfilter:"optional,field=scheme,doc=Encryption scheme to use. Defaults to RSA. Currently supported: RSA"`
}

type PKEncryptFunction struct{}

type PKDecryptFunction struct{}

func (self *PKEncryptFunction) Call(ctx context.Context,
	scope vfilter.Scope,
	args *ordereddict.Dict) vfilter.Any {

	defer vql_subsystem.RegisterMonitor(ctx, "pk_encrypt", args)()

	arg := &PKEncryptArgs{}
	err := arg_parser.ExtractArgsWithContext(ctx, scope, args, arg)
	if err != nil {
		scope.Log("ERROR:pk_encrypt: %s", err.Error())
		return vfilter.Null{}
	}

	if arg.PublicKey == "" && (arg.Scheme == "" || strings.ToLower(arg.Scheme) == "x509") {
		config_obj, ok := vql_subsystem.GetServerConfig(scope)
		if !ok {
			scope.Log("ERROR:pk_encrypt: Must specify public key if not running on server")
			return vfilter.Null{}
		}
		arg.PublicKey = config_obj.Frontend.Certificate
		arg.Scheme = "x509"
	}

	switch strings.ToLower(arg.Scheme) {
	case "x509":
		{
			cert, err := crypto_utils.ParseX509CertFromPemStr([]byte(arg.PublicKey))
			if err != nil {
				scope.Log("ERROR:pk_encrypt: %s", err.Error())
				return vfilter.Null{}
			}

			ciphertext, err := crypto_utils.EncryptWithX509PubKey([]byte(arg.Data), cert)
			if err != nil {
				scope.Log("ERROR:pk_encrypt: %s", err.Error())
				return vfilter.Null{}
			}
			return ciphertext
		}
	default:
		scope.Log("ERROR:pk_encrypt: Unsupported Encryption Scheme.")
		return vfilter.Null{}
	}
}

func (self *PKDecryptFunction) Call(ctx context.Context,
	scope vfilter.Scope,
	args *ordereddict.Dict) vfilter.Any {
	defer vql_subsystem.RegisterMonitor(ctx, "pk_decrypt", args)()

	arg := &PKDecryptArgs{}
	err := arg_parser.ExtractArgsWithContext(ctx, scope, args, arg)
	if err != nil {
		scope.Log("ERROR:pk_decrypt: %s", err.Error())
		return vfilter.Null{}
	}
	if arg.PrivateKey == "" && (arg.Scheme == "" || strings.ToLower(arg.Scheme) == "rsa") {
		err = vql_subsystem.CheckAccess(scope, acls.SERVER_ADMIN)
		if err != nil {
			scope.Log("ERROR:pk_decrypt: Must be server admin to use private key")
			return vfilter.Null{}
		}
		config_obj, ok := vql_subsystem.GetServerConfig(scope)
		if !ok {
			scope.Log("ERROR:pk_decrypt: Must specify public key if not running on server")
			return vfilter.Null{}
		}
		arg.PrivateKey = config_obj.Frontend.PrivateKey
		arg.Scheme = "rsa"

	}

	switch strings.ToLower(arg.Scheme) {
	case "rsa":
		{
			key, err := crypto_utils.ParseRsaPrivateKeyFromPemStr([]byte(arg.PrivateKey))
			if err != nil {
				scope.Log("ERROR:pk_decrypt: %s", err.Error())
				return vfilter.Null{}
			}
			plaintext, err := crypto_utils.DecryptRSAOAEP(key, []byte(arg.Data))
			if err != nil {
				scope.Log("ERROR:pk_decrypt: %s", err.Error())
				return vfilter.Null{}
			}
			return plaintext
		}
	default:
		scope.Log("ERROR:pk_decrypt: Unsupported Encryption Scheme.")
		return vfilter.Null{}
	}
}

func (self PKEncryptFunction) Info(
	scope vfilter.Scope, type_map *vfilter.TypeMap) *vfilter.FunctionInfo {
	return &vfilter.FunctionInfo{
		Name:     "pk_encrypt",
		Doc:      "Encrypt files using pubkey encryption",
		ArgType:  type_map.AddType(scope, &PKEncryptArgs{}),
		Metadata: vql_subsystem.VQLMetadata().Permissions(acls.SERVER_ADMIN).Build(),
	}
}

func (self PKDecryptFunction) Info(
	scope vfilter.Scope, type_map *vfilter.TypeMap) *vfilter.FunctionInfo {
	return &vfilter.FunctionInfo{
		Name:    "pk_decrypt",
		Doc:     "Decrypt files using pubkey encryption",
		ArgType: type_map.AddType(scope, &PKDecryptArgs{}),
	}
}

func init() {
	vql_subsystem.RegisterFunction(&PKEncryptFunction{})
	vql_subsystem.RegisterFunction(&PKDecryptFunction{})
}
