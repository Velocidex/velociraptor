package launcher

import (
	"context"
	"fmt"

	actions_proto "www.velocidex.com/golang/velociraptor/actions/proto"
	artifacts_proto "www.velocidex.com/golang/velociraptor/artifacts/proto"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/constants"
	"www.velocidex.com/golang/velociraptor/services"
)

func compileParametersToVQLPreamble(
	ctx context.Context, config_obj *config_proto.Config,
	artifact *artifacts_proto.Artifact) (
	res []string, res_env []*actions_proto.VQLEnv) {

	for _, parameter := range artifact.Parameters {
		value := parameter.Default
		name := parameter.Name

		env := &actions_proto.VQLEnv{
			Key:   name,
			Value: value,
		}

		// If the parameter has a type, convert it
		// appropriately. Note that parameters are always
		// passed into the client as strings, so they need to
		// be converted into their declared types explicitly
		// in the VQL code.

		// If the variable contains spaces we need to escape
		// the name in backticks.
		escaped_name := maybeEscape(name)

		switch parameter.Type {
		case "", "string", "regex", "yara":
			// Nothing to do with these types.

		case "redacted":
			env.Comment = "redacted"

		case "upload":
			res = append(res, fmt.Sprintf(
				`LET %v <= if(condition=%v, then={
   SELECT Content FROM http_client(url=%v)
})`, maybeEscape(name+"_"), escaped_name, escaped_name))

			res = append(res, fmt.Sprintf("LET %v <= %v.Content[0]",
				escaped_name, maybeEscape(name+"_")))

		case "upload_file":
			res = append(res, fmt.Sprintf(
				`LET %v <= if(condition=%v, then={
   SELECT Content FROM http_client(url=%v, tempfile_extension='.tmp')
})`, maybeEscape(name+"_"), escaped_name, escaped_name))

			res = append(res, fmt.Sprintf("LET %v <= %v.Content[0]",
				escaped_name, maybeEscape(name+"_")))

		case "server_metadata":
			client_info_manager, err := services.GetClientInfoManager(config_obj)
			if err == nil {
				md, err := client_info_manager.GetMetadata(ctx,
					constants.VELOCIRAPTOR_SERVER_CLIENT_ID)
				if err == nil {
					value, pres := md.GetString(name)
					if pres {
						env.Value = value
					}
				}
			}

		case "int", "int64", "integer":
			res = append(res, fmt.Sprintf("LET %v <= int(int=%v)",
				escaped_name, escaped_name))

		case "float":
			res = append(res, fmt.Sprintf("LET %v <= parse_float(string=%v)",
				escaped_name, escaped_name))

		case "timestamp":
			res = append(res, fmt.Sprintf("LET %v <= timestamp(epoch=%v)",
				escaped_name, escaped_name))

		case "starlark":
			res = append(res, fmt.Sprintf(`
LET %v <= if(
    condition=format(format="%%T", args=[%v,]) =~ "string",
    then=starl(code=%v),
    else=%v)
`,
				escaped_name, escaped_name, escaped_name, escaped_name))

		case "csv", "artifactset":
			// Only parse from CSV if it is a string.
			res = append(res, fmt.Sprintf(`
LET %v <= SELECT * FROM if(
    condition=format(format="%%T", args=[%v,]) =~ "string",
    then={SELECT * FROM parse_csv(filename=%v, accessor='data')},
    else=%v)
`, escaped_name, escaped_name, escaped_name, escaped_name))

			// Only parse from JSON if it is a string.
		case "json":
			res = append(res, fmt.Sprintf(`
LET %v <= if(
    condition=format(format="%%T", args=[%v,]) =~ "string",
    then=parse_json(data=%v),
    else=%v)
`, escaped_name, escaped_name, escaped_name, escaped_name))

		case "json_array", "regex_array", "multichoice":
			res = append(res, fmt.Sprintf(`
LET %v <= if(
    condition=format(format="%%T", args=[%v,]) = "string",
    then=parse_json_array(data=%v),
    else=%v)
`, escaped_name, escaped_name, escaped_name, escaped_name))

		case "xml":
			res = append(res, fmt.Sprintf(`
LET %v <= if(
    condition=format(format="%%T", args=[%v,]) =~ "string",
    then=parse_xml(file=%v, accessor="data"),
    else=%v)
`, escaped_name, escaped_name, escaped_name, escaped_name))

		case "yaml":
			res = append(res, fmt.Sprintf(`
LET %v <= if(
    condition=format(format="%%T", args=[%v,]) =~ "string",
    then=parse_yaml(filename=%v, accessor="data"),
    else=%v)
`, escaped_name, escaped_name, escaped_name, escaped_name))

		case "bool":
			res = append(res, fmt.Sprintf(
				`LET %v <= get(field='%v') = TRUE OR get(field='%v') =~ '^(Y|TRUE|YES|OK)$' `,
				escaped_name, name, name))
		}

		res_env = append(res_env, env)
	}

	return res, res_env
}
