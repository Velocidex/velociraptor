//+build extras

package tools

import (
	"bufio"
	"bytes"
	"io"
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

type GPGEncryptArgs struct {
	Data       string `vfilter:"required,field=data,doc=The data to encrypt"`
	SigningKey string `vfilter:"optional,field=signing_key,doc=Private key to sign with"`
	PublicKey  string `vfilter:"required,field=public_key,doc=Public key to encrypt with"`
}

type GPGEncryptFunction struct{}

func (self *GPGEncryptFunction) Call(ctx context.Context,
	scope vfilter.Scope,
	args *ordereddict.Dict) vfilter.Any {

	arg := &GPGEncryptArgs{}
	err := arg_parser.ExtractArgsWithContext(ctx, scope, args, arg)
	if err != nil {
		scope.Log("gpg_encrypt: %s", err.Error())
		return vfilter.Null{}
	}

	pub_key_reader := strings.NewReader(arg.PublicKey)

	pk_entity, err := readEntity(pub_key_reader)
	if err != nil {
		scope.Log("gpg_encrypt: %s", err.Error())
		return vfilter.Null{}
	}

	var signing_key_entity *openpgp.Entity
	if arg.SigningKey != "" {
		signing_key := strings.NewReader(arg.SigningKey)
		signing_key_entity, err = readEntity(signing_key)
		if err != nil {
			scope.Log("gpg_encrypt: %s", err.Error())
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
	return b.String()
}

func readEntity(reader io.Reader) (*openpgp.Entity, error) {
	block, err := armor.Decode(reader)

	if err != nil {
		return nil, err
	}

	return openpgp.ReadEntity(packet.NewReader(block.Body))
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

func (self GPGEncryptFunction) Info(
	scope vfilter.Scope, type_map *vfilter.TypeMap) *vfilter.FunctionInfo {
	return &vfilter.FunctionInfo{
		Name:    "gpg_encrypt",
		Doc:     "Encrypt files using GPG",
		ArgType: type_map.AddType(scope, &GPGEncryptArgs{}),
	}
}

func init() {
	vql_subsystem.RegisterFunction(&GPGEncryptFunction{})
}
