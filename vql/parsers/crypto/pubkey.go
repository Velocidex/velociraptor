package crypto

import (
	"bufio"
	"bytes"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha512"
	"crypto/x509"
	"errors"
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

type PKEncryptArgs struct {
	Data       string `vfilter:"required,field=data,doc=The data to encrypt"`
	SigningKey string `vfilter:"optional,field=signing_key,doc=Private key to sign with"`
	PublicKey  string `vfilter:"required,field=public_key,doc=Public key to encrypt with"`
	Scheme     string `vfilter:"required,field=scheme,doc=Encryption scheme to use. Currently supported: PGP"`
}

type PKDecryptArgs struct {
	Data       string `vfilter:"required,field=data,doc=The data to decrypt"`
	SigningKey string `vfilter:"optional,field=signing_key,doc=Public key to verify signature"`
	PrivateKey string `vfilter:"required,field=private_key,doc=Private key to decrypt with"`
	Scheme     string `vfilter:"required,field=scheme,doc=Encryption scheme to use. Currently supported: PGP"`
}

type PKEncryptFunction struct{}

type PKDecryptFunction struct{}

func (self *PKEncryptFunction) Call(ctx context.Context,
	scope vfilter.Scope,
	args *ordereddict.Dict) vfilter.Any {

	arg := &PKEncryptArgs{}
	err := arg_parser.ExtractArgsWithContext(ctx, scope, args, arg)
	if err != nil {
		scope.Log("ERROR:pk_encrypt: %s", err.Error())
		return vfilter.Null{}
	}

	switch strings.ToLower(arg.Scheme) {
	case "pgp":
		{
			pub_key_reader := strings.NewReader(arg.PublicKey)

			pk_entity, err := readPGPEntity(pub_key_reader)
			if err != nil {
				scope.Log("ERROR:pk_encrypt: %s", err.Error())
				return vfilter.Null{}
			}

			var signing_key_entity *openpgp.Entity
			if arg.SigningKey != "" {
				signing_key := strings.NewReader(arg.SigningKey)
				signing_key_entity, err = readPGPEntity(signing_key)
				if err != nil {
					scope.Log("ERROR:pk_encrypt: %s", err.Error())
					return vfilter.Null{}
				}
			}

			var b bytes.Buffer
			reader := strings.NewReader(arg.Data)
			writer := bufio.NewWriter(&b)
			err = encryptPGP([]*openpgp.Entity{pk_entity}, signing_key_entity, reader, writer)
			if err != nil {
				return vfilter.Null{}
			}
			writer.Flush()
			return b.Bytes()
		}
	case "x509":
		{
			cert, err := x509.ParseCertificate([]byte(arg.PublicKey))
			if err != nil {
				scope.Log("ERROR:pk_encrypt: %s", err.Error())
				return vfilter.Null{}
			}
			ciphertext, err := encryptWithX509PubKey([]byte(arg.Data), cert)
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

func encryptWithX509PubKey(msg []byte, cert *x509.Certificate) ([]byte, error) {
	pub := cert.PublicKey
	switch pub := pub.(type) {
	case *rsa.PublicKey:
		hash := sha512.New()
		return rsa.EncryptOAEP(hash, rand.Reader, pub, msg, nil)
	default:
		return nil, errors.New("Unsupported Type of Public Key")
	}
}

func readPGPEntity(reader io.Reader) (*openpgp.Entity, error) {
	block, err := armor.Decode(reader)

	if err != nil {
		return nil, err
	}

	return openpgp.ReadEntity(packet.NewReader(block.Body))
}

func readPGPEntityList(reader io.Reader) (openpgp.EntityList, error) {
	el, err := openpgp.ReadArmoredKeyRing(reader)
	if err == nil {
		return el, nil
	} else {
		return openpgp.ReadKeyRing(reader)
	}

}

func encryptPGP(recip []*openpgp.Entity,
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

func (self *PKDecryptFunction) Call(ctx context.Context,
	scope vfilter.Scope,
	args *ordereddict.Dict) vfilter.Any {

	arg := &PKDecryptArgs{}
	err := arg_parser.ExtractArgsWithContext(ctx, scope, args, arg)
	if err != nil {
		scope.Log("ERROR:pk_decrypt: %s", err.Error())
		return vfilter.Null{}
	}

	switch strings.ToLower(arg.Scheme) {
	case "pgp":
		{
			priv_key_reader := strings.NewReader(arg.PrivateKey)

			pk_entity, err := readPGPEntityList(priv_key_reader)
			if err != nil {
				scope.Log("ERROR:pk_decrypt: %s", err.Error())
				return vfilter.Null{}
			}

			var signing_key_entity *openpgp.Entity
			if arg.SigningKey != "" {
				signing_key := strings.NewReader(arg.SigningKey)
				signing_key_entity, err = readPGPEntity(signing_key)
				if err != nil {
					scope.Log("ERROR:pk_decrypt: %s", err.Error())
					return vfilter.Null{}
				}
			}

			reader := strings.NewReader(arg.Data)
			m, err := decryptPGP(pk_entity, signing_key_entity, reader)
			if err != nil {
				scope.Log("pk_decrypt: %s", err.Error())
				return vfilter.Null{}
			}
			bytes, err := ioutil.ReadAll(m.UnverifiedBody)
			if err != nil {
				scope.Log("pk_decrypt: %s", err.Error())
				return vfilter.Null{}
			}
			return bytes
		}
	default:
		scope.Log("ERROR:pk_encrypt: Unsupported Encryption Scheme.")
		return vfilter.Null{}
	}
}

func decryptPGP(recip openpgp.EntityList,
	signer *openpgp.Entity,
	r io.Reader,
) (*openpgp.MessageDetails, error) {

	m, err := openpgp.ReadMessage(r, recip, nil, nil)
	if err != nil {
		return nil, err
	}
	return m, nil

}

func (self PKEncryptFunction) Info(
	scope vfilter.Scope, type_map *vfilter.TypeMap) *vfilter.FunctionInfo {
	return &vfilter.FunctionInfo{
		Name:    "pk_encrypt",
		Doc:     "Encrypt files using pubkey encryption",
		ArgType: type_map.AddType(scope, &PKEncryptArgs{}),
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
