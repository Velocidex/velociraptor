package artifacts

import (
	"regexp"

	"github.com/pkg/errors"
	actions_proto "www.velocidex.com/golang/velociraptor/actions/proto"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/crypto"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/vfilter"
)

var (
	obfuscator = &crypto.Obfuscator{}
)

// Compile the artifact definition into a VQL Request.
// TODO: Obfuscate let queries.
func Obfuscate(
	config_obj *config_proto.Config,
	result *actions_proto.VQLCollectorArgs) error {
	var err error

	// Do not do anything if we do not compress artifacts.
	if config_obj.Frontend == nil || config_obj.Frontend.DoNotCompressArtifacts {
		return nil
	}

	scope := vql_subsystem.MakeScope()
	for _, query := range result.Query {
		if query.Name != "" {
			query.Name, err = obfuscator.Encrypt(config_obj, query.Name)
			if err != nil {
				return err
			}
		}

		query.Description = ""

		// Parse and re-serialize the query into standard
		// forms. This removes comments.
		ast, err := vfilter.Parse(query.VQL)
		if err != nil {
			return errors.Wrap(err, "While parsing VQL: "+query.VQL)
		}

		// TODO: Compress the AST.
		query.VQL = ast.ToString(scope)
	}

	return nil
}

var obfuscated_item = regexp.MustCompile(`\$[a-fA-F0-9]+`)

func DeobfuscateString(config_obj *config_proto.Config, in string) string {
	if config_obj.Frontend.DoNotCompressArtifacts {
		return in
	}

	return obfuscated_item.ReplaceAllStringFunc(in, func(in string) string {
		out, err := obfuscator.Decrypt(config_obj, in)
		if err != nil {
			return in
		}
		return out
	})
}

func ObfuscateString(config_obj *config_proto.Config, in string) string {
	if config_obj.Frontend.DoNotCompressArtifacts {
		return in
	}

	out, err := obfuscator.Encrypt(config_obj, in)
	if err != nil {
		return in
	}
	return out
}

func Deobfuscate(
	config_obj *config_proto.Config,
	response *actions_proto.VQLResponse) error {
	var err error

	response.Query.Name, err = obfuscator.Decrypt(config_obj, response.Query.Name)
	if err != nil {
		return err
	}
	return err
}
