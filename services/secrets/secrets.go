package secrets

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/Velocidex/ordereddict"
	"github.com/Velocidex/ttlcache/v2"
	"google.golang.org/protobuf/proto"
	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/datastore"
	"www.velocidex.com/golang/velociraptor/paths"
	"www.velocidex.com/golang/velociraptor/services"
	"www.velocidex.com/golang/velociraptor/utils"
	"www.velocidex.com/golang/vfilter"
)

type SecretDefinition struct {
	*api_proto.SecretDefinition

	verifierLambda *vfilter.Lambda
}

func (self *SecretDefinition) Clone() *SecretDefinition {
	return &SecretDefinition{
		SecretDefinition: proto.Clone(self.SecretDefinition).(*api_proto.SecretDefinition),
		verifierLambda:   self.verifierLambda,
	}
}

func NewSecretDefinition(definition *api_proto.SecretDefinition) (
	*SecretDefinition, error) {

	// an empty verifier means all secret formats are accepted.
	verifier := definition.Verifier
	if verifier == "" {
		verifier = "x=>TRUE"
	}

	lambda, err := vfilter.ParseLambda(verifier)
	if err != nil {
		return nil, fmt.Errorf("Invalid verifier lambda: %w", err)
	}

	return &SecretDefinition{
		SecretDefinition: definition,
		verifierLambda:   lambda,
	}, nil
}

func SecretLRUKey(type_name, name string) string {
	return type_name + "/" + name
}

func NewSecret(type_name, name string,
	secret *ordereddict.Dict) *services.Secret {
	result := &services.Secret{
		Secret: &api_proto.Secret{
			Name:     name,
			TypeName: type_name,
			Secret:   make(map[string]string),
		},
		Data: secret,
	}

	for _, k := range secret.Keys() {
		v, pres := secret.Get(k)
		if pres {
			v_str, ok := v.(string)
			if ok {
				result.Secret.Secret[k] = v_str
			}
		}
	}

	return result
}

type SecretsService struct {
	mu          sync.Mutex
	definitions map[string]*SecretDefinition
	secrets_lru *ttlcache.Cache

	config_obj *config_proto.Config
}

func (self *SecretsService) getSecretDefinition(
	ctx context.Context, type_name string) (*SecretDefinition, error) {
	definition, pres := self.definitions[type_name]
	if !pres {
		return nil, fmt.Errorf("SecretDefinition %v not found: %w",
			type_name, utils.NotFoundError)
	}

	return definition, nil
}

func (self *SecretsService) getSecret(
	ctx context.Context, type_name, secret_name string) (
	*services.Secret, error) {
	secret, err := self.secrets_lru.Get(SecretLRUKey(type_name, secret_name))
	if err == nil {
		return secret.(*services.Secret), nil
	}

	db, err := datastore.GetDB(self.config_obj)
	if err != nil {
		return nil, err
	}

	secret_path_manager := paths.SecretsPathManager{}
	secret_proto := &api_proto.Secret{}
	err = db.GetSubject(self.config_obj,
		secret_path_manager.Secret(type_name, secret_name),
		secret_proto)
	if err != nil {
		return nil, err
	}

	result, err := NewSecretFromProto(ctx, self.config_obj, secret_proto)
	if err != nil {
		return nil, err
	}

	return result, self.secrets_lru.Set(
		SecretLRUKey(type_name, secret_name), result)
}

func (self *SecretsService) AddSecret(ctx context.Context,
	scope vfilter.Scope,
	type_name, secret_name string, secret *ordereddict.Dict) error {

	secrets_definition, err := self.getSecretDefinition(ctx, type_name)
	if err != nil {
		return err
	}

	// Make sure no extra fields are specified - just drop them on the
	// floor if they are.
	for _, k := range secret.Keys() {
		if !utils.InString(secrets_definition.Fields, k) {
			secret.Delete(k)
		}
	}

	// Ensure all the fields in the template are defined.
	for _, field := range secrets_definition.Fields {
		_, pres := secret.Get(field)
		if !pres {
			secret.Set(field, "")
		}
	}

	// Verify the secret using the verifier
	if !scope.Bool(secrets_definition.verifierLambda.Reduce(
		ctx, scope, []vfilter.Any{secret})) {
		return fmt.Errorf("Unable to verify secret for type %v", type_name)
	}

	secret_record := NewSecret(type_name, secret_name, secret)
	return self.setSecret(ctx, secret_record)
}

// Store the secret in datastore
func (self *SecretsService) setSecret(
	ctx context.Context, secret_record *services.Secret) error {

	db, err := datastore.GetDB(self.config_obj)
	if err != nil {
		return err
	}

	stored_secret, err := PrepareForStorage(
		ctx, self.config_obj, secret_record)
	if err != nil {
		return err
	}

	secret_path_manager := paths.SecretsPathManager{}
	err = db.SetSubject(self.config_obj,
		secret_path_manager.Secret(
			secret_record.TypeName, secret_record.Name),
		stored_secret)

	if err != nil {
		return err
	}

	return self.secrets_lru.Set(
		SecretLRUKey(secret_record.Name, secret_record.TypeName),
		secret_record)
}

func (self *SecretsService) GetSecretDefinitions(
	ctx context.Context) (result []*api_proto.SecretDefinition) {
	self.mu.Lock()
	defer self.mu.Unlock()

	for _, v := range self.definitions {
		result = append(result,
			proto.Clone(v.SecretDefinition).(*api_proto.SecretDefinition))
	}
	return result
}

func (self *SecretsService) deleteSecret(
	ctx context.Context, type_name, secret_name string) error {
	db, err := datastore.GetDB(self.config_obj)
	if err != nil {
		return err
	}

	_ = self.secrets_lru.Remove(SecretLRUKey(type_name, secret_name))

	secret_path_manager := paths.SecretsPathManager{}
	return db.DeleteSubject(self.config_obj,
		secret_path_manager.Secret(type_name, secret_name))
}

func (self *SecretsService) ModifySecret(ctx context.Context,
	request *api_proto.ModifySecretRequest) error {

	if request.Delete {
		return self.deleteSecret(ctx, request.TypeName, request.Name)
	}

	secret_record, err := self.getSecret(ctx, request.TypeName, request.Name)
	if err != nil {
		return err
	}

	users := ordereddict.NewDict()
	for _, user := range secret_record.Users {
		users.Set(user, 1)
	}

	for _, user := range request.RemoveUsers {
		users.Delete(user)
	}

	for _, user := range request.AddUsers {
		users.Set(user, 1)
	}

	secret_record.Users = users.Keys()

	return self.setSecret(ctx, secret_record)
}

func (self *SecretsService) GetSecret(ctx context.Context,
	principal, type_name, secret_name string) (*services.Secret, error) {

	secret_record, err := self.getSecret(ctx, type_name, secret_name)
	if err != nil {
		return nil, err
	}

	for _, u := range secret_record.Users {
		if u == principal {
			return secret_record, nil
		}
	}

	return nil, fmt.Errorf("Permission Denied accessing secret %v", secret_name)
}

// Returns a reducted version of the secret.
func (self *SecretsService) GetSecretMetadata(ctx context.Context,
	type_name, secret_name string) (*services.Secret, error) {

	secret_record, err := self.getSecret(ctx, type_name, secret_name)
	if err != nil {
		return nil, err
	}

	return &services.Secret{
		Secret: &api_proto.Secret{
			TypeName: secret_record.TypeName,
			Name:     secret_record.Name,

			// Do not return any actual secrets
			Secret: nil,
			Users:  secret_record.Users,
		}}, nil
}

func NewSecretsService(
	ctx context.Context,
	wg *sync.WaitGroup,
	config_obj *config_proto.Config) (services.SecretsService, error) {

	result := &SecretsService{
		definitions: buildInitialSecretDefinitions(),
		secrets_lru: ttlcache.NewCache(),
		config_obj:  config_obj,
	}

	result.secrets_lru.SetCacheSizeLimit(100)
	_ = result.secrets_lru.SetTTL(time.Minute)
	result.secrets_lru.SkipTTLExtensionOnHit(true)

	go func() {
		<-ctx.Done()
		result.secrets_lru.Close()
	}()

	return result, nil
}
