package main

import (
	"fmt"
	"os"
	"reflect"
	"regexp"
	"strings"
	"unicode"

	"github.com/Velocidex/yaml/v2"
	"github.com/davecgh/go-spew/spew"
	kingpin "gopkg.in/alecthomas/kingpin.v2"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/constants"
	"www.velocidex.com/golang/velociraptor/utils"
)

var (
	app      = kingpin.New("config_check", "Check Config.")
	filename = app.Arg("filename",
		"Yaml filename to read (server.config.yaml)").Required().String()
	tagRegEx = regexp.MustCompile("json:\"([^,]+)")

	// Usually deprecated fields we dont want people to use so we dont
	// document them.
	hidden_fields = []string{
		"sub_authenticators",
		"autocert_domain",
		"version.description",
		"Client.min_poll",
		"Client.version",
		"Client.server_version",
		"Client.local_buffer.filename",
		"Client.dns_cache_refresh_min",
		"GUI.internal_cidr",
		"GUI.vpn_cidr",
		"GUI.authenticator.sub_authenticators",
		"GUI.artifact_search_filter",
		"GUI.saml_certificate",
		"GUI.saml_private_key",
		"GUI.saml_idp_metadata_url",
		"GUI.saml_root_url",
		"GUI.saml_user_attribute",
		"GUI.google_oauth_client_id",
		"GUI.google_oauth_client_secret",
		"Frontend.public_path",
		"Frontend.dns_name",
		"Frontend.server_services",
		"Frontend.resources.target_heap_size",
		"Frontend.resources.per_client_upload_rate",
		"Frontend.resources.global_upload_rate",
		"Frontend.resources.client_info_lru_ttl",
		"Frontend.resources.client_info_sync_time",
		"Frontend.resources.client_info_write_time",
		"Frontend.is_minion",
		"Frontend.concurrency",
		"Frontend.max_upload_size",
		"Frontend.expected_clients",
		"Frontend.per_client_upload_rate",
		"Frontend.global_upload_rate",
		"Frontend.client_event_max_wait",
		"Frontend.do_not_redirect",
		"ExtraFrontends",
		"Mail",
		"Writeback",
		"Logging.rotation_time",
		"Logging.max_age",
		"verbose",
		"api_config",
		"autoexec.artifact_definitions",
		"remappings",
		"org_id",
		"org_name",
		"analysis_target",
		"services",
		"Client.disable_compression",
		"Client.default_server_flow_stats_update",
		"defaults.max_vfs_directory_size",
		"Datastore.remote_datastore_rpc_deadline",

		// Deprecated fields that were moved to security:
		"defaults.inflight_check_time",
		"defaults.allowed_plugins",
		"defaults.allowed_functions",
		"defaults.allowed_accessors",
		"defaults.denied_plugins",
		"defaults.denied_functions",
		"defaults.denied_accessors",
		"defaults.lockdown_denied_permissions",
		"defaults.certificate_validity_days",

		// Fields that are already handled but their default value is
		// false or 0.
		"GUI.use_plain_http",
		"GUI.links.disabled",
		"GUI.authenticator.oidc_issuer",
		"GUI.authenticator.claims.roles",
		"GUI.authenticator.oidc_debug",
		"Frontend.use_plain_http",
		"Frontend.require_client_certificates",
		"defaults.disable_unicode_usernames",
		"Client.panic_file",
		"GUI.authenticator.saml_allow_idp_initiated",

		"Client.nanny_max_connection_delay",
		"Client.prevent_execve",
		"Client.max_memory_hard_limit",
		"Client.Crypto.allow_weak_tls_server",
		"(reflect.Kind) map",
		"Client.fallback_addresses",
		"Client.disable_checkpoints",
		"Client.additional_event_artifacts",
		"Frontend.resources.default_log_batch_time",
		"Frontend.resources.default_monitoring_log_batch_time",
		"Frontend.resources.disable_file_buffering",
		"defaults.notebook_memory_low_water_mark",
		"defaults.notebook_memory_high_water_mark",
		"defaults.event_change_notify_all_clients",
		"defaults.max_sparse_expand_size",
		"defaults.disable_server_events",
		"defaults.auth_redirect_template",
		"defaults.disable_quarantine_button",
		"defaults.default_theme",
		"defaults.max_rows",
		"defaults.max_row_buffer_size",
		"defaults.max_batch_wait",
		"defaults.disable_inventory_service_external_access",
		"lockdown",
		"debug_mode",
		"Client.proxy_config.ignore_environment",
		"Frontend.proxy_config.ignore_environment",
		"defaults.disable_active_inflight_checks",
		"defaults.write_internal_events",
		"security.secrets_dek",
		"security.vql_must_use_secrets",
		"security.disable_inventory_service_external_access",
	}
)

func IsExported(name string) bool {
	switch name {

	// Ignore common methods which should not be exported.
	case "MarshalJSON", "MarshalYAML":
		return false

	default:
		if len(name) == 0 || name[0] == '_' {
			return false
		}

		runes := []rune(name)
		return runes[0] == unicode.ToUpper(runes[0])
	}
}

func is_set(a reflect.Value) bool {
	if a.Kind() == reflect.Ptr {
		return !a.IsNil()
	}

	if a.Kind() == reflect.Slice {
		return a.Len() > 0
	}

	if a.Kind() == reflect.String {
		return a.Interface().(string) != ""
	}

	if a.Kind() == reflect.Bool {
		return a.Interface().(bool)
	}

	if a.Kind() == reflect.Int64 {
		return a.Interface().(int64) != 0
	}

	if a.Kind() == reflect.Uint64 {
		return a.Interface().(uint64) != 0
	}

	if a.Kind() == reflect.Uint32 {
		return a.Interface().(uint32) != 0
	}

	if a.Kind() == reflect.Float32 {
		return a.Interface().(float32) != 0
	}

	if a.Kind() == reflect.Map {
		return a.Len() > 0
	}

	spew.Dump(a.Kind())
	return false
}

func getFieldNameFromTag(tag string) string {
	submatches := tagRegEx.FindStringSubmatch(tag)
	if len(submatches) > 0 {
		return submatches[1]
	}
	return tag
}

func walk_field(a reflect.Value, prefix []string) {
	a_value := reflect.Indirect(a)
	if a_value.Kind() == reflect.Struct {
		for i := 0; i < a_value.NumField(); i++ {
			field_type := a_value.Type().Field(i)
			if !IsExported(field_type.Name) {
				continue
			}

			field_value := a_value.FieldByName(field_type.Name)
			if !field_value.IsValid() ||
				!field_value.CanInterface() {
				continue
			}

			field_name := getFieldNameFromTag(string(field_type.Tag))
			full_field_name := strings.Join(append(prefix, field_name), ".")
			if InString(full_field_name, hidden_fields) {
				continue
			}

			if !is_set(field_value) {
				fmt.Println(full_field_name)
				if field_value.Kind() == reflect.Ptr && field_value.IsNil() {
					dummy := reflect.New(field_value.Type().Elem())
					walk_field(dummy, append(prefix, field_name))
				}

				if field_value.Kind() == reflect.Slice &&
					field_value.Len() == 0 {
					dummy_item := field_value.Type().Elem()
					if dummy_item.Kind() == reflect.Ptr {
						dummy := reflect.New(dummy_item.Elem())
						walk_field(dummy, append(prefix, field_name))
					}
				}

			} else {
				walk_field(field_value, append(prefix, field_name))
			}
		}

		return
	}

	if a_value.Kind() == reflect.Slice {
		if a_value.Len() == 0 {
			return
		}

		// Just need one example from an array
		walk_field(a_value.Index(0), prefix)
	}
}

func parse_config(filename string) error {
	config := &config_proto.Config{}
	fd, err := os.Open(filename)
	if err != nil {
		return err
	}

	serialized, err := utils.ReadAllWithLimit(fd, constants.MAX_MEMORY)
	if err != nil {
		return err
	}
	err = yaml.Unmarshal(serialized, config)
	if err != nil {
		return err
	}

	walk_field(reflect.ValueOf(config), nil)

	return nil
}

func InString(name string, array []string) bool {
	for _, i := range array {
		if name == i {
			return true
		}
	}
	return false
}

func main() {
	args := os.Args[1:]
	kingpin.MustParse(app.Parse(args))

	err := parse_config(*filename)
	kingpin.FatalIfError(err, "")
}
