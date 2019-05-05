package artifacts

import (
	actions_proto "www.velocidex.com/golang/velociraptor/actions/proto"
	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
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
	config_obj *api_proto.Config,
	result *actions_proto.VQLCollectorArgs) error {
	scope := vql_subsystem.MakeScope()
	var err error

	// Do not do anything if we do not compress artifacts.
	if config_obj.Frontend.DoNotCompressArtifacts {
		return nil
	}

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
			return err
		}

		// TODO: Compress the AST.
		query.VQL = ast.ToString(scope)
	}

	return nil
}

func Deobfuscate(
	config_obj *api_proto.Config,
	response *actions_proto.VQLResponse) error {
	var err error

	if config_obj.Frontend.DoNotCompressArtifacts {
		return nil
	}

	response.Query.Name, err = obfuscator.Decrypt(config_obj, response.Query.Name)
	if err != nil {
		return err
	}
	return err
}
