package crypto

import (
	"bufio"
	"bytes"
	"io"
	"io/ioutil"
	"strings"

	"github.com/Velocidex/ordereddict"
	"golang.org/x/crypto/openpgp"
	"golang.org/x/crypto/openpgp/armor"
	"golang.org/x/crypto/openpgp/packet"

	"golang.org/x/net/context"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/vfilter"
	"www.velocidex.com/golang/vfilter/arg_parser"
)

type PGPEncryptArgs struct {
	Data       string `vfilter:"required,field=data,doc=The data to encrypt"`
	SigningKey string `vfilter:"optional,field=signing_key,doc=Private key to sign with"`
	PublicKey  string `vfilter:"required,field=public_key,doc=Public key to encrypt with"`
}

type PGPDecryptArgs struct {
	Data       string `vfilter:"required,field=data,doc=The data to decrypt"`
	SigningKey string `vfilter:"optional,field=signing_key,doc=Public key to verify signature"`
	PrivateKey string `vfilter:"required,field=private_key,doc=Private key to decrypt with"`
}

type PGPEncryptFunction struct{}

type PGPDecryptFunction struct{}

func (self *PGPEncryptFunction) Call(ctx context.Context,
	scope vfilter.Scope,
	args *ordereddict.Dict) vfilter.Any {

	arg := &PGPEncryptArgs{}
	err := arg_parser.ExtractArgsWithContext(ctx, scope, args, arg)
	if err != nil {
		scope.Log("pgp_encrypt: %s", err.Error())
		return vfilter.Null{}
	}

	pub_key_reader := strings.NewReader(arg.PublicKey)

	pk_entity, err := readEntity(pub_key_reader)
	if err != nil {
		scope.Log("pgp_encrypt: %s", err.Error())
		return vfilter.Null{}
	}

	var signing_key_entity *openpgp.Entity
	if arg.SigningKey != "" {
		signing_key := strings.NewReader(arg.SigningKey)
		signing_key_entity, err = readEntity(signing_key)
		if err != nil {
			scope.Log("pgp_encrypt: %s", err.Error())
			return vfilter.Null{}
		}
	}

	var b bytes.Buffer
	reader := strings.NewReader(arg.Data)
	writer := bufio.NewWriter(&b)
	err = encrypt([]*openpgp.Entity{pk_entity}, signing_key_entity, reader, writer)
	if err != nil {
		return vfilter.Null{}
	}
	writer.Flush()
	return b.Bytes()
}

func readEntity(reader io.Reader) (*openpgp.Entity, error) {
	block, err := armor.Decode(reader)

	if err != nil {
		return nil, err
	}

	return openpgp.ReadEntity(packet.NewReader(block.Body))
}

func readEntityList(reader io.Reader) (openpgp.EntityList, error) {
	el, err := openpgp.ReadArmoredKeyRing(reader)
	if err == nil {
		return el, nil
	} else {
		return openpgp.ReadKeyRing(reader)
	}

}

func encrypt(recip []*openpgp.Entity,
	signer *openpgp.Entity,
	r io.Reader,
	w io.Writer) error {

	wc, err := openpgp.Encrypt(w, recip, signer, nil, nil)
	defer wc.Close()
	if err != nil {
		return err
	}
	if _, err := io.Copy(wc, r); err != nil {
		return err
	}
	return nil

}

func (self *PGPDecryptFunction) Call(ctx context.Context,
	scope vfilter.Scope,
	args *ordereddict.Dict) vfilter.Any {

	arg := &PGPDecryptArgs{}
	err := arg_parser.ExtractArgsWithContext(ctx, scope, args, arg)
	if err != nil {
		scope.Log("pgp_decrypt: %s", err.Error())
		return vfilter.Null{}
	}

	priv_key_reader := strings.NewReader(arg.PrivateKey)

	pk_entity, err := readEntityList(priv_key_reader)
	if err != nil {
		scope.Log("pgp_decrypt: %s", err.Error())
		return vfilter.Null{}
	}

	var signing_key_entity *openpgp.Entity
	if arg.SigningKey != "" {
		signing_key := strings.NewReader(arg.SigningKey)
		signing_key_entity, err = readEntity(signing_key)
		if err != nil {
			scope.Log("pgp_decrypt: %s", err.Error())
			return vfilter.Null{}
		}
	}

	reader := strings.NewReader(arg.Data)
	m, err := decrypt(pk_entity, signing_key_entity, reader)
	if err != nil {
		scope.Log("pgp_decrypt: %s", err.Error())
		return vfilter.Null{}
	}
	bytes, err := ioutil.ReadAll(m.UnverifiedBody)
	if err != nil {
		scope.Log("pgp_decrypt: %s", err.Error())
		return vfilter.Null{}
	}
	return bytes
}

func decrypt(recip openpgp.EntityList,
	signer *openpgp.Entity,
	r io.Reader,
) (*openpgp.MessageDetails, error) {

	m, err := openpgp.ReadMessage(r, recip, nil, nil)
	if err != nil {
		return nil, err
	}
	return m, nil

}

func (self PGPEncryptFunction) Info(
	scope vfilter.Scope, type_map *vfilter.TypeMap) *vfilter.FunctionInfo {
	return &vfilter.FunctionInfo{
		Name:    "pgp_encrypt",
		Doc:     "Encrypt files using PGP",
		ArgType: type_map.AddType(scope, &PGPEncryptArgs{}),
	}
}

func (self PGPDecryptFunction) Info(
	scope vfilter.Scope, type_map *vfilter.TypeMap) *vfilter.FunctionInfo {
	return &vfilter.FunctionInfo{
		Name:    "pgp_decrypt",
		Doc:     "Decrypt files using PGP",
		ArgType: type_map.AddType(scope, &PGPDecryptArgs{}),
	}
}

func init() {
	vql_subsystem.RegisterFunction(&PGPEncryptFunction{})
	vql_subsystem.RegisterFunction(&PGPDecryptFunction{})
}
